package staff_incorporate

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/modules/runtime"
)

const (
	moduleID      = "staff-incorporate"
	moduleVersion = "1.0.0"
)

// StaffIncorporateModule applies the Staff Engineer review to MODULES.md and
// PLAN.md and signals completion with the staff-feedback marker.
type StaffIncorporateModule struct {
	*module.Base
	windowName string
}

// Register installs the module factory into the registry.
func Register(reg *module.Registry) {
	if reg == nil {
		return
	}
	reg.MustRegister(moduleID, func(module.Config) (module.Module, error) {
		return New(), nil
	})
}

// New configures IO contracts for the module.
func New() *StaffIncorporateModule {
	info := module.Info{
		ID:          moduleID,
		Name:        "Apply Staff Engineer Feedback",
		Description: "Applies the Staff Engineer review to MODULES.md and PLAN.md and creates the readiness marker.",
		Version:     moduleVersion,
	}
	base := module.NewBase(info)
	base.SetInputs(
		artifact.StaffReviewDoc,
		artifact.ModulesDoc,
		artifact.ActionPlanDoc,
		artifact.CommissionDoc,
		artifact.ArchitectureDoc,
		artifact.ConventionsDoc,
	)
	base.SetOutputs(
		artifact.ModulesDoc,
		artifact.ActionPlanDoc,
		artifact.StaffFeedbackApplied,
	)
	return &StaffIncorporateModule{Base: &base}
}

// Run validates prerequisites and launches the tmux session if needed.
func (m *StaffIncorporateModule) Run(ctx *module.ModuleContext) (module.Result, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
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
		return module.Result{Status: module.StatusNoOp, Message: "staff feedback already applied"}, nil
	}
	if m.windowName != "" {
		return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("staff incorporation running in %s", m.windowName)}, nil
	}
	window := fmt.Sprintf("staff-incorporate-%d", time.Now().Unix())
	if err := createTmuxWindow(window, ctx.Config.ProjectDir); err != nil {
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("staff-incorporate: create tmux window: %w", err)
	}
	prompt := fmt.Sprintf(
		"You already wrote the Staff Engineer review at %s. Before the user sees anything, apply that feedback directly to the plan. "+
			"Read the planning docs in %s (COMMISSION.md, ARCHITECTURE.md, CONVENTIONS.md) and the current action plan in %s (MODULES.md, PLAN.md). "+
			"Update MODULES.md and PLAN.md so the guidance from your review is fully incorporated and clearly explained. "+
			"Add a short section near the top of PLAN.md summarizing the adjustments made. "+
			"When the updates are complete, create the marker file %s to signal that the user can now review the improved plan. "+
			"Do not ask the user to read the review file—deliver the updated plan instead.",
		ctx.Workflow.StaffReviewPath(),
		ctx.Workflow.PlanDir(),
		ctx.Workflow.ActionDir(),
		ctx.Workflow.StaffFeedbackAppliedPath(),
	)
	if err := runOpenCode(window, prompt); err != nil {
		killTmuxWindow(window)
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("staff-incorporate: launch opencode: %w", err)
	}
	m.windowName = window
	return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("staff incorporation running in %s", window)}, nil
}

// IsComplete checks whether MODULES/PLAN plus the marker are in place.
func (m *StaffIncorporateModule) IsComplete(ctx *module.ModuleContext) (bool, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return false, err
	}
	markerReady, err := runtime.EnsureMarker(ctx, moduleID, moduleVersion, artifact.StaffFeedbackApplied)
	if err != nil {
		return false, err
	}
	if markerReady {
		m.stopSession()
		return true, nil
	}
	ready, err := runtime.EnsureDocuments(ctx, moduleID, moduleVersion, []artifact.ArtifactRef{artifact.ModulesDoc, artifact.ActionPlanDoc}, runtime.WithInputs(m.Inputs()...))
	if err != nil || !ready {
		return false, err
	}
	return false, nil
}

func (m *StaffIncorporateModule) missingInput(ctx *module.ModuleContext) (string, error) {
	for _, ref := range m.Inputs() {
		result, err := ctx.Artifacts.Check(ref)
		if err != nil {
			return "", fmt.Errorf("staff-incorporate: check %s: %w", ref.ID, err)
		}
		if result.State != artifact.StateReady {
			return ref.Name, nil
		}
	}
	return "", nil
}

func (m *StaffIncorporateModule) stopSession() {
	if m.windowName == "" {
		return
	}
	killTmuxWindow(m.windowName)
	m.windowName = ""
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
