package staff_review

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/modules/runtime"
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
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return false, err
	}
	ready, err := runtime.EnsureDocument(ctx, moduleID, moduleVersion, artifact.StaffReviewDoc, runtime.WithInputs(m.Inputs()...))
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

func (m *StaffReviewModule) stopSession() {
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
