package plugins

import (
	"fmt"
	"os"
	"strings"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/module"
)

// ModuleDefinition describes a skill-driven plugin module loaded from YAML.
//
// The struct mirrors the on-disk schema under .lattice/modules/*.yaml and is
// intentionally narrow so the engine can validate plugin metadata before
// wiring it into the workflow runtime.
type ModuleDefinition struct {
	ID          string                    `json:"id" yaml:"id"`
	Name        string                    `json:"name,omitempty" yaml:"name,omitempty"`
	Description string                    `json:"description,omitempty" yaml:"description,omitempty"`
	Version     string                    `json:"version" yaml:"version"`
	Skill       SkillDefinition           `json:"skill" yaml:"skill"`
	Inputs      []ArtifactBinding         `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs     []ArtifactBinding         `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	Concurrency module.ConcurrencyProfile `json:"concurrency,omitempty" yaml:"concurrency,omitempty"`
	Config      module.Config             `json:"config,omitempty" yaml:"config,omitempty"`
}

// Normalized returns a trimmed, copy-on-write variant of the definition.
func (def ModuleDefinition) Normalized() ModuleDefinition {
	clone := ModuleDefinition{
		ID:          strings.TrimSpace(def.ID),
		Name:        strings.TrimSpace(def.Name),
		Description: strings.TrimSpace(def.Description),
		Version:     strings.TrimSpace(def.Version),
		Skill:       def.Skill.normalized(),
		Concurrency: def.Concurrency,
	}
	if len(def.Inputs) > 0 {
		clone.Inputs = make([]ArtifactBinding, len(def.Inputs))
		for i, binding := range def.Inputs {
			clone.Inputs[i] = binding.normalized()
		}
	}
	if len(def.Outputs) > 0 {
		clone.Outputs = make([]ArtifactBinding, len(def.Outputs))
		for i, binding := range def.Outputs {
			clone.Outputs[i] = binding.normalized()
		}
	}
	if len(def.Config) > 0 {
		clone.Config = make(module.Config, len(def.Config))
		for key, value := range def.Config {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			clone.Config[trimmed] = value
		}
	}
	return clone
}

// Validate ensures the plugin definition is well-formed and references known
// artifacts/skills.
func (def ModuleDefinition) Validate() error {
	normalized := def.Normalized()
	if normalized.ID == "" {
		return fmt.Errorf("plugin: id is required")
	}
	if normalized.Version == "" {
		return fmt.Errorf("plugin %s: version is required", normalized.ID)
	}
	if err := normalized.Skill.Validate(); err != nil {
		return fmt.Errorf("plugin %s: skill: %w", normalized.ID, err)
	}
	if err := validateBindings("inputs", normalized.Inputs); err != nil {
		return fmt.Errorf("plugin %s: %w", normalized.ID, err)
	}
	if err := validateBindings("outputs", normalized.Outputs); err != nil {
		return fmt.Errorf("plugin %s: %w", normalized.ID, err)
	}
	if len(normalized.Outputs) == 0 {
		return fmt.Errorf("plugin %s: at least one output is required", normalized.ID)
	}
	return nil
}

// SkillDefinition declares how the skill runner should launch OpenCode.
type SkillDefinition struct {
	Slug       string            `json:"slug,omitempty" yaml:"slug,omitempty"`
	Path       string            `json:"path,omitempty" yaml:"path,omitempty"`
	Prompt     string            `json:"prompt" yaml:"prompt"`
	WindowName string            `json:"window_name,omitempty" yaml:"window_name,omitempty"`
	Env        map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Variables  map[string]string `json:"variables,omitempty" yaml:"variables,omitempty"`
}

func (def SkillDefinition) normalized() SkillDefinition {
	clone := SkillDefinition{
		Slug:       strings.TrimSpace(def.Slug),
		Path:       strings.TrimSpace(def.Path),
		Prompt:     strings.TrimSpace(def.Prompt),
		WindowName: strings.TrimSpace(def.WindowName),
	}
	if len(def.Env) > 0 {
		clone.Env = make(map[string]string, len(def.Env))
		for key, value := range def.Env {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				continue
			}
			clone.Env[trimmedKey] = strings.TrimSpace(value)
		}
	}
	if len(def.Variables) > 0 {
		clone.Variables = make(map[string]string, len(def.Variables))
		for key, value := range def.Variables {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				continue
			}
			clone.Variables[trimmedKey] = strings.TrimSpace(value)
		}
	}
	return clone
}

// Validate ensures the skill definition is executable by the runner.
func (def SkillDefinition) Validate() error {
	normalized := def.normalized()
	if normalized.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	if normalized.Slug == "" && normalized.Path == "" {
		return fmt.Errorf("either slug or path is required")
	}
	if normalized.Slug != "" && strings.Contains(normalized.Slug, string(os.PathSeparator)) {
		return fmt.Errorf("slug %s contains path separator", normalized.Slug)
	}
	return nil
}

// ArtifactBinding references a declared artifact ID and whether it is optional.
type ArtifactBinding struct {
	Artifact string `json:"artifact" yaml:"artifact"`
	Optional bool   `json:"optional,omitempty" yaml:"optional,omitempty"`
}

func (binding ArtifactBinding) normalized() ArtifactBinding {
	return ArtifactBinding{
		Artifact: strings.TrimSpace(binding.Artifact),
		Optional: binding.Optional,
	}
}

// Validate ensures the binding references a known artifact.
func (binding ArtifactBinding) Validate() error {
	normalized := binding.normalized()
	if normalized.Artifact == "" {
		return fmt.Errorf("artifact id is required")
	}
	if _, ok := artifact.Lookup(normalized.Artifact); !ok {
		return fmt.Errorf("artifact %s is not registered", normalized.Artifact)
	}
	return nil
}

// Resolve returns the artifact reference declared by the binding. Optional flags
// override the default optionality set by the artifact catalog.
func (binding ArtifactBinding) Resolve() (artifact.ArtifactRef, error) {
	normalized := binding.normalized()
	ref, ok := artifact.Lookup(normalized.Artifact)
	if !ok {
		return artifact.ArtifactRef{}, fmt.Errorf("artifact %s is not registered", normalized.Artifact)
	}
	ref.Optional = normalized.Optional
	return ref, nil
}

func validateBindings(label string, bindings []ArtifactBinding) error {
	if len(bindings) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(bindings))
	for idx, binding := range bindings {
		if err := binding.Validate(); err != nil {
			return fmt.Errorf("%s[%d]: %w", label, idx, err)
		}
		key := binding.normalized().Artifact
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%s[%d]: duplicate artifact %s", label, idx, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}
