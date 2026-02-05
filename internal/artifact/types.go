// Package artifact defines the filesystem-level contracts (inputs/outputs)
// that modules exchange. Each artifact has a stable identifier, kind, and a
// resolver that maps to the actual path within the project's .lattice tree.

package artifact

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/kingrea/The-Lattice/internal/workflow"
)

// Kind captures the storage shape and serialization format for an artifact.
type Kind string

const (
	// KindDocument represents a markdown-like text document with YAML frontmatter.
	KindDocument Kind = "document"
	// KindJSON represents a JSON document enriched with a _lattice metadata block.
	KindJSON Kind = "json"
	// KindMarker represents an empty file used as a marker/flag.
	KindMarker Kind = "marker"
	// KindDirectory represents a directory that must exist.
	KindDirectory Kind = "directory"
)

// PathResolver returns the fully-qualified path to an artifact for the current workflow run.
type PathResolver func(*workflow.Workflow) string

// ArtifactRef declares a stable identifier and metadata for an artifact.
type ArtifactRef struct {
	ID          string
	Name        string
	Description string
	Kind        Kind
	Optional    bool
	path        PathResolver
}

// Path resolves the artifact path for the provided workflow instance.
func (r ArtifactRef) Path(wf *workflow.Workflow) string {
	if wf == nullWorkflow || r.path == nil {
		return ""
	}
	return filepath.Clean(r.path(wf))
}

// Validate ensures the reference is well-formed.
func (r ArtifactRef) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("artifact: id is required")
	}
	if r.Kind == "" {
		return fmt.Errorf("artifact: kind is required for %s", r.ID)
	}
	if r.path == nil {
		return fmt.Errorf("artifact: path resolver missing for %s", r.ID)
	}
	return nil
}

var nullWorkflow *workflow.Workflow

// Metadata captures provenance stored inside artifact frontmatter or metadata blocks.
type Metadata struct {
	ArtifactID string
	ModuleID   string
	Version    string
	Workflow   string
	Inputs     []string
	CreatedAt  time.Time
	Checksum   string
	Notes      map[string]string
}

// WithDefaults ensures metadata carries the artifact ID and timestamps.
func (m Metadata) WithDefaults(ref ArtifactRef, now time.Time) Metadata {
	clone := m
	if clone.ArtifactID == "" {
		clone.ArtifactID = ref.ID
	}
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = now.UTC()
	} else {
		clone.CreatedAt = clone.CreatedAt.UTC()
	}
	return clone
}

// ValidateFor ensures metadata matches the artifact contract.
func (m Metadata) ValidateFor(ref ArtifactRef) error {
	if m.ArtifactID != ref.ID {
		return fmt.Errorf("artifact: metadata id %s does not match ref %s", m.ArtifactID, ref.ID)
	}
	if m.ModuleID == "" {
		return fmt.Errorf("artifact: module id is required for %s", ref.ID)
	}
	if m.Version == "" {
		return fmt.Errorf("artifact: version is required for %s", ref.ID)
	}
	return nil
}

// State captures the readiness of an artifact on disk.
type State string

const (
	StateMissing State = "missing"
	StateReady   State = "ready"
	StateInvalid State = "invalid"
	StateError   State = "error"
)

// CheckResult captures ArtifactStore.Check results.
type CheckResult struct {
	Ref      ArtifactRef
	Path     string
	State    State
	Metadata *Metadata
	Err      error
}

// helper to register global references
func register(ref ArtifactRef) ArtifactRef {
	if refs == nil {
		refs = map[string]ArtifactRef{}
	}
	refs[ref.ID] = ref
	return ref
}

var refs map[string]ArtifactRef

// Lookup returns a registered artifact reference by ID.
func Lookup(id string) (ArtifactRef, bool) {
	ref, ok := refs[id]
	return ref, ok
}

