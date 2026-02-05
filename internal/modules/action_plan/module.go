package action_plan

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
	moduleID      = "action-plan"
	moduleVersion = "1.0.0"
)

// ActionPlanModule orchestrates the creation of MODULES.md and PLAN.md from the
// previously generated anchor documents.
type ActionPlanModule struct {
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

// New constructs the module definition with its IO contracts.
func New() *ActionPlanModule {
	info := module.Info{
		ID:          moduleID,
		Name:        "Create Action Plan",
		Description: "Produces MODULES.md and PLAN.md from the anchor documents.",
		Version:     moduleVersion,
	}
	base := module.NewBase(info)
	base.SetInputs(
		artifact.CommissionDoc,
		artifact.ArchitectureDoc,
		artifact.ConventionsDoc,
	)
	base.SetOutputs(
		artifact.ModulesDoc,
		artifact.ActionPlanDoc,
	)
	return &ActionPlanModule{Base: &base}
}

// Run ensures prerequisites exist, spawns the OpenCode session when needed, and
// keeps running until both MODULES.md and PLAN.md are ready.
func (m *ActionPlanModule) Run(ctx *module.ModuleContext) (module.Result, error) {
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
		return module.Result{Status: module.StatusNoOp, Message: "action plan already exists"}, nil
	}
	if m.windowName != "" {
		return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("action plan running in %s", m.windowName)}, nil
	}
	window := fmt.Sprintf("action-plan-%d", time.Now().Unix())
	if err := createTmuxWindow(window, ctx.Config.ProjectDir); err != nil {
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("action-plan: create tmux window: %w", err)
	}
	prompt := fmt.Sprintf(
		"You are creating an action plan based on completed planning documents. "+
			"Read the anchor documents from %s (COMMISSION.md, ARCHITECTURE.md, CONVENTIONS.md). "+
			"Based on these documents, create two files in %s: "+
			"1. MODULES.md - Break down the work into top-level parallelizable modules. "+
			"Each module should be independent enough to be worked on by a separate agent. "+
			"Include clear boundaries and interfaces between modules. "+
			"2. PLAN.md - Create an implementation plan that sequences the work. "+
			"Reference the modules and show dependencies. "+
			"Write both files. Do not end until both exist in %s.",
		ctx.Workflow.PlanDir(),
		ctx.Workflow.ActionDir(),
		ctx.Workflow.ActionDir(),
	)
	if err := runOpenCode(window, prompt); err != nil {
		killTmuxWindow(window)
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("action-plan: launch opencode: %w", err)
	}
	m.windowName = window
	return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("action plan running in %s", window)}, nil
}

// IsComplete verifies that both output documents exist with the expected
// metadata.
func (m *ActionPlanModule) IsComplete(ctx *module.ModuleContext) (bool, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return false, err
	}
	inputs := runtime.WithInputs(m.Inputs()...)
	for _, ref := range m.Outputs() {
		ready, err := runtime.EnsureDocument(ctx, moduleID, moduleVersion, ref, inputs)
		if err != nil {
			return false, err
		}
		if !ready {
			return false, nil
		}
	}
	m.stopSession()
	return true, nil
}

func (m *ActionPlanModule) missingInput(ctx *module.ModuleContext) (string, error) {
	for _, ref := range m.Inputs() {
		result, err := ctx.Artifacts.Check(ref)
		if err != nil {
			return "", fmt.Errorf("action-plan: check %s: %w", ref.ID, err)
		}
		if result.State != artifact.StateReady {
			return ref.Name, nil
		}
	}
	return "", nil
}

func (m *ActionPlanModule) stopSession() {
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
