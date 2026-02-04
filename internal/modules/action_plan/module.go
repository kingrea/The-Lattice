package action_plan

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
	if err := validateContext(ctx); err != nil {
		return false, err
	}
	for _, ref := range m.Outputs() {
		ready, err := m.ensureArtifact(ctx, ref)
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

func (m *ActionPlanModule) ensureArtifact(ctx *module.ModuleContext, ref artifact.ArtifactRef) (bool, error) {
	result, err := ctx.Artifacts.Check(ref)
	if err != nil {
		return false, fmt.Errorf("action-plan: check %s: %w", ref.ID, err)
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
			return false, fmt.Errorf("action-plan: %s: %w", ref.ID, result.Err)
		}
		return false, fmt.Errorf("action-plan: %s encountered an unknown error", ref.ID)
	default:
		return false, nil
	}
}

func (m *ActionPlanModule) writeMetadata(ctx *module.ModuleContext, ref artifact.ArtifactRef) error {
	path := ref.Path(ctx.Workflow)
	if path == "" {
		return fmt.Errorf("action-plan: unable to resolve path for %s", ref.ID)
	}
	body, err := readDocumentBody(path)
	if err != nil {
		return fmt.Errorf("action-plan: read %s: %w", ref.ID, err)
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
		},
	}
	if err := ctx.Artifacts.Write(ref, body, meta); err != nil {
		return fmt.Errorf("action-plan: write %s: %w", ref.ID, err)
	}
	return nil
}

func (m *ActionPlanModule) stopSession() {
	if m.windowName == "" {
		return
	}
	killTmuxWindow(m.windowName)
	m.windowName = ""
}

func validateContext(ctx *module.ModuleContext) error {
	if ctx == nil {
		return fmt.Errorf("action-plan: context is nil")
	}
	if ctx.Config == nil {
		return fmt.Errorf("action-plan: config is required")
	}
	if ctx.Workflow == nil {
		return fmt.Errorf("action-plan: workflow is required")
	}
	if ctx.Artifacts == nil {
		return fmt.Errorf("action-plan: artifact store is required")
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
