// internal/workflow/phase.go
//
// Phase detection for the commission-work workflow.
// The workflow state is determined by examining files in .lattice/workflow/
// This makes state git-trackable and crash-recoverable.

package workflow

import (
	"os"
	"path/filepath"
)

// Phase represents a stage in the commission-work workflow
type Phase int

const (
	PhaseNone Phase = iota
	PhasePlanning
	PhaseOrchestratorSelection
	PhaseHiring
	PhaseWorkProcess
	PhaseRefinement
	PhaseAgentRelease
	PhaseWorkCleanup
	PhaseOrchestratorRelease
	PhaseComplete
)

// String returns a human-readable name for the phase
func (p Phase) String() string {
	switch p {
	case PhaseNone:
		return "Not Started"
	case PhasePlanning:
		return "Planning"
	case PhaseOrchestratorSelection:
		return "Orchestrator Selection"
	case PhaseHiring:
		return "Hiring"
	case PhaseWorkProcess:
		return "Work In Progress"
	case PhaseRefinement:
		return "Refinement"
	case PhaseAgentRelease:
		return "Agent Release"
	case PhaseWorkCleanup:
		return "Work Cleanup"
	case PhaseOrchestratorRelease:
		return "Orchestrator Release"
	case PhaseComplete:
		return "Complete"
	default:
		return "Unknown"
	}
}

// FriendlyName returns a short description suitable for menu display
func (p Phase) FriendlyName() string {
	switch p {
	case PhasePlanning:
		return "Planning Session"
	case PhaseOrchestratorSelection:
		return "Select Orchestrator"
	case PhaseHiring:
		return "Hiring Workers"
	case PhaseWorkProcess:
		return "Work In Progress"
	case PhaseRefinement:
		return "Refinement"
	case PhaseAgentRelease:
		return "Releasing Agents"
	case PhaseWorkCleanup:
		return "Cleanup"
	case PhaseOrchestratorRelease:
		return "Final Release"
	default:
		return p.String()
	}
}

// Next returns the next phase in the workflow
func (p Phase) Next() Phase {
	if p >= PhaseComplete {
		return PhaseComplete
	}
	return p + 1
}

// IsTerminal returns true if this phase represents workflow completion
func (p Phase) IsTerminal() bool {
	return p == PhaseComplete || p == PhaseNone
}

// IsResumable returns true if this phase can be resumed
func (p Phase) IsResumable() bool {
	return p > PhaseNone && p < PhaseComplete
}

// DetectPhase examines the workflow directory to determine current phase.
// It checks for marker files in reverse order (most complete first).
func DetectPhase(workflowDir string) Phase {
	// workflowDir is .lattice/workflow, we need .lattice for plan/action dirs
	latticeDir := filepath.Dir(workflowDir)
	planDir := filepath.Join(latticeDir, PlanDir)
	actionDir := filepath.Join(latticeDir, ActionDir)

	// Check in reverse order - most complete state first
	if fileExists(workflowDir, ReleaseDir, MarkerOrchestratorReleased) {
		return PhaseComplete
	}
	if fileExists(workflowDir, ReleaseDir, MarkerCleanupDone) {
		return PhaseOrchestratorRelease
	}
	if fileExists(workflowDir, ReleaseDir, MarkerAgentsReleased) {
		return PhaseWorkCleanup
	}
	if fileExists(workflowDir, WorkDir, MarkerRefinementNeeded) {
		return PhaseRefinement
	}
	if fileExists(workflowDir, WorkDir, MarkerWorkComplete) {
		return PhaseAgentRelease
	}
	if fileExists(workflowDir, TeamDir, FileWorkers) {
		return PhaseWorkProcess
	}
	if fileExists(workflowDir, FileOrchestrator) {
		return PhaseHiring
	}
	// Planning is complete when action/MODULES.md and action/PLAN.md exist
	if fileExists(actionDir, FileModules) && fileExists(actionDir, FileActionPlan) {
		return PhaseOrchestratorSelection
	}
	// Check if we're in the middle of planning (anchor docs exist but action plan doesn't)
	if fileExists(planDir, FileCommission) || fileExists(planDir, FileArchitecture) || fileExists(planDir, FileConventions) {
		return PhasePlanning
	}
	// Check if workflow directory exists at all with any content
	if dirHasContent(workflowDir) || dirHasContent(planDir) || dirHasContent(actionDir) {
		return PhasePlanning
	}
	return PhaseNone
}

// fileExists checks if a file exists at the given path segments
func fileExists(parts ...string) bool {
	path := filepath.Join(parts...)
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirHasContent reports true only when the directory contains at least one
// regular file. We intentionally ignore nested directories that `InitLatticeDir`
// creates so a freshly initialized project still reports PhaseNone until real
// workflow artifacts exist.
func dirHasContent(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.Type().IsRegular() {
			return true
		}
	}
	return false
}
