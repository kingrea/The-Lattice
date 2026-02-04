package consolidation

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/module"
)

const (
	moduleID      = "consolidation"
	moduleVersion = "1.0.0"
)

// ConsolidationModule launches the orchestrator session that synthesizes all
// reviewer feedback into MODULES.md and PLAN.md.
type ConsolidationModule struct {
	*module.Base
	windowName string
}

// Register installs the module factory.
func Register(reg *module.Registry) {
	if reg == nil {
		return
	}
	reg.MustRegister(moduleID, func(module.Config) (module.Module, error) {
		return New(), nil
	})
}

// New configures the module description and IO contracts.
func New() *ConsolidationModule {
	info := module.Info{
		ID:          moduleID,
		Name:        "Consolidate Reviews",
		Description: "Synthesizes all reviewer feedback into the plan and sets the reviews-applied marker.",
		Version:     moduleVersion,
	}
	base := module.NewBase(info)
	base.SetInputs(
		artifact.CommissionDoc,
		artifact.ArchitectureDoc,
		artifact.ConventionsDoc,
		artifact.ModulesDoc,
		artifact.ActionPlanDoc,
		artifact.StaffReviewDoc,
		artifact.ReviewPragmatistDoc,
		artifact.ReviewSimplifierDoc,
		artifact.ReviewAdvocateDoc,
		artifact.ReviewSkepticDoc,
	)
	base.SetOutputs(
		artifact.ModulesDoc,
		artifact.ActionPlanDoc,
		artifact.ReviewsAppliedMarker,
	)
	return &ConsolidationModule{Base: &base}
}

// Run validates prerequisites and launches the tmux session when needed.
func (m *ConsolidationModule) Run(ctx *module.ModuleContext) (module.Result, error) {
	if err := validateContext(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if missing, err := m.missingInput(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	} else if missing != "" {
		return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("waiting for %s", missing)}, nil
	}
	if complete, err := m.IsComplete(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	} else if complete {
		return module.Result{Status: module.StatusNoOp, Message: "consolidation already complete"}, nil
	}
	if m.windowName != "" {
		return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("consolidation running in %s", m.windowName)}, nil
	}
	window := fmt.Sprintf("consolidation-%d", time.Now().Unix())
	if err := createTmuxWindow(window, ctx.Config.ProjectDir); err != nil {
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("consolidation: create tmux window: %w", err)
	}
	prompt := fmt.Sprintf(
		"You are the ORCHESTRATOR consolidating feedback from multiple reviewers. "+
			"Read all documents: "+
			"- Original plan: %s (COMMISSION.md, ARCHITECTURE.md, CONVENTIONS.md) "+
			"- Action plan: %s (MODULES.md, PLAN.md) "+
			"- Staff review: %s/STAFF_REVIEW.md "+
			"- Pragmatist review: %s/REVIEW_PRAGMATIST.md "+
			"- Simplifier review: %s/REVIEW_SIMPLIFIER.md "+
			"- User Advocate review: %s/REVIEW_USER_ADVOCATE.md "+
			"- Skeptic review: %s/REVIEW_SKEPTIC.md "+
			"Synthesize the feedback, update MODULES.md and PLAN.md, and capture what was applied vs. deferred. "+
			"When done, create an empty marker file at %s to signal completion. Do not end until the marker exists.",
		ctx.Workflow.PlanDir(),
		ctx.Workflow.ActionDir(),
		ctx.Workflow.ActionDir(),
		ctx.Workflow.ActionDir(),
		ctx.Workflow.ActionDir(),
		ctx.Workflow.ActionDir(),
		ctx.Workflow.ActionDir(),
		ctx.Workflow.ReviewsAppliedPath(),
	)
	if err := runOpenCode(window, prompt); err != nil {
		killTmuxWindow(window)
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("consolidation: launch opencode: %w", err)
	}
	m.windowName = window
	return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("consolidation running in %s", window)}, nil
}

// IsComplete returns true when the marker exists and plan docs were stamped.
func (m *ConsolidationModule) IsComplete(ctx *module.ModuleContext) (bool, error) {
	if err := validateContext(ctx); err != nil {
		return false, err
	}
	markerReady, err := m.markerReady(ctx)
	if err != nil {
		return false, err
	}
	if markerReady {
		m.stopSession()
		return true, nil
	}
	for _, ref := range []artifact.ArtifactRef{artifact.ModulesDoc, artifact.ActionPlanDoc} {
		ready, err := m.ensureDocument(ctx, ref)
		if err != nil {
			return false, err
		}
		if !ready {
			return false, nil
		}
	}
	return false, nil
}

func (m *ConsolidationModule) missingInput(ctx *module.ModuleContext) (string, error) {
	for _, ref := range m.Inputs() {
		result, err := ctx.Artifacts.Check(ref)
		if err != nil {
			return "", fmt.Errorf("consolidation: check %s: %w", ref.ID, err)
		}
		if result.State != artifact.StateReady {
			return ref.Name, nil
		}
	}
	return "", nil
}

