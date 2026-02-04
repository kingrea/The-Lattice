package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/config"
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/workflow"
	"github.com/yourusername/lattice/internal/workflow/engine"
)

func TestWorkflowStartAndResume(t *testing.T) {
	projectDir := t.TempDir()
	setTestLatticeRoot(t)
	if err := config.InitLatticeDir(projectDir); err != nil {
		t.Fatalf("init lattice dir: %v", err)
	}
	app := newTestApp(t, projectDir)
	model, cmd := app.startWorkflowRun(false)
	app = runCommands(t, model, cmd)
	if app.workflowView == nil {
		t.Fatalf("workflow view must be initialized")
	}
	firstRun := app.workflowView.state.RunID
	if firstRun == "" {
		t.Fatalf("expected run id to be set")
	}

	app2 := newTestApp(t, projectDir)
	model, cmd = app2.startWorkflowRun(true)
	app2 = runCommands(t, model, cmd)
	if app2.workflowView == nil {
		t.Fatalf("resume should attach workflow view")
	}
	if app2.workflowView.state.RunID != firstRun {
		t.Fatalf("expected resume to keep run id, got %s want %s", app2.workflowView.state.RunID, firstRun)
	}
}

func TestHandleModuleRunMarksCompletion(t *testing.T) {
	projectDir := t.TempDir()
	setTestLatticeRoot(t)
	if err := config.InitLatticeDir(projectDir); err != nil {
		t.Fatalf("init lattice dir: %v", err)
	}
	app := newTestApp(t, projectDir)
	model, cmd := app.startWorkflowRun(false)
	app = runCommands(t, model, cmd)
	view := app.workflowView
	if view == nil {
		t.Fatalf("workflow view missing")
	}
	if got := view.state.Status; got != engine.EngineStatusRunning {
		t.Fatalf("expected running status, got %s", got)
	}
	mod, err := view.registry.Resolve("stub-alpha", nil)
	if err != nil {
		t.Fatalf("resolve module: %v", err)
	}
	if _, err := mod.Run(view.moduleCtx); err != nil {
		t.Fatalf("run module: %v", err)
	}
	view.handleModuleRunFinished(moduleRunFinishedMsg{id: "alpha", result: module.Result{Status: module.StatusCompleted}})
	if got := view.state.Status; got != engine.EngineStatusComplete {
		t.Fatalf("expected complete status after module run, got %s", got)
	}
}

func newTestApp(t *testing.T, projectDir string) *App {
	t.Helper()
	loader := func(cfg *config.Config, workflowID string) (workflow.WorkflowDefinition, error) {
		return workflow.WorkflowDefinition{
			ID:   "test-workflow",
			Name: "Test Workflow",
			Modules: []workflow.ModuleRef{
				{ID: "alpha", ModuleID: "stub-alpha", Name: "Alpha"},
			},
		}, nil
	}
	factory := func() *module.Registry {
		reg := module.NewRegistry()
		reg.MustRegister("stub-alpha", func(module.Config) (module.Module, error) {
			return &stubModule{id: "stub-alpha"}, nil
		})
		return reg
	}
	app, err := NewApp(projectDir, WithWorkflowDefinitionLoader(loader), WithModuleRegistryFactory(factory))
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	return app
}

func runCommands(t *testing.T, model tea.Model, cmd tea.Cmd) *App {
	t.Helper()
	app, ok := model.(*App)
	if !ok {
		t.Fatalf("unexpected model type: %T", model)
	}
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		nextModel, nextCmd := app.Update(msg)
		var ok bool
		app, ok = nextModel.(*App)
		if !ok {
			t.Fatalf("unexpected model type: %T", nextModel)
		}
		cmd = nextCmd
	}
	return app
}

func setTestLatticeRoot(t *testing.T) {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("cwd: %v", err)
	}
	prev := os.Getenv("LATTICE_ROOT")
	if err := os.Setenv("LATTICE_ROOT", filepath.Clean(root)); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("LATTICE_ROOT", prev)
	})
}

type stubModule struct {
	id string
}

func (m *stubModule) Info() module.Info {
	return module.Info{ID: m.id, Name: strings.ToUpper(m.id), Version: "1.0.0"}
}

func (m *stubModule) Inputs() []artifact.ArtifactRef { return nil }

func (m *stubModule) Outputs() []artifact.ArtifactRef { return nil }

func (m *stubModule) IsComplete(ctx *module.ModuleContext) (bool, error) {
	path := m.markerPath(ctx)
	if path == "" {
		return false, nil
	}
	if _, err := os.Stat(path); err == nil {
		return true, nil
	}
	return false, nil
}

func (m *stubModule) Run(ctx *module.ModuleContext) (module.Result, error) {
	path := m.markerPath(ctx)
	if path == "" {
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("missing marker path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := os.WriteFile(path, []byte("done"), 0o644); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	return module.Result{Status: module.StatusCompleted, Message: "ok"}, nil
}

func (m *stubModule) markerPath(ctx *module.ModuleContext) string {
	if ctx == nil || ctx.Workflow == nil {
		return ""
	}
	return filepath.Join(ctx.Workflow.Dir(), "engine-test", m.id+".marker")
}
