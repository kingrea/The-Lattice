package workflow

import (
	"fmt"
	"sort"
)

// DependencyGraph maps workflow-scoped module identifiers to the module IDs they
// depend on. The resolver treats the keys as aliases that correspond to
// ModuleRef.InstanceID().
type DependencyGraph map[string][]string

// Clone returns a deep copy of the graph.
func (g DependencyGraph) Clone() DependencyGraph {
	if len(g) == 0 {
		return nil
	}
	out := make(DependencyGraph, len(g))
	for key, deps := range g {
		if len(deps) == 0 {
			out[key] = nil
			continue
		}
		clone := make([]string, len(deps))
		copy(clone, deps)
		out[key] = clone
	}
	return out
}

// WorkflowDefinition declares an executable workflow graph composed of modules
// plus any metadata required to render it inside the TUI.
type WorkflowDefinition struct {
	ID          string                `json:"id" yaml:"id"`
	Name        string                `json:"name" yaml:"name"`
	Description string                `json:"description,omitempty" yaml:"description,omitempty"`
	Modules     []ModuleRef           `json:"modules" yaml:"modules"`
	Graph       DependencyGraph       `json:"graph,omitempty" yaml:"graph,omitempty"`
	Metadata    map[string]string     `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Runtime     WorkflowRuntimeConfig `json:"runtime,omitempty" yaml:"runtime,omitempty"`
}

// Clone returns a deep copy of the workflow definition.
func (def WorkflowDefinition) Clone() WorkflowDefinition {
	clone := WorkflowDefinition{
		ID:          def.ID,
		Name:        def.Name,
		Description: def.Description,
		Metadata:    cloneStringMap(def.Metadata),
		Graph:       def.Graph.Clone(),
		Runtime:     def.Runtime,
	}
	if len(def.Modules) > 0 {
		clone.Modules = make([]ModuleRef, len(def.Modules))
		for i, ref := range def.Modules {
			clone.Modules[i] = ref.Clone()
		}
	}
	return clone
}

// Validate ensures the workflow definition is self-consistent.
func (def WorkflowDefinition) Validate() error {
	if def.ID == "" {
		return fmt.Errorf("workflow: id is required")
	}
	if len(def.Modules) == 0 {
		return fmt.Errorf("workflow %s: at least one module is required", def.ID)
	}
	seen := map[string]struct{}{}
	for idx, ref := range def.Modules {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("workflow %s module[%d]: %w", def.ID, idx, err)
		}
		instanceID := ref.InstanceID()
		if _, exists := seen[instanceID]; exists {
			return fmt.Errorf("workflow %s: duplicate module instance id %s", def.ID, instanceID)
		}
		seen[instanceID] = struct{}{}
	}
	for key, deps := range def.Graph {
		if _, ok := seen[key]; !ok {
			return fmt.Errorf("workflow %s: graph references unknown module %s", def.ID, key)
		}
		for _, dep := range deps {
			if _, ok := seen[dep]; !ok {
				return fmt.Errorf("workflow %s: graph dependency %s -> %s references unknown module", def.ID, key, dep)
			}
		}
	}
	if err := def.Runtime.validate(); err != nil {
		return fmt.Errorf("workflow %s runtime: %w", def.ID, err)
	}
	return nil
}

// Normalized clones the definition, merges any inline module dependencies into
// the graph, and validates the result.
func (def WorkflowDefinition) Normalized() (WorkflowDefinition, error) {
	clone := def.Clone()
	if clone.Graph == nil {
		clone.Graph = DependencyGraph{}
	}
	for _, ref := range clone.Modules {
		id := ref.InstanceID()
		clone.Graph[id] = mergeDependencies(clone.Graph[id], ref.DependsOn)
	}
	clone.Runtime = clone.Runtime.normalized()
	if err := clone.Validate(); err != nil {
		return WorkflowDefinition{}, err
	}
	return clone, nil
}

// WorkflowRuntimeConfig configures execution constraints for a workflow.
type WorkflowRuntimeConfig struct {
	MaxParallel int `json:"max_parallel,omitempty" yaml:"max_parallel,omitempty"`
}

func (cfg WorkflowRuntimeConfig) normalized() WorkflowRuntimeConfig {
	if cfg.MaxParallel < 0 {
		cfg.MaxParallel = 0
	}
	return cfg
}

func (cfg WorkflowRuntimeConfig) validate() error {
	if cfg.MaxParallel < 0 {
		return fmt.Errorf("max_parallel must be >= 0")
	}
	return nil
}

// ModuleIDs returns the workflow-scoped identifiers in declaration order.
func (def WorkflowDefinition) ModuleIDs() []string {
	ids := make([]string, 0, len(def.Modules))
	for _, ref := range def.Modules {
		ids = append(ids, ref.InstanceID())
	}
	return ids
}

// Dependencies returns the dependency list for a module instance.
func (def WorkflowDefinition) Dependencies(id string) []string {
	if def.Graph == nil {
		return nil
	}
	deps := def.Graph[id]
	if len(deps) == 0 {
		return nil
	}
	clone := make([]string, len(deps))
	copy(clone, deps)
	return clone
}

// ModuleRef describes how a workflow composes and configures a module.
type ModuleRef struct {
	ID          string       `json:"id,omitempty" yaml:"id,omitempty"`
	ModuleID    string       `json:"module" yaml:"module"`
	Name        string       `json:"name,omitempty" yaml:"name,omitempty"`
	Description string       `json:"description,omitempty" yaml:"description,omitempty"`
	DependsOn   []string     `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Config      ModuleConfig `json:"config,omitempty" yaml:"config,omitempty"`
	Optional    bool         `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// Clone returns a deep copy of the module reference.
func (ref ModuleRef) Clone() ModuleRef {
	clone := ModuleRef{
		ID:          ref.ID,
		ModuleID:    ref.ModuleID,
		Name:        ref.Name,
		Description: ref.Description,
		Optional:    ref.Optional,
	}
	if len(ref.DependsOn) > 0 {
		clone.DependsOn = cloneStringSlice(ref.DependsOn)
	}
	if len(ref.Config) > 0 {
		clone.Config = ref.Config.Clone()
	}
	return clone
}

// ModuleConfig carries module-specific overrides (opaque to the runtime).
type ModuleConfig map[string]any

// Clone returns a shallow copy of the config map.
func (cfg ModuleConfig) Clone() ModuleConfig {
	if len(cfg) == 0 {
		return nil
	}
	clone := make(ModuleConfig, len(cfg))
	for key, value := range cfg {
		clone[key] = value
	}
	return clone
}

// InstanceID returns the workflow-local identifier used by dependency graphs.
func (ref ModuleRef) InstanceID() string {
	if ref.ID != "" {
		return ref.ID
	}
	return ref.ModuleID
}

// Validate ensures the reference is usable.
func (ref ModuleRef) Validate() error {
	if ref.ModuleID == "" {
		return fmt.Errorf("workflow: module id is required")
	}
	deps := append([]string{}, ref.DependsOn...)
	sort.Strings(deps)
	for i := 1; i < len(deps); i++ {
		if deps[i] == deps[i-1] {
			return fmt.Errorf("workflow: module %s has duplicate dependency on %s", ref.InstanceID(), deps[i])
		}
	}
	return nil
}

func mergeDependencies(existing, adds []string) []string {
	if len(adds) == 0 && len(existing) == 0 {
		return nil
	}
	set := map[string]struct{}{}
	for _, id := range existing {
		if id == "" {
			continue
		}
		set[id] = struct{}{}
	}
	for _, id := range adds {
		if id == "" {
			continue
		}
		set[id] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	clone := make([]string, len(values))
	copy(clone, values)
	return clone
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}
