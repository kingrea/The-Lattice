package engine

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/workflow"
)

func TestCommissionWorkflowIncludesDeliveryModules(t *testing.T) {
	def := loadCommissionWorkflowDefinition(t)
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
	def := loadCommissionWorkflowDefinition(t)
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

func loadCommissionWorkflowDefinition(t *testing.T) workflow.WorkflowDefinition {
	t.Helper()
	path := filepath.Join("..", "..", "..", "workflows", "commission-work.yaml")
	def, err := workflow.LoadDefinitionFile(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	return def
}
