package module

import (
	"fmt"
	"sort"
	"sync"
)

// Config represents module-specific configuration (opaque to the runtime).
type Config map[string]any

// Factory constructs a module with the provided configuration.
type Factory func(Config) (Module, error)

// Registry maintains known module factories.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: map[string]Factory{}}
}

// Register installs a module factory. Returns an error if the ID already exists.
func (r *Registry) Register(id string, factory Factory) error {
	if id == "" {
		return fmt.Errorf("module: id is required")
	}
	if factory == nil {
		return fmt.Errorf("module: factory is required for %s", id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[id]; exists {
		return fmt.Errorf("module: %s already registered", id)
	}
	r.factories[id] = factory
	return nil
}

// MustRegister panics if registration fails.
func (r *Registry) MustRegister(id string, factory Factory) {
	if err := r.Register(id, factory); err != nil {
		panic(err)
	}
}

// Resolve constructs a module by ID.
func (r *Registry) Resolve(id string, cfg Config) (Module, error) {
	r.mu.RLock()
	factory, ok := r.factories[id]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("module: unknown id %s", id)
	}
	module, err := factory(cfg)
	if err != nil {
		return nil, err
	}
	if err := module.Info().Validate(); err != nil {
		return nil, err
	}
	return module, nil
}

// IDs returns a sorted list of registered module identifiers.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.factories))
	for id := range r.factories {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
