package runtime

import (
	"fmt"
	"os"
	"strings"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/module"
)

// MetadataOption customizes the metadata written for an artifact.
type MetadataOption func(*artifact.Metadata)

// WithInputs records the upstream artifact identifiers in metadata.
func WithInputs(refs ...artifact.ArtifactRef) MetadataOption {
	return func(meta *artifact.Metadata) {
		if len(refs) == 0 {
			return
		}
		ids := make([]string, 0, len(refs))
		for _, ref := range refs {
			if ref.ID != "" {
				ids = append(ids, ref.ID)
			}
		}
		if len(ids) > 0 {
			meta.Inputs = ids
		}
	}
}

// WithFingerprint records a fingerprint value for the provided artifact.
func WithFingerprint(ref artifact.ArtifactRef, value string) MetadataOption {
	return func(meta *artifact.Metadata) {
		if strings.TrimSpace(value) == "" {
			return
		}
		if meta.Notes == nil {
			meta.Notes = map[string]string{}
		}
		meta.Notes[module.FingerprintNoteKey(ref.ID)] = value
	}
}

// ValidateContext ensures modules receive a usable context.
func ValidateContext(moduleID string, ctx *module.ModuleContext) error {
	if ctx == nil {
		return fmt.Errorf("%s: context is nil", moduleID)
	}
	if ctx.Config == nil {
		return fmt.Errorf("%s: config is required", moduleID)
	}
	if ctx.Workflow == nil {
		return fmt.Errorf("%s: workflow is required", moduleID)
	}
	if ctx.Artifacts == nil {
		return fmt.Errorf("%s: artifact store is required", moduleID)
	}
	return nil
}

// EnsureDocument checks the artifact and rewrites it with lattice metadata if needed.
func EnsureDocument(ctx *module.ModuleContext, moduleID, version string, ref artifact.ArtifactRef, opts ...MetadataOption) (bool, error) {
	result, err := ctx.Artifacts.Check(ref)
	if err != nil {
		return false, fmt.Errorf("%s: check %s: %w", moduleID, ref.ID, err)
	}
	switch result.State {
	case artifact.StateReady:
		if result.Metadata == nil || result.Metadata.ModuleID != moduleID || result.Metadata.Version != version {
			if err := writeDocument(ctx, moduleID, version, ref, opts...); err != nil {
				return false, err
			}
			return false, nil
		}
		return true, nil
	case artifact.StateMissing:
		return false, nil
	case artifact.StateInvalid:
		if err := writeDocument(ctx, moduleID, version, ref, opts...); err != nil {
			return false, err
		}
		return false, nil
	case artifact.StateError:
		if result.Err != nil {
			return false, fmt.Errorf("%s: %s: %w", moduleID, ref.ID, result.Err)
		}
		return false, fmt.Errorf("%s: %s encountered an unknown error", moduleID, ref.ID)
	default:
		return false, nil
	}
}

// EnsureDocuments iterates over multiple artifacts.
func EnsureDocuments(ctx *module.ModuleContext, moduleID, version string, refs []artifact.ArtifactRef, opts ...MetadataOption) (bool, error) {
	for _, ref := range refs {
		ready, err := EnsureDocument(ctx, moduleID, version, ref, opts...)
		if err != nil {
			return false, err
		}
		if !ready {
			return false, nil
		}
	}
	return true, nil
}

// EnsureMarker validates marker artifacts.
func EnsureMarker(ctx *module.ModuleContext, moduleID, version string, ref artifact.ArtifactRef) (bool, error) {
	result, err := ctx.Artifacts.Check(ref)
	if err != nil {
		return false, fmt.Errorf("%s: check %s: %w", moduleID, ref.ID, err)
	}
	switch result.State {
	case artifact.StateReady:
		return true, nil
	case artifact.StateMissing:
		return false, nil
	case artifact.StateInvalid:
		if err := ctx.Artifacts.Write(ref, nil, artifact.Metadata{ArtifactID: ref.ID, ModuleID: moduleID, Version: version, Workflow: ctx.Workflow.Dir()}); err != nil {
			return false, fmt.Errorf("%s: rewrite %s: %w", moduleID, ref.ID, err)
		}
		return false, nil
	case artifact.StateError:
		if result.Err != nil {
			return false, fmt.Errorf("%s: %s: %w", moduleID, ref.ID, result.Err)
		}
		return false, fmt.Errorf("%s: %s encountered an unknown error", moduleID, ref.ID)
	default:
		return false, nil
	}
}

func writeDocument(ctx *module.ModuleContext, moduleID, version string, ref artifact.ArtifactRef, opts ...MetadataOption) error {
	path := ref.Path(ctx.Workflow)
	if path == "" {
		return fmt.Errorf("%s: unable to resolve path for %s", moduleID, ref.ID)
	}
	body, err := readDocumentBody(path)
	if err != nil {
		return fmt.Errorf("%s: read %s: %w", moduleID, ref.ID, err)
	}
	meta := artifact.Metadata{
		ArtifactID: ref.ID,
		ModuleID:   moduleID,
		Version:    version,
		Workflow:   ctx.Workflow.Dir(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&meta)
		}
	}
	if err := ctx.Artifacts.Write(ref, body, meta); err != nil {
		return fmt.Errorf("%s: write %s: %w", moduleID, ref.ID, err)
	}
	return nil
}

func readDocumentBody(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	if _, body, err := artifact.ParseFrontMatter(data); err == nil {
		return body, nil
	}
	return data, nil
}
