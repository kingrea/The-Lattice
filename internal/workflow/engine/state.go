package engine

import (
	"time"

	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/workflow"
	"github.com/kingrea/The-Lattice/internal/workflow/resolver"
	"github.com/kingrea/The-Lattice/internal/workflow/scheduler"
)

// EngineStatus enumerates coarse workflow engine phases.
type EngineStatus string

const (
	EngineStatusUnknown  EngineStatus = "unknown"
	EngineStatusRunning  EngineStatus = "running"
	EngineStatusBlocked  EngineStatus = "blocked"
	EngineStatusComplete EngineStatus = "complete"
	EngineStatusError    EngineStatus = "error"
)

// State captures the persisted snapshot of a workflow run.
type State struct {
	RunID      string                      `json:"run_id"`
	WorkflowID string                      `json:"workflow_id"`
	Definition workflow.WorkflowDefinition `json:"definition"`
	Status     EngineStatus                `json:"status"`
	// StatusReason provides human readable explanation for non-running states.
	StatusReason string                          `json:"status_reason,omitempty"`
	Runtime      EngineRuntime                   `json:"runtime"`
	Nodes        []ModuleStatus                  `json:"nodes"`
	Runnable     []string                        `json:"runnable"`
	Skipped      map[string]scheduler.SkipReason `json:"skipped,omitempty"`
	Runs         map[string]ModuleRun            `json:"runs,omitempty"`
	UpdatedAt    time.Time                       `json:"updated_at"`
}

// EngineRuntime mirrors scheduler constraints that survive across updates.
type EngineRuntime struct {
	Targets     []string                             `json:"targets,omitempty"`
	BatchSize   int                                  `json:"batch_size,omitempty"`
	MaxParallel int                                  `json:"max_parallel,omitempty"`
	Running     []string                             `json:"running,omitempty"`
	ManualGates map[string]scheduler.ManualGateState `json:"manual_gates,omitempty"`
}

// RuntimeOverrides selectively mutates EngineRuntime fields.
type RuntimeOverrides struct {
	Targets     *[]string
	BatchSize   *int
	MaxParallel *int
	Running     *[]string
	ManualGates *map[string]scheduler.ManualGateState
}

// ModuleStatus exposes resolver metadata for a workflow node.
type ModuleStatus struct {
	ID           string                    `json:"id"`
	ModuleID     string                    `json:"module_id"`
	Name         string                    `json:"name"`
	Description  string                    `json:"description,omitempty"`
	Optional     bool                      `json:"optional,omitempty"`
	Concurrency  module.ConcurrencyProfile `json:"concurrency"`
	State        resolver.NodeState        `json:"state"`
	Dependencies []string                  `json:"dependencies,omitempty"`
	Dependents   []string                  `json:"dependents,omitempty"`
	BlockedBy    []string                  `json:"blocked_by,omitempty"`
	Error        string                    `json:"error,omitempty"`
	Artifacts    map[string]ArtifactStatus `json:"artifacts,omitempty"`
	LastRun      *ModuleRun                `json:"last_run,omitempty"`
}

// ArtifactStatus mirrors resolver artifact evaluation for UI/state consumers.
type ArtifactStatus struct {
	ID                  string                `json:"id"`
	Status              module.ArtifactStatus `json:"status"`
	ExpectedFingerprint string                `json:"expected_fingerprint,omitempty"`
	StoredFingerprint   string                `json:"stored_fingerprint,omitempty"`
	Error               string                `json:"error,omitempty"`
}

// ModuleRun persists the last known runtime result for a module execution.
type ModuleRun struct {
	Status     module.Status `json:"status"`
	Message    string        `json:"message,omitempty"`
	Error      string        `json:"error,omitempty"`
	FinishedAt time.Time     `json:"finished_at"`
}

// schedulerRequest converts EngineRuntime into a scheduler request payload.
func (rt EngineRuntime) schedulerRequest() scheduler.RunnableRequest {
	return scheduler.RunnableRequest{
		Targets:     cloneStrings(rt.Targets),
		BatchSize:   rt.BatchSize,
		MaxParallel: rt.MaxParallel,
		Running:     cloneStrings(rt.Running),
		ManualGates: cloneManualGates(rt.ManualGates),
	}
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func cloneManualGates(values map[string]scheduler.ManualGateState) map[string]scheduler.ManualGateState {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]scheduler.ManualGateState, len(values))
	for id, state := range values {
		out[id] = state
	}
	return out
}

func (rt EngineRuntime) clone() EngineRuntime {
	return EngineRuntime{
		Targets:     cloneStrings(rt.Targets),
		BatchSize:   rt.BatchSize,
		MaxParallel: rt.MaxParallel,
		Running:     cloneStrings(rt.Running),
		ManualGates: cloneManualGates(rt.ManualGates),
	}
}
