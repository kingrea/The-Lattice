package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/config"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/workflow"
	"github.com/kingrea/The-Lattice/internal/workflow/engine"
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

func TestWorkflowCompletionReturnsToMainMenu(t *testing.T) {
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
	mod, err := view.registry.Resolve("stub-alpha", nil)
	if err != nil {
		t.Fatalf("resolve module: %v", err)
	}
	if _, err := mod.Run(view.moduleCtx); err != nil {
		t.Fatalf("run module: %v", err)
	}
	finishCmd := view.handleModuleRunFinished(moduleRunFinishedMsg{id: "alpha", result: module.Result{Status: module.StatusCompleted}})
	if finishCmd == nil {
		t.Fatalf("expected workflow completion command")
	}
	msg := finishCmd()
	if msg == nil {
		t.Fatalf("expected workflow completion message")
	}
	nextModel, nextCmd := app.Update(msg)
	app = runCommands(t, nextModel, nextCmd)
	if app.state != stateMainMenu {
		t.Fatalf("expected return to main menu after completion, got state %d", app.state)
	}
}

func TestWorkflowCompletionQuitsWithoutParent(t *testing.T) {
	projectDir := t.TempDir()
	setTestLatticeRoot(t)
	if err := config.InitLatticeDir(projectDir); err != nil {
		t.Fatalf("init lattice dir: %v", err)
	}
	app := newTestApp(t, projectDir)
	app.workflowReturnState = stateCommissionWork
	model, cmd := app.handleWorkflowFinished(workflowFinishedMsg{WorkflowID: "solo", Status: engine.EngineStatusComplete})
	var ok bool
	app, ok = model.(*App)
	if !ok {
		t.Fatalf("expected app model, got %T", model)
	}
	if cmd == nil {
		t.Fatalf("expected quit command")
	}
	if msg := cmd(); msg == nil {
		t.Fatalf("expected quit message")
	} else {
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Fatalf("expected tea.QuitMsg, got %T", msg)
		}
	}
}

func TestWorkflowSelectionPersistsAndLoadsDefinition(t *testing.T) {
	projectDir := t.TempDir()
	setTestLatticeRoot(t)
	if err := config.InitLatticeDir(projectDir); err != nil {
		t.Fatalf("init lattice dir: %v", err)
	}
	loaderCalls := map[string]int{}
	loader := func(cfg *config.Config, workflowID string) (workflow.WorkflowDefinition, error) {
		loaderCalls[workflowID]++
		return workflow.WorkflowDefinition{
			ID:   workflowID,
			Name: strings.ToUpper(workflowID),
			Modules: []workflow.ModuleRef{
				{ID: "alpha", ModuleID: "stub-alpha", Name: "Alpha"},
			},
		}, nil
	}
	app := newTestApp(t, projectDir, WithWorkflowDefinitionLoader(loader))
	if err := app.setWorkflowSelection("solo"); err != nil {
		t.Fatalf("set workflow selection: %v", err)
	}
	if got := app.config.DefaultWorkflow(); got != "solo" {
		t.Fatalf("expected config default to update, got %s", got)
	}
	model, cmd := app.startWorkflowRun(false)
	app = runCommands(t, model, cmd)
	if app.workflowView == nil {
		t.Fatalf("workflow view missing after selection")
	}
	if got := app.workflowView.state.WorkflowID; got != "solo" {
		t.Fatalf("workflow view launched %s, want solo", got)
	}
	if loaderCalls["solo"] == 0 {
		t.Fatalf("expected workflow loader to be invoked for solo")
	}
}

func TestWorkflowSelectorIncludesBundledWorkflows(t *testing.T) {
	projectDir := t.TempDir()
	setTestLatticeRoot(t)
	if err := config.InitLatticeDir(projectDir); err != nil {
		t.Fatalf("init lattice dir: %v", err)
	}
	app := newTestApp(t, projectDir)
	app.config.Project.Workflows.Available = nil
	app.selectedWorkflow = ""
	app.refreshWorkflowMenu()
	ids := map[string]struct{}{}
	for _, option := range app.workflowChoices {
		ids[option.ID()] = struct{}{}
	}
	for _, id := range []string{"commission-work", "quick-start", "solo"} {
		if _, ok := ids[id]; !ok {
			t.Fatalf("workflow menu missing %s", id)
		}
	}
}

func newTestApp(t *testing.T, projectDir string, opts ...AppOption) *App {
	t.Helper()
	t.Setenv("LATTICE_BRIDGE_ENABLED", "false")
	loader := func(cfg *config.Config, workflowID string) (workflow.WorkflowDefinition, error) {
		id := strings.TrimSpace(workflowID)
		if id == "" {
			id = "test-workflow"
		}
		return workflow.WorkflowDefinition{
			ID:   id,
			Name: "Test Workflow",
			Modules: []workflow.ModuleRef{
				{ID: "alpha", ModuleID: "stub-alpha", Name: "Alpha"},
			},
		}, nil
	}
	factory := func(*config.Config) (*module.Registry, error) {
		reg := module.NewRegistry()
		reg.MustRegister("stub-alpha", func(module.Config) (module.Module, error) {
			return &stubModule{id: "stub-alpha"}, nil
		})
		return reg, nil
	}
	baseOpts := []AppOption{WithWorkflowDefinitionLoader(loader), WithModuleRegistryFactory(factory)}
	baseOpts = append(baseOpts, opts...)
	app, err := NewApp(projectDir, baseOpts...)
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
