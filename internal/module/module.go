package module

import (
	"fmt"

	"github.com/kingrea/The-Lattice/internal/artifact"
)

// Info describes a module's identity and intent.
type Info struct {
	ID          string
	Name        string
	Description string
	Version     string
	Concurrency ConcurrencyProfile
}

// Validate ensures the info block is well-formed.
func (i Info) Validate() error {
	if i.ID == "" {
		return fmt.Errorf("module: id is required")
	}
	if i.Name == "" {
		return fmt.Errorf("module: name is required for %s", i.ID)
	}
	if i.Version == "" {
		return fmt.Errorf("module: version is required for %s", i.ID)
	}
	if err := i.Concurrency.validate(i.ID); err != nil {
		return err
	}
	return nil
}

// ConcurrencyProfile declares how many scheduler slots a module consumes and
// whether it requires exclusive execution.
type ConcurrencyProfile struct {
	// Slots describes how many scheduler capacity units are required to execute
	// the module. Zero or negative values default to one slot.
	Slots int
	// Exclusive forces the module to run without any other modules occupying the
	// workflow engine. Useful for resources that cannot be shared safely.
	Exclusive bool
}

func (p ConcurrencyProfile) slotsOrDefault() int {
	if p.Slots <= 0 {
		return 1
	}
	return p.Slots
}

func (p ConcurrencyProfile) validate(moduleID string) error {
	if p.Slots < 0 {
		return fmt.Errorf("module: concurrency slots must be >= 0 for %s", moduleID)
	}
	return nil
}

// SlotCost returns how many scheduler slots the module consumes simultaneously.
func (i Info) SlotCost() int {
	return i.Concurrency.slotsOrDefault()
}

// RequiresExclusiveExecution reports whether the module must run without other
// concurrent modules.
func (i Info) RequiresExclusiveExecution() bool {
	return i.Concurrency.Exclusive
}

// Result captures the outcome of a module execution.
type Result struct {
	Status  Status
	Message string
}

// Status enumerates module run outcomes.
type Status string

const (
	StatusCompleted  Status = "completed"
	StatusNoOp       Status = "no-op"
	StatusNeedsInput Status = "needs-input"
	StatusFailed     Status = "failed"
)

// Module is implemented by every runtime unit.
type Module interface {
	Info() Info
	Inputs() []artifact.ArtifactRef
	Outputs() []artifact.ArtifactRef
	IsComplete(ctx *ModuleContext) (bool, error)
	Run(ctx *ModuleContext) (Result, error)
}