func (m *ConsolidationModule) ensureDocument(ctx *module.ModuleContext, ref artifact.ArtifactRef) (bool, error) {
	result, err := ctx.Artifacts.Check(ref)
	if err != nil {
		return false, fmt.Errorf("consolidation: check %s: %w", ref.ID, err)
	}
	switch result.State {
	case artifact.StateReady:
		if result.Metadata == nil || result.Metadata.ModuleID != moduleID || result.Metadata.Version != moduleVersion {
			if err := m.writeMetadata(ctx, ref); err != nil {
				return false, err
			}
			return false, nil
		}
		return true, nil
	case artifact.StateMissing:
		return false, nil
	case artifact.StateInvalid:
		if err := m.writeMetadata(ctx, ref); err != nil {
			return false, err
		}
		return false, nil
	case artifact.StateError:
		if result.Err != nil {
			return false, fmt.Errorf("consolidation: %s: %w", ref.ID, result.Err)
		}
		return false, fmt.Errorf("consolidation: %s encountered an unknown error", ref.ID)
	default:
		return false, nil
	}
}

func (m *ConsolidationModule) writeMetadata(ctx *module.ModuleContext, ref artifact.ArtifactRef) error {
	path := ref.Path(ctx.Workflow)
	if path == "" {
		return fmt.Errorf("consolidation: unable to resolve path for %s", ref.ID)
	}
	body, err := readDocumentBody(path)
	if err != nil {
		return fmt.Errorf("consolidation: read %s: %w", ref.ID, err)
	}
	meta := artifact.Metadata{
		ArtifactID: ref.ID,
		ModuleID:   moduleID,
		Version:    moduleVersion,
		Workflow:   ctx.Workflow.Dir(),
		Inputs: []string{
			artifact.CommissionDoc.ID,
			artifact.ArchitectureDoc.ID,
			artifact.ConventionsDoc.ID,
			artifact.ModulesDoc.ID,
			artifact.ActionPlanDoc.ID,
			artifact.StaffReviewDoc.ID,
			artifact.ReviewPragmatistDoc.ID,
			artifact.ReviewSimplifierDoc.ID,
			artifact.ReviewAdvocateDoc.ID,
			artifact.ReviewSkepticDoc.ID,
		},
	}
	if err := ctx.Artifacts.Write(ref, body, meta); err != nil {
		return fmt.Errorf("consolidation: write %s: %w", ref.ID, err)
	}
	return nil
}

func (m *ConsolidationModule) markerReady(ctx *module.ModuleContext) (bool, error) {
	result, err := ctx.Artifacts.Check(artifact.ReviewsAppliedMarker)
	if err != nil {
		return false, fmt.Errorf("consolidation: check marker: %w", err)
	}
	switch result.State {
	case artifact.StateReady:
		return true, nil
	case artifact.StateMissing:
		return false, nil
	case artifact.StateInvalid:
		if err := ctx.Artifacts.Write(artifact.ReviewsAppliedMarker, nil, artifact.Metadata{ArtifactID: artifact.ReviewsAppliedMarker.ID, ModuleID: moduleID, Version: moduleVersion, Workflow: ctx.Workflow.Dir()}); err != nil {
			return false, fmt.Errorf("consolidation: rewrite marker: %w", err)
		}
		return false, nil
	case artifact.StateError:
		if result.Err != nil {
			return false, fmt.Errorf("consolidation: marker error: %w", result.Err)
		}
		return false, fmt.Errorf("consolidation: marker encountered unknown error")
	default:
		return false, nil
	}
}

func (m *ConsolidationModule) stopSession() {
	if m.windowName == "" {
		return
	}
	killTmuxWindow(m.windowName)
	m.windowName = ""
}

func validateContext(ctx *module.ModuleContext) error {
	if ctx == nil {
		return fmt.Errorf("consolidation: context is nil")
	}
	if ctx.Config == nil {
		return fmt.Errorf("consolidation: config is required")
	}
	if ctx.Workflow == nil {
		return fmt.Errorf("consolidation: workflow is required")
	}
	if ctx.Artifacts == nil {
		return fmt.Errorf("consolidation: artifact store is required")
	}
	return nil
}

func readDocumentBody(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	if _, body, err := artifact.ParseFrontMatter(data); err == nil {
		return body, nil
	}
	return data, nil
}

func createTmuxWindow(name, dir string) error {
	args := []string{"new-window", "-n", name}
	if strings.TrimSpace(dir) != "" {
		args = append(args, "-c", dir)
	}
	cmd := exec.Command("tmux", args...)
	return cmd.Run()
}

func killTmuxWindow(name string) {
	if name == "" {
		return
	}
	_ = exec.Command("tmux", "kill-window", "-t", name).Run()
}

func runOpenCode(window, prompt string) error {
	escaped := strings.ReplaceAll(prompt, "\"", `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", " ")
	cmd := exec.Command("tmux", "send-keys", "-t", window, fmt.Sprintf(`opencode --prompt "%s"`, escaped), "Enter")
	return cmd.Run()
}
