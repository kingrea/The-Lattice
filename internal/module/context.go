package module

import (
	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/config"
	"github.com/yourusername/lattice/internal/logbook"
	"github.com/yourusername/lattice/internal/orchestrator"
	"github.com/yourusername/lattice/internal/workflow"
)

// ModuleContext carries shared runtime dependencies into every module.
type ModuleContext struct {
	Config       *config.Config
	Workflow     *workflow.Workflow
	Orchestrator *orchestrator.Orchestrator
	Logbook      *logbook.Logbook
	Artifacts    *artifact.Store
	OriginMode   string
}

// NewContext builds a ModuleContext with a fresh ArtifactStore.
func NewContext(cfg *config.Config, wf *workflow.Workflow, orch *orchestrator.Orchestrator, lb *logbook.Logbook) *ModuleContext {
	return &ModuleContext{
		Config:       cfg,
		Workflow:     wf,
		Orchestrator: orch,
		Logbook:      lb,
		Artifacts:    artifact.NewStore(wf),
	}
}

// WithArtifacts allows dependency injection of a pre-built store.
func (ctx *ModuleContext) WithArtifacts(store *artifact.Store) *ModuleContext {
	clone := *ctx
	clone.Artifacts = store
	return &clone
}

// WithMode records which Bubble Tea mode triggered the invocation.
func (ctx *ModuleContext) WithMode(name string) *ModuleContext {
	clone := *ctx
	clone.OriginMode = name
	return &clone
}
