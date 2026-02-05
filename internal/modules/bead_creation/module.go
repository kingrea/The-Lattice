package bead_creation

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/modules/runtime"
)

const (
	moduleID      = "bead-creation"
	moduleVersion = "1.0.0"
)

// BeadCreationModule initializes beads (bd) and guides the user through creating
// beads from MODULES.md and PLAN.md.
type BeadCreationModule struct {
	*module.Base
	windowName string
}

// Register installs the module factory in the registry.
func Register(reg *module.Registry) {
	if reg == nil {
		return
	}
	reg.MustRegister(moduleID, func(module.Config) (module.Module, error) {
		return New(), nil
	})
}

// New configures module metadata and IO contracts.
func New() *BeadCreationModule {
	info := module.Info{
		ID:          moduleID,
		Name:        "Create Beads",
		Description: "Turns MODULES/PLAN into beads records and writes the beads-created marker.",
		Version:     moduleVersion,
	}
	base := module.NewBase(info)
	base.SetInputs(
		artifact.ModulesDoc,
		artifact.ActionPlanDoc,
		artifact.ReviewsAppliedMarker,
	)
	base.SetOutputs(artifact.BeadsCreatedMarker)
	return &BeadCreationModule{Base: &base}
}

// Run ensures beads are initialized and launches the tmux session.
func (m *BeadCreationModule) Run(ctx *module.ModuleContext) (module.Result, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := ensureBeadsInitialized(ctx.Config.ProjectDir); err != nil {
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
		return module.Result{Status: module.StatusNoOp, Message: "beads already created"}, nil
	}
	if m.windowName != "" {
		return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("bead creation running in %s", m.windowName)}, nil
	}
	window := fmt.Sprintf("bead-creation-%d", time.Now().Unix())
	if err := createTmuxWindow(window, ctx.Config.ProjectDir); err != nil {
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("bead-creation: create tmux window: %w", err)
	}
	prompt := fmt.Sprintf(
		"You are setting up work tracking with beads (bd). IMPORTANT: Use 'bd' for task tracking. Read AGENTS.md for instructions. "+
			"First, initialize beads in the project directory: cd %s && bd init. "+
			"Then read the planning documents: - %s/MODULES.md - %s/PLAN.md. "+
			"Create beads for tracking: 1. For each MODULE, create an epic bead. 2. For each task in PLAN.md, create a child bead under its parent module. "+
			"When all beads are created, create an empty marker file at %s to signal completion. Run 'bd list' at the end to verify the structure. Do not end until the marker file exists.",
		ctx.Config.ProjectDir,
		ctx.Workflow.ActionDir(),
		ctx.Workflow.ActionDir(),
		ctx.Workflow.BeadsCreatedPath(),
	)
	if err := runOpenCode(window, prompt); err != nil {
		killTmuxWindow(window)
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("bead-creation: launch opencode: %w", err)
	}
	m.windowName = window
	return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("bead creation running in %s", window)}, nil
}

// IsComplete waits for the beads-created marker.
func (m *BeadCreationModule) IsComplete(ctx *module.ModuleContext) (bool, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return false, err
	}
	ready, err := runtime.EnsureMarker(ctx, moduleID, moduleVersion, artifact.BeadsCreatedMarker)
	if err != nil {
		return false, err
	}
	if ready {
		m.stopSession()
		return true, nil
	}
	return false, nil
}

func (m *BeadCreationModule) missingInput(ctx *module.ModuleContext) (string, error) {
	for _, ref := range m.Inputs() {
		result, err := ctx.Artifacts.Check(ref)
		if err != nil {
			return "", fmt.Errorf("bead-creation: check %s: %w", ref.ID, err)
		}
		if result.State != artifact.StateReady {
			return ref.Name, nil
		}
	}
	return "", nil
}

func (m *BeadCreationModule) stopSession() {
	if m.windowName == "" {
		return
	}
	killTmuxWindow(m.windowName)
	m.windowName = ""
}

func ensureBeadsInitialized(projectDir string) error {
	needsInit, err := beadsInitRequired(projectDir)
	if err != nil {
		return err
	}
	if !needsInit {
		return nil
	}
	return runBeadsInit(projectDir)
}

func beadsInitRequired(projectDir string) (bool, error) {
	agentsPath := filepath.Join(projectDir, "AGENTS.md")
	if _, err := os.Stat(agentsPath); err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("bead-creation: checking AGENTS.md: %w", err)
	}
	beadsDir := filepath.Join(projectDir, ".beads")
	entries, err := os.ReadDir(beadsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("bead-creation: reading .beads: %w", err)
	}
	if len(entries) == 0 {
		return true, nil
	}
	return false, nil
}

func runBeadsInit(projectDir string) error {
	cmd := exec.Command("bd", "init")
	cmd.Dir = projectDir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		trimmed := strings.TrimSpace(out.String())
		if trimmed != "" {
			return fmt.Errorf("bead-creation: bd init failed: %s: %w", trimmed, err)
		}
		return fmt.Errorf("bead-creation: bd init failed: %w", err)
	}
	return nil
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
