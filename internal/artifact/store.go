package artifact

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/yourusername/lattice/internal/workflow"
)

// Store manages artifact IO rooted at the workflow directory.
type Store struct {
	workflow *workflow.Workflow
	now      func() time.Time
}

// StoreOption customizes a Store during construction.
type StoreOption func(*Store)

// WithClock overrides the clock used for metadata timestamps.
func WithClock(clock func() time.Time) StoreOption {
	return func(s *Store) {
		s.now = clock
	}
}

// NewStore builds a store for a workflow.
func NewStore(wf *workflow.Workflow, opts ...StoreOption) *Store {
	store := &Store{
		workflow: wf,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(store)
	}
	return store
}

// Check inspects the artifact on disk and returns its status and metadata.
func (s *Store) Check(ref ArtifactRef) (CheckResult, error) {
	path := ref.Path(s.workflow)
	if path == "" {
		err := fmt.Errorf("artifact: %s path could not be resolved", ref.ID)
		return CheckResult{Ref: ref, Path: path, State: StateError, Err: err}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return CheckResult{Ref: ref, Path: path, State: StateMissing}, nil
		}
		return CheckResult{Ref: ref, Path: path, State: StateError, Err: err}, err
	}
	switch ref.Kind {
	case KindMarker:
		if info.IsDir() {
			return invalidResult(ref, path, fmt.Errorf("artifact: expected marker file got directory"))
		}
		return CheckResult{Ref: ref, Path: path, State: StateReady}, nil
	case KindDirectory:
		if !info.IsDir() {
			return invalidResult(ref, path, fmt.Errorf("artifact: expected directory"))
		}
		return CheckResult{Ref: ref, Path: path, State: StateReady}, nil
	case KindJSON:
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return CheckResult{Ref: ref, Path: path, State: StateError, Err: readErr}, readErr
		}
		meta, metaErr := parseJSONMetadata(data)
		if metaErr != nil {
			return invalidResult(ref, path, metaErr)
		}
		if meta.ArtifactID != ref.ID {
			return invalidResult(ref, path, fmt.Errorf("artifact: metadata id %s does not match %s", meta.ArtifactID, ref.ID))
		}
		return CheckResult{Ref: ref, Path: path, State: StateReady, Metadata: &meta}, nil
	default:
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return CheckResult{Ref: ref, Path: path, State: StateError, Err: readErr}, readErr
		}
		meta, _, metaErr := ParseFrontMatter(data)
		if metaErr != nil {
			return invalidResult(ref, path, metaErr)
		}
		if meta.ArtifactID != ref.ID {
			return invalidResult(ref, path, fmt.Errorf("artifact: metadata id %s does not match %s", meta.ArtifactID, ref.ID))
		}
		return CheckResult{Ref: ref, Path: path, State: StateReady, Metadata: &meta}, nil
	}
}

// Write persists the artifact contents and metadata based on its kind.
func (s *Store) Write(ref ArtifactRef, body []byte, meta Metadata) error {
	path := ref.Path(s.workflow)
	if path == "" {
		return fmt.Errorf("artifact: %s path could not be resolved", ref.ID)
	}
	switch ref.Kind {
	case KindMarker:
		return ensureMarker(path)
	case KindDirectory:
		return os.MkdirAll(path, 0o755)
	case KindJSON:
		return s.writeJSON(path, ref, body, meta)
	default:
		return s.writeDocument(path, ref, body, meta)
	}
}

func (s *Store) writeDocument(path string, ref ArtifactRef, body []byte, meta Metadata) error {
	if body == nil {
		body = []byte{}
	}
	prepared := meta.WithDefaults(ref, s.now())
	if err := prepared.ValidateFor(ref); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content, err := WriteFrontMatter(prepared, body)
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func (s *Store) writeJSON(path string, ref ArtifactRef, body []byte, meta Metadata) error {
	if body == nil {
		body = []byte("{}")
	}
	prepared := meta.WithDefaults(ref, s.now())
	if err := prepared.ValidateFor(ref); err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("artifact: invalid json body for %s: %w", ref.ID, err)
	}
	payload["_lattice"] = metadataToJSON(prepared)
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("artifact: encode json for %s: %w", ref.ID, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o644)
}

func ensureMarker(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte{}, 0o644)
}

func invalidResult(ref ArtifactRef, path string, err error) (CheckResult, error) {
	return CheckResult{Ref: ref, Path: path, State: StateInvalid, Err: err}, err
}

func parseJSONMetadata(data []byte) (Metadata, error) {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return Metadata{}, fmt.Errorf("artifact: parse json metadata: %w", err)
	}
	raw, ok := payload["_lattice"]
	if !ok {
		return Metadata{}, fmt.Errorf("artifact: missing _lattice metadata")
	}
	metaMap, ok := raw.(map[string]any)
	if !ok {
		return Metadata{}, fmt.Errorf("artifact: invalid _lattice metadata structure")
	}
	return metadataFromMap(metaMap)
}

func metadataToJSON(meta Metadata) map[string]any {
	result := map[string]any{
		"artifact": meta.ArtifactID,
		"module":   meta.ModuleID,
		"version":  meta.Version,
		"workflow": meta.Workflow,
		"inputs":   append([]string{}, meta.Inputs...),
		"created":  meta.CreatedAt.UTC().Format(timeLayout),
	}
	if meta.Checksum != "" {
		result["checksum"] = meta.Checksum
	}
	if len(meta.Notes) > 0 {
		result["notes"] = cloneNotes(meta.Notes)
	}
	return result
}

func metadataFromMap(values map[string]any) (Metadata, error) {
	artifactID := stringValue(values["artifact"])
	moduleID := stringValue(values["module"])
	version := stringValue(values["version"])
	workflow := stringValue(values["workflow"])
	if artifactID == "" || moduleID == "" || version == "" {
		return Metadata{}, fmt.Errorf("artifact: incomplete metadata")
	}
	created := stringValue(values["created"])
	if created == "" {
		return Metadata{}, fmt.Errorf("artifact: metadata missing created timestamp")
	}
	timeValue, err := parseTime(created)
	if err != nil {
		return Metadata{}, err
	}
	inputs := sliceStringValue(values["inputs"])
	notes := mapStringValue(values["notes"])
	return Metadata{
		ArtifactID: artifactID,
		ModuleID:   moduleID,
		Version:    version,
		Workflow:   workflow,
		Inputs:     inputs,
		CreatedAt:  timeValue,
		Checksum:   stringValue(values["checksum"]),
		Notes:      notes,
	}, nil
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return ""
	}
}

func sliceStringValue(value any) []string {
	arr, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s := stringValue(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func mapStringValue(value any) map[string]string {
	raw, ok := value.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s := stringValue(v); s != "" {
			out[k] = s
		}
	}
	return out
}