// newDocRef creates a markdown document reference helper.
func newDocRef(id, name, desc string, resolver PathResolver) ArtifactRef {
	return ArtifactRef{
		ID:          id,
		Name:        name,
		Description: desc,
		Kind:        KindDocument,
		path:        resolver,
	}
}

// newJSONRef creates a JSON artifact reference helper.
func newJSONRef(id, name, desc string, resolver PathResolver) ArtifactRef {
	return ArtifactRef{
		ID:          id,
		Name:        name,
		Description: desc,
		Kind:        KindJSON,
		path:        resolver,
	}
}

// newMarkerRef creates a marker file reference helper.
func newMarkerRef(id, name, desc string, resolver PathResolver) ArtifactRef {
	return ArtifactRef{
		ID:          id,
		Name:        name,
		Description: desc,
		Kind:        KindMarker,
		path:        resolver,
	}
}

// newDirectoryRef creates a directory reference helper.
func newDirectoryRef(id, name, desc string, resolver PathResolver) ArtifactRef {
	return ArtifactRef{
		ID:          id,
		Name:        name,
		Description: desc,
		Kind:        KindDirectory,
		path:        resolver,
	}
}

// Canonical artifact references for the commission workflow.
var (
	CommissionDoc   = register(newDocRef("commission-doc", "Commission Brief", "Primary intake describing the commission", func(wf *workflow.Workflow) string { return wf.CommissionPath() }))
	ArchitectureDoc = register(newDocRef("architecture-doc", "Architecture Guide", "Technical architecture generated during planning", func(wf *workflow.Workflow) string { return wf.ArchitecturePath() }))
	ConventionsDoc  = register(newDocRef("conventions-doc", "Conventions", "Team conventions and agreements", func(wf *workflow.Workflow) string { return wf.ConventionsPath() }))

	ModulesDoc     = register(newDocRef("modules-doc", "Modules Specification", "MODULES.md describing units of work", func(wf *workflow.Workflow) string { return wf.ModulesPath() }))
	ActionPlanDoc  = register(newDocRef("action-plan", "Action Plan", "PLAN.md describing the execution plan", func(wf *workflow.Workflow) string { return wf.ActionPlanPath() }))
	StaffReviewDoc = register(newDocRef("staff-review", "Staff Review", "STAFF_REVIEW.md with orchestrator feedback", func(wf *workflow.Workflow) string { return wf.StaffReviewPath() }))

	ReviewPragmatistDoc = register(newDocRef("review-pragmatist", "Pragmatist Review", "Expert review focused on feasibility", func(wf *workflow.Workflow) string { return wf.ReviewPragmatistPath() }))
	ReviewSimplifierDoc = register(newDocRef("review-simplifier", "Simplifier Review", "Expert review focused on DX and simplicity", func(wf *workflow.Workflow) string { return wf.ReviewSimplifierPath() }))
	ReviewAdvocateDoc   = register(newDocRef("review-advocate", "User Advocate Review", "Expert review focused on user value", func(wf *workflow.Workflow) string { return wf.ReviewAdvocatePath() }))
	ReviewSkepticDoc    = register(newDocRef("review-skeptic", "Skeptic Review", "Expert review stress-testing risks", func(wf *workflow.Workflow) string { return wf.ReviewSkepticPath() }))
	StakeholdersJSON    = register(newJSONRef("stakeholders-json", "Stakeholders Manifest", "stakeholders.json describing refinement reviewer assignments", func(wf *workflow.Workflow) string {
		return filepath.Join(wf.TeamDir(), "stakeholders.json")
	}))
	AuditDirectory = register(newDirectoryRef("audit-dir", "Audit Directory", "workflow/audit folder storing stakeholder audits", func(wf *workflow.Workflow) string {
		return filepath.Join(wf.Dir(), "audit")
	}))
	AuditSynthesisDoc = register(newDocRef("audit-synthesis", "Audit Synthesis Summary", "SYNTHESIS.md summarizing stakeholder audits", func(wf *workflow.Workflow) string {
		return filepath.Join(wf.Dir(), "audit", "SYNTHESIS.md")
	}))

	OrchestratorState    = register(newJSONRef("orchestrator-state", "Orchestrator State", "orchestrator.json describing current orchestrator configuration", func(wf *workflow.Workflow) string { return wf.OrchestratorPath() }))
	WorkersJSON          = register(newJSONRef("workers-json", "Workers Roster", "workers.json with currently hired agents", func(wf *workflow.Workflow) string { return wf.WorkersPath() }))
	WorkLogDoc           = register(newDocRef("work-log", "Work Log", "work-log.md chronologically tracking progress", func(wf *workflow.Workflow) string { return wf.WorkLogPath() }))
	WorkTasksDoc         = register(newDocRef("work-tasks", "Work Tasks", "TASKS.md enumerating prepared work sessions", func(wf *workflow.Workflow) string { return filepath.Join(wf.WorkDir(), "TASKS.md") }))
	WorkInProgressMarker = register(newMarkerRef("work-in-progress", "Work In Progress Marker", "Marker created when a work cycle is actively staging or executing", func(wf *workflow.Workflow) string {
		return filepath.Join(wf.WorkDir(), workflow.MarkerWorkInProgress)
	}))
	WorkCompleteMarker = register(newMarkerRef("work-complete", "Work Complete Marker", "Marker written after down-cycle cleanup finishes", func(wf *workflow.Workflow) string {
		return filepath.Join(wf.WorkDir(), workflow.MarkerWorkComplete)
	}))
	RefinementNeededMarker = register(newMarkerRef("refinement-needed", "Refinement Needed Marker", "Marker emitted when no ready beads remain and refinement must run", func(wf *workflow.Workflow) string {
		return filepath.Join(wf.WorkDir(), workflow.MarkerRefinementNeeded)
	}))

	ReviewsAppliedMarker = register(newMarkerRef("reviews-applied", "Reviews Applied Marker", "Marker file set when review feedback was incorporated", func(wf *workflow.Workflow) string { return wf.ReviewsAppliedPath() }))
	StaffFeedbackApplied = register(newMarkerRef("staff-feedback-applied", "Staff Feedback Applied Marker", "Marker after staff feedback incorporation", func(wf *workflow.Workflow) string { return wf.StaffFeedbackAppliedPath() }))
	BeadsCreatedMarker   = register(newMarkerRef("beads-created", "Beads Created Marker", "Marker created after beads are generated", func(wf *workflow.Workflow) string { return wf.BeadsCreatedPath() }))

	ReleaseNotesDoc = register(newDocRef("release-notes", "Release Notes", "RELEASE_NOTES.md summarizing shipped work", func(wf *workflow.Workflow) string {
		return filepath.Join(wf.ReleaseDir(), "RELEASE_NOTES.md")
	}))
	ReleasePackagesDir = register(newDirectoryRef("release-packages", "Release Packages Directory", "workflow/release/packages folder storing archived bundles", func(wf *workflow.Workflow) string {
		return filepath.Join(wf.ReleaseDir(), "packages")
	}))
	AgentsReleasedMarker = register(newMarkerRef("agents-released", "Agents Released Marker", "Marker created when worker agents are released", func(wf *workflow.Workflow) string {
		return filepath.Join(wf.ReleaseDir(), workflow.MarkerAgentsReleased)
	}))
	CleanupDoneMarker = register(newMarkerRef("cleanup-done", "Cleanup Done Marker", "Marker created after post-work cleanup completes", func(wf *workflow.Workflow) string {
		return filepath.Join(wf.ReleaseDir(), workflow.MarkerCleanupDone)
	}))
	OrchestratorReleasedMarker = register(newMarkerRef("orchestrator-released", "Orchestrator Released Marker", "Marker created when orchestrator cleanup is finished", func(wf *workflow.Workflow) string {
		return filepath.Join(wf.ReleaseDir(), workflow.MarkerOrchestratorReleased)
	}))
)
