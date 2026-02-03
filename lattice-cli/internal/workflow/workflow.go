// internal/workflow/workflow.go
//
// Defines the workflow directory structure and file constants.
// All workflow state is stored in .lattice/workflow/ for git tracking.

package workflow

import (
	"os"
	"path/filepath"
)

// Directory names within .lattice/
const (
	WorkflowDir = "workflow"
	PlanDir     = "plan"
	ActionDir   = "action"
	TeamDir     = "team"
	WorkDir     = "work"
	ReleaseDir  = "release"
)

// File names for workflow artifacts
const (
	FilePlan         = "plan.md"
	FileOrchestrator = "orchestrator.json"
	FileWorkers      = "workers.json"
	FileWorkLog      = "work-log.md"
)

// Planning phase output files (in .lattice/plan/)
const (
	FileCommission   = "COMMISSION.md"
	FileArchitecture = "ARCHITECTURE.md"
	FileConventions  = "CONVENTIONS.md"
)

// Action phase output files (in .lattice/action/)
const (
	FileModules    = "MODULES.md"
	FileActionPlan = "PLAN.md"
)

// Review files (in .lattice/action/)
const (
	FileStaffReview          = "STAFF_REVIEW.md"
	FileReviewPragmatist     = "REVIEW_PRAGMATIST.md"
	FileReviewSimplifier     = "REVIEW_SIMPLIFIER.md"
	FileReviewAdvocate       = "REVIEW_USER_ADVOCATE.md"
	FileReviewSkeptic        = "REVIEW_SKEPTIC.md"
	FileStaffFeedbackApplied = ".staff-feedback-applied" // Marker that staff feedback has been applied to the plan
	FilePlanChatReady        = ".plan-chat-ready"        // Marker that the planning chat concluded and the user is ready
	FilePlanChatActive       = ".plan-chat-active"       // Marker that a planning chat session is active
	FileReviewsApplied       = ".reviews-applied"        // Marker that orchestrator applied feedback
)

// Beads tracking (in .lattice/action/)
const (
	FileBeadsCreated = ".beads-created" // Marker that beads were created from modules/plan
)

// Marker files (empty files that signal phase completion)
const (
	MarkerHiringComplete       = ".hiring-complete"
	MarkerWorkInProgress       = ".in-progress"
	MarkerWorkComplete         = ".complete"
	MarkerRefinementNeeded     = ".refinement-needed"
	MarkerAgentsReleased       = ".agents-released"
	MarkerCleanupDone          = ".cleanup-done"
	MarkerOrchestratorReleased = ".orchestrator-released"
)

// Workflow manages the workflow directory structure
type Workflow struct {
	// Base path to .lattice directory
	latticeDir string
}

// New creates a new Workflow manager
func New(latticeDir string) *Workflow {
	return &Workflow{
		latticeDir: latticeDir,
	}
}

// Dir returns the base workflow directory path
func (w *Workflow) Dir() string {
	return filepath.Join(w.latticeDir, WorkflowDir)
}

// PlanDir returns the path to the plan directory (.lattice/plan/)
func (w *Workflow) PlanDir() string {
	return filepath.Join(w.latticeDir, PlanDir)
}

// ActionDir returns the path to the action directory (.lattice/action/)
func (w *Workflow) ActionDir() string {
	return filepath.Join(w.latticeDir, ActionDir)
}

// PlanPath returns the path to plan.md (legacy)
func (w *Workflow) PlanPath() string {
	return filepath.Join(w.Dir(), FilePlan)
}

// CommissionPath returns the path to COMMISSION.md
func (w *Workflow) CommissionPath() string {
	return filepath.Join(w.PlanDir(), FileCommission)
}

// ArchitecturePath returns the path to ARCHITECTURE.md
func (w *Workflow) ArchitecturePath() string {
	return filepath.Join(w.PlanDir(), FileArchitecture)
}

// ConventionsPath returns the path to CONVENTIONS.md
func (w *Workflow) ConventionsPath() string {
	return filepath.Join(w.PlanDir(), FileConventions)
}

// ModulesPath returns the path to MODULES.md
func (w *Workflow) ModulesPath() string {
	return filepath.Join(w.ActionDir(), FileModules)
}

// ActionPlanPath returns the path to action/PLAN.md
func (w *Workflow) ActionPlanPath() string {
	return filepath.Join(w.ActionDir(), FileActionPlan)
}

// StaffReviewPath returns the path to STAFF_REVIEW.md
func (w *Workflow) StaffReviewPath() string {
	return filepath.Join(w.ActionDir(), FileStaffReview)
}

// ReviewPragmatistPath returns the path to REVIEW_PRAGMATIST.md
func (w *Workflow) ReviewPragmatistPath() string {
	return filepath.Join(w.ActionDir(), FileReviewPragmatist)
}

