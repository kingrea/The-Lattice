package module

import (
	"fmt"

	"github.com/yourusername/lattice/internal/artifact"
)

// Info describes a module's identity and intent.
type Info struct {
	ID          string
	Name        string
	Description string
	Version     string
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
	return nil
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
	Run(ctx *ModuleContext) (Result, error)
}
