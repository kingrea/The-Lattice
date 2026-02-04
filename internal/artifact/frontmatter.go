package artifact

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	// ErrMissingFrontMatter indicates the document did not start with a YAML fence.
	ErrMissingFrontMatter = errors.New("artifact: missing frontmatter")
	// ErrMalformedFrontMatter indicates the YAML block could not be parsed.
	ErrMalformedFrontMatter = errors.New("artifact: malformed frontmatter")
)

// ParseFrontMatter extracts the metadata block and body from a document that starts
// with `---` YAML fences.
func ParseFrontMatter(content []byte) (Metadata, []byte, error) {
	if len(content) == 0 {
		return Metadata{}, nil, ErrMissingFrontMatter
	}
	normalized := normalizeNewlines(content)
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return Metadata{}, nil, ErrMissingFrontMatter
	}
	rest := normalized[4:]
	parts := bytes.SplitN(rest, []byte("\n---\n"), 2)
	if len(parts) < 2 {
		return Metadata{}, nil, ErrMalformedFrontMatter
	}
	metaBytes := parts[0]
	body := parts[1]
	var envelope latticeEnvelope
	if err := yaml.Unmarshal(metaBytes, &envelope); err != nil {
		return Metadata{}, nil, fmt.Errorf("artifact: parse frontmatter: %w", err)
	}
	meta, err := envelope.toMetadata()
	if err != nil {
		return Metadata{}, nil, err
	}
	return meta, body, nil
}

// WriteFrontMatter renders metadata + body with YAML fences.
func WriteFrontMatter(meta Metadata, body []byte) ([]byte, error) {
	if meta.ArtifactID == "" {
		return nil, fmt.Errorf("artifact: metadata missing artifact id")
	}
	envelope := latticeEnvelope{}
	envelope.fromMetadata(meta)
	data, err := yaml.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("artifact: encode frontmatter: %w", err)
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(bytes.TrimRight(data, "\n"))
	buf.WriteString("\n---\n\n")
	buf.Write(body)
	return buf.Bytes(), nil
}

type latticeEnvelope struct {
	Lattice latticeMetadata `yaml:"lattice"`
}

type latticeMetadata struct {
	Artifact string            `yaml:"artifact"`
	Module   string            `yaml:"module"`
	Version  string            `yaml:"version"`
	Workflow string            `yaml:"workflow,omitempty"`
	Inputs   []string          `yaml:"inputs,omitempty"`
	Created  string            `yaml:"created"`
	Checksum string            `yaml:"checksum,omitempty"`
	Notes    map[string]string `yaml:"notes,omitempty"`
}

func (e latticeEnvelope) toMetadata() (Metadata, error) {
	if e.Lattice.Artifact == "" || e.Lattice.Module == "" || e.Lattice.Version == "" {
		return Metadata{}, ErrMalformedFrontMatter
	}
	created, err := parseTime(e.Lattice.Created)
	if err != nil {
		return Metadata{}, fmt.Errorf("artifact: parse created timestamp: %w", err)
	}
	return Metadata{
		ArtifactID: e.Lattice.Artifact,
		ModuleID:   e.Lattice.Module,
		Version:    e.Lattice.Version,
		Workflow:   e.Lattice.Workflow,
		Inputs:     append([]string{}, e.Lattice.Inputs...),
		CreatedAt:  created,
		Checksum:   e.Lattice.Checksum,
		Notes:      cloneNotes(e.Lattice.Notes),
	}, nil
}

func (e *latticeEnvelope) fromMetadata(meta Metadata) {
	e.Lattice.Artifact = meta.ArtifactID
	e.Lattice.Module = meta.ModuleID
	e.Lattice.Version = meta.Version
	e.Lattice.Workflow = meta.Workflow
	e.Lattice.Inputs = append([]string{}, meta.Inputs...)
	e.Lattice.Created = meta.CreatedAt.UTC().Format(timeLayout)
	e.Lattice.Checksum = meta.Checksum
	e.Lattice.Notes = cloneNotes(meta.Notes)
}

func cloneNotes(notes map[string]string) map[string]string {
	if len(notes) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(notes))
	for k, v := range notes {
		cloned[k] = v
	}
	return cloned
}

const timeLayout = "2006-01-02T15:04:05Z07:00"

func parseTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, fmt.Errorf("artifact: empty created timestamp")
	}
	t, err := time.Parse(timeLayout, value)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func normalizeNewlines(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
}
