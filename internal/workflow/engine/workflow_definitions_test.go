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
	if def.Runtime.MaxParallel != 3 {
		t.Fatalf("commission-work max_parallel mismatch: want 3, got %d", def.Runtime.MaxParallel)
	}
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
	assertMetadataValue(t, def, "intent", "rapid-engagement")
	assertMetadataValue(t, def, "recommended_use", "Short scoped work where teams need a staffed cycle quickly")
	assertMetadataValue(t, def, "default_targets", "release")
}

func TestQuickStartWorkflowRunsToCompletionWithEngine(t *testing.T) {
	def := loadWorkflowDefinition(t, "quick-start")
	runWorkflowToCompletion(t, def)
}

func TestSoloWorkflowIncludesSingleOperatorModules(t *testing.T) {
	def := loadWorkflowDefinition(t, "solo")
	want := []string{
		"anchor-docs",
		"action-plan",
		"solo-work",
		"release",
	}
	if got := def.ModuleIDs(); !slices.Equal(got, want) {
		t.Fatalf("solo module order mismatch\nwant %v\ngot  %v", want, got)
	}
	assertDependencies := func(id string, expected []string) {
		if deps := def.Dependencies(id); !slices.Equal(deps, expected) {
			t.Fatalf("%s dependencies mismatch\nwant %v\ngot  %v", id, expected, deps)
		}
	}
	assertDependencies("action-plan", []string{"anchor-docs"})
	assertDependencies("solo-work", []string{"action-plan"})
	assertDependencies("release", []string{"solo-work"})
	if def.Runtime.MaxParallel != 1 {
		t.Fatalf("solo max_parallel mismatch: want 1, got %d", def.Runtime.MaxParallel)
	}
	assertMetadataValue(t, def, "intent", "solo-delivery")
	assertMetadataValue(t, def, "recommended_use", "Individuals running scoped delivery cycles without a crew")
	assertMetadataValue(t, def, "default_targets", "release")
}

func TestSoloWorkflowRunsToCompletionWithEngine(t *testing.T) {
	def := loadWorkflowDefinition(t, "solo")
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

func assertMetadataValue(t *testing.T, def workflow.WorkflowDefinition, key, want string) {
	t.Helper()
	got, ok := def.Metadata[key]
	if !ok {
		t.Fatalf("workflow %s metadata missing key %s", def.ID, key)
	}
	if got != want {
		t.Fatalf("workflow %s metadata %s mismatch: want %q, got %q", def.ID, key, want, got)
	}
}