// ReviewSimplifierPath returns the path to REVIEW_SIMPLIFIER.md
func (w *Workflow) ReviewSimplifierPath() string {
	return filepath.Join(w.ActionDir(), FileReviewSimplifier)
}

// ReviewAdvocatePath returns the path to REVIEW_USER_ADVOCATE.md
func (w *Workflow) ReviewAdvocatePath() string {
	return filepath.Join(w.ActionDir(), FileReviewAdvocate)
}

// ReviewSkepticPath returns the path to REVIEW_SKEPTIC.md
func (w *Workflow) ReviewSkepticPath() string {
	return filepath.Join(w.ActionDir(), FileReviewSkeptic)
}

// StaffFeedbackAppliedPath returns the marker path after staff feedback is incorporated
func (w *Workflow) StaffFeedbackAppliedPath() string {
	return filepath.Join(w.ActionDir(), FileStaffFeedbackApplied)
}

// ReviewsAppliedPath returns the path to the reviews-applied marker
func (w *Workflow) ReviewsAppliedPath() string {
	return filepath.Join(w.ActionDir(), FileReviewsApplied)
}

// PlanChatReadyPath returns the marker path for concluding a plan chat session
func (w *Workflow) PlanChatReadyPath() string {
	return filepath.Join(w.ActionDir(), FilePlanChatReady)
}

// PlanChatActivePath returns the marker path that signals an active plan chat session
func (w *Workflow) PlanChatActivePath() string {
	return filepath.Join(w.ActionDir(), FilePlanChatActive)
}

// BeadsCreatedPath returns the path to the beads-created marker
func (w *Workflow) BeadsCreatedPath() string {
	return filepath.Join(w.ActionDir(), FileBeadsCreated)
}

// AllReviewsComplete returns true if all 4 reviewer files exist
func (w *Workflow) AllReviewsComplete() bool {
	return fileExistsAt(w.ReviewPragmatistPath()) &&
		fileExistsAt(w.ReviewSimplifierPath()) &&
		fileExistsAt(w.ReviewAdvocatePath()) &&
		fileExistsAt(w.ReviewSkepticPath())
}

// PlanningComplete returns true if all planning outputs exist (including beads created)
func (w *Workflow) PlanningComplete() bool {
	return fileExistsAt(w.CommissionPath()) &&
		fileExistsAt(w.ArchitecturePath()) &&
		fileExistsAt(w.ConventionsPath()) &&
		fileExistsAt(w.ModulesPath()) &&
		fileExistsAt(w.ActionPlanPath()) &&
		fileExistsAt(w.ReviewsAppliedPath()) &&
		fileExistsAt(w.BeadsCreatedPath())
}

// AnchorDocsComplete returns true if the 3 anchor docs from lattice-planning exist
func (w *Workflow) AnchorDocsComplete() bool {
	return fileExistsAt(w.CommissionPath()) &&
		fileExistsAt(w.ArchitecturePath()) &&
		fileExistsAt(w.ConventionsPath())
}

func fileExistsAt(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// OrchestratorPath returns the path to orchestrator.json
func (w *Workflow) OrchestratorPath() string {
	return filepath.Join(w.Dir(), FileOrchestrator)
}

// TeamDir returns the path to the team directory
func (w *Workflow) TeamDir() string {
	return filepath.Join(w.Dir(), TeamDir)
}

// WorkersPath returns the path to workers.json
func (w *Workflow) WorkersPath() string {
	return filepath.Join(w.TeamDir(), FileWorkers)
}

// WorkDir returns the path to the work directory
func (w *Workflow) WorkDir() string {
	return filepath.Join(w.Dir(), WorkDir)
}

// WorkLogPath returns the path to work-log.md
func (w *Workflow) WorkLogPath() string {
	return filepath.Join(w.WorkDir(), FileWorkLog)
}

// ReleaseDir returns the path to the release directory
func (w *Workflow) ReleaseDir() string {
	return filepath.Join(w.Dir(), ReleaseDir)
}

// CurrentPhase detects the current workflow phase
func (w *Workflow) CurrentPhase() Phase {
	return DetectPhase(w.Dir())
}

// Initialize creates the workflow directory structure
func (w *Workflow) Initialize() error {
	dirs := []string{
		w.Dir(),
		w.PlanDir(),
		w.ActionDir(),
		w.TeamDir(),
		w.WorkDir(),
		w.ReleaseDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

// WriteMarker creates an empty marker file
func (w *Workflow) WriteMarker(dir, marker string) error {
	path := filepath.Join(dir, marker)
	return os.WriteFile(path, []byte{}, 0644)
}

// HasMarker checks if a marker file exists
func (w *Workflow) HasMarker(dir, marker string) bool {
	return fileExists(dir, marker)
}

// Reset removes the entire workflow directory (for starting fresh)
func (w *Workflow) Reset() error {
	return os.RemoveAll(w.Dir())
}
