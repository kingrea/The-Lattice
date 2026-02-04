package staff_review

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
	moduleID      = "staff-review"
	moduleVersion = "1.0.0"
)

// StaffReviewModule launches the Staff Engineer review session and tracks the
// resulting artifact.
type StaffReviewModule struct {
	*module.Base
	windowName string
}

// Register installs the module factory for runtime usage.
func Register(reg *module.Registry) {
	if reg == nil {
		return
	}
	reg.MustRegister(moduleID, func(module.Config) (module.Module, error) {
		return New(), nil
	})
}

// New configures the module metadata and IO contracts.
func New() *StaffReviewModule {
	info := module.Info{
		ID:          moduleID,
		Name:        "Staff Engineer Review",
		Description: "Runs the Staff Engineer review for MODULES.md and PLAN.md.",
		Version:     moduleVersion,
	}
	base := module.NewBase(info)
	base.SetInputs(
		artifact.CommissionDoc,
		artifact.ArchitectureDoc,
		artifact.ConventionsDoc,
		artifact.ModulesDoc,
		artifact.ActionPlanDoc,
	)
	base.SetOutputs(artifact.StaffReviewDoc)
	return &StaffReviewModule{Base: &base}
}

// Run validates prerequisites and starts the tmux session if needed.
func (m *StaffReviewModule) Run(ctx *module.ModuleContext) (module.Result, error) {
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
		return module.Result{Status: module.StatusNoOp, Message: "staff review already complete"}, nil
	}
	if m.windowName != "" {
		return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("staff review running in %s", m.windowName)}, nil
	}
	window := fmt.Sprintf("staff-review-%d", time.Now().Unix())
	if err := createTmuxWindow(window, ctx.Config.ProjectDir); err != nil {
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("staff-review: create tmux window: %w", err)
	}
	prompt := fmt.Sprintf(
		"You are a STAFF ENGINEER conducting a thorough review. "+
			"Read the planning documents from %s (COMMISSION.md, ARCHITECTURE.md, CONVENTIONS.md) "+
			"and the action plan from %s (MODULES.md, PLAN.md). "+
			"Review them as a staff engineer would: "+
			"- Are the modules well-defined and truly parallelizable? "+
			"- Is the plan realistic and complete? "+
			"- Are there gaps, risks, or unclear boundaries? "+
			"- What advice would you give before implementation begins? "+
			"Write your review to %s. Be thorough but constructive and do not end until your review is written.",
		ctx.Workflow.PlanDir(),
		ctx.Workflow.ActionDir(),
		ctx.Workflow.StaffReviewPath(),
	)
	if err := runOpenCode(window, prompt); err != nil {
		killTmuxWindow(window)
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("staff-review: launch opencode: %w", err)
	}
	m.windowName = window
	return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("staff review running in %s", window)}, nil
}

// IsComplete verifies that STAFF_REVIEW.md exists with correct metadata.
func (m *StaffReviewModule) IsComplete(ctx *module.ModuleContext) (bool, error) {
	if err := validateContext(ctx); err != nil {
		return false, err
	}
	ready, err := m.ensureReview(ctx)
	if err != nil || !ready {
		return ready, err
	}
	m.stopSession()
	return true, nil
}

func (m *StaffReviewModule) missingInput(ctx *module.ModuleContext) (string, error) {
	for _, ref := range m.Inputs() {
		result, err := ctx.Artifacts.Check(ref)
		if err != nil {
			return "", fmt.Errorf("staff-review: check %s: %w", ref.ID, err)
		}
		if result.State != artifact.StateReady {
			return ref.Name, nil
		}
	}
	return "", nil
}

func (m *StaffReviewModule) ensureReview(ctx *module.ModuleContext) (bool, error) {
	ref := artifact.StaffReviewDoc
	result, err := ctx.Artifacts.Check(ref)
	if err != nil {
		return false, fmt.Errorf("staff-review: check %s: %w", ref.ID, err)
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
			return false, fmt.Errorf("staff-review: %s: %w", ref.ID, result.Err)
		}
		return false, fmt.Errorf("staff-review: %s encountered an unknown error", ref.ID)
	default:
		return false, nil
	}
}

func (m *StaffReviewModule) writeMetadata(ctx *module.ModuleContext, ref artifact.ArtifactRef) error {
	path := ref.Path(ctx.Workflow)
	if path == "" {
		return fmt.Errorf("staff-review: unable to resolve path for %s", ref.ID)
	}
	body, err := readDocumentBody(path)
	if err != nil {
		return fmt.Errorf("staff-review: read %s: %w", ref.ID, err)
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
		},
	}
	if err := ctx.Artifacts.Write(ref, body, meta); err != nil {
		return fmt.Errorf("staff-review: write %s: %w", ref.ID, err)
	}
	return nil
}

func (m *StaffReviewModule) stopSession() {
	if m.windowName == "" {
		return
	}
	killTmuxWindow(m.windowName)
	m.windowName = ""
}

func validateContext(ctx *module.ModuleContext) error {
	if ctx == nil {
		return fmt.Errorf("staff-review: context is nil")
	}
	if ctx.Config == nil {
		return fmt.Errorf("staff-review: config is required")
	}
	if ctx.Workflow == nil {
		return fmt.Errorf("staff-review: workflow is required")
	}
	if ctx.Artifacts == nil {
		return fmt.Errorf("staff-review: artifact store is required")
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
