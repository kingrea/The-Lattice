package engine

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/workflow"
)

var workflowsDir = filepath.Join("..", "..", "..", "workflows")

func TestCommissionWorkflowIncludesDeliveryModules(t *testing.T) {
	def := loadWorkflowDefinition(t, "commission-work")
	want := []string{
		"anchor-docs",
		"action-plan",
		"staff-review",
		"staff-incorporate",
		"parallel-reviews",
		"consolidation",
		"bead-creation",
		"orchestrator-selection",
		"hiring",
		"work-process",
		"refinement",
		"release",
	}
	if got := def.ModuleIDs(); !slices.Equal(got, want) {
		t.Fatalf("commission-work module order mismatch\nwant %v\ngot  %v", want, got)
	}
	assertDependencies := func(id string, expected []string) {
		if deps := def.Dependencies(id); !slices.Equal(deps, expected) {
			t.Fatalf("%s dependencies mismatch\nwant %v\ngot  %v", id, expected, deps)
		}
	}
	assertDependencies("orchestrator-selection", []string{"bead-creation"})
	assertDependencies("hiring", []string{"orchestrator-selection"})
	assertDependencies("work-process", []string{"hiring"})
	assertDependencies("refinement", []string{"work-process"})
	assertDependencies("release", []string{"refinement"})
}

func TestCommissionWorkflowRunsToCompletionWithEngine(t *testing.T) {
	def := loadWorkflowDefinition(t, "commission-work")
	runWorkflowToCompletion(t, def)
}

func TestQuickStartWorkflowIncludesRapidModules(t *testing.T) {
	def := loadWorkflowDefinition(t, "quick-start")
	want := []string{
		"anchor-docs",
		"action-plan",
		"staff-review",
		"bead-creation",
		"orchestrator-selection",
		"hiring",
		"work-process",
		"release",
	}
	if got := def.ModuleIDs(); !slices.Equal(got, want) {
		t.Fatalf("quick-start module order mismatch\nwant %v\ngot  %v", want, got)
	}
	assertDependencies := func(id string, expected []string) {
		if deps := def.Dependencies(id); !slices.Equal(deps, expected) {
			t.Fatalf("%s dependencies mismatch\nwant %v\ngot  %v", id, expected, deps)
		}
	}
	assertDependencies("action-plan", []string{"anchor-docs"})
	assertDependencies("staff-review", []string{"action-plan"})
	assertDependencies("bead-creation", []string{"staff-review"})
	assertDependencies("orchestrator-selection", []string{"bead-creation"})
	assertDependencies("hiring", []string{"orchestrator-selection"})
	assertDependencies("work-process", []string{"hiring"})
	assertDependencies("release", []string{"work-process"})
	if def.Runtime.MaxParallel != 2 {
		t.Fatalf("quick-start max_parallel mismatch: want 2, got %d", def.Runtime.MaxParallel)
	}
}

func TestQuickStartWorkflowRunsToCompletionWithEngine(t *testing.T) {
	def := loadWorkflowDefinition(t, "quick-start")
	runWorkflowToCompletion(t, def)
}

func loadWorkflowDefinition(t *testing.T, id string) workflow.WorkflowDefinition {
	t.Helper()
	path := filepath.Join(workflowsDir, id+".yaml")
	def, err := workflow.LoadDefinitionFile(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	return def
}

func runWorkflowToCompletion(t *testing.T, def workflow.WorkflowDefinition) {
	t.Helper()
	ctx := newTestModuleContext(t)
	reg := module.NewRegistry()
	stubs := map[string]*stubModule{}
	for _, ref := range def.Modules {
		modID := ref.ModuleID
		if _, exists := stubs[modID]; exists {
			continue
		}
		stub := newStubModule(modID)
		stubs[modID] = stub
		instance := stub
		reg.MustRegister(modID, func(module.Config) (module.Module, error) {
			return instance, nil
		})
	}
	repo := NewRepository(ctx.Workflow)
	eng, err := New(reg, repo)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	state, err := eng.Start(ctx, StartRequest{Definition: def})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(state.Runnable) == 0 || state.Runnable[0] != def.Modules[0].InstanceID() {
		t.Fatalf("expected first module runnable, got %+v", state.Runnable)
	}
	for _, ref := range def.Modules {
		stubs[ref.ModuleID].setComplete(true)
		state, err = eng.Update(ctx, UpdateRequest{Results: []ModuleStatusUpdate{{
			ID:     ref.InstanceID(),
			Result: module.Result{Status: module.StatusCompleted},
		}}})
		if err != nil {
			t.Fatalf("update %s: %v", ref.InstanceID(), err)
		}
	}
	if state.Status != EngineStatusComplete {
		t.Fatalf("expected engine complete, got %s", state.Status)
	}
}
