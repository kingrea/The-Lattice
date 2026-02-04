package engine

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/config"
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/workflow"
	"github.com/yourusername/lattice/internal/workflow/resolver"
	"github.com/yourusername/lattice/internal/workflow/scheduler"
)

func TestEngineStartPersistsState(t *testing.T) {
	eng, repo, ctx, stubs, def := newEngineHarness(t)
	stubs["plan"].setComplete(false)
	state, err := eng.Start(ctx, StartRequest{Definition: def})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if state.RunID == "" {
		t.Fatalf("expected run id")
	}
	if len(state.Runnable) != 1 || state.Runnable[0] != "anchor-plan" {
		t.Fatalf("unexpected runnable set: %+v", state.Runnable)
	}
	stored, err := repo.Load()
	if err != nil {
		t.Fatalf("load repo: %v", err)
	}
	if stored.RunID != state.RunID {
		t.Fatalf("persisted run id mismatch: %s vs %s", stored.RunID, state.RunID)
	}
}

func TestEngineResumeRefreshesCompletion(t *testing.T) {
	eng, _, ctx, stubs, def := newEngineHarness(t)
	stubs["plan"].setComplete(false)
	if _, err := eng.Start(ctx, StartRequest{Definition: def}); err != nil {
		t.Fatalf("start: %v", err)
	}
	stubs["plan"].setComplete(true)
	state, err := eng.Resume(ctx, ResumeRequest{})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if len(state.Runnable) == 0 || state.Runnable[0] != "module-build" {
		t.Fatalf("expected module-build runnable after plan completion, got %+v", state.Runnable)
	}
	plan := findModule(state, "anchor-plan")
	if plan.State != resolver.NodeStateComplete {
		t.Fatalf("expected plan complete, got %s", plan.State)
	}
}

func TestEngineUpdateRecordsResultsAndFailures(t *testing.T) {
	eng, _, ctx, stubs, def := newEngineHarness(t)
	stubs["plan"].setComplete(true)
	if _, err := eng.Start(ctx, StartRequest{Definition: def}); err != nil {
		t.Fatalf("start: %v", err)
	}
	state, err := eng.Update(ctx, UpdateRequest{Results: []ModuleStatusUpdate{{
		ID:     "anchor-plan",
		Result: module.Result{Status: module.StatusCompleted, Message: "ok"},
	}}})
	if err != nil {
		t.Fatalf("update complete: %v", err)
	}
	if run, ok := state.Runs["anchor-plan"]; !ok || run.Status != module.StatusCompleted {
		t.Fatalf("expected run log for anchor-plan, got %+v", state.Runs["anchor-plan"])
	}
	stubs["build"].setComplete(false)
	state, err = eng.Update(ctx, UpdateRequest{Results: []ModuleStatusUpdate{{
		ID:     "module-build",
		Result: module.Result{Status: module.StatusFailed, Message: "boom"},
		Err:    errors.New("boom"),
	}}})
	if err != nil {
		t.Fatalf("update failure: %v", err)
	}
	if state.Status != EngineStatusError {
		t.Fatalf("expected engine error after failure, got %s", state.Status)
	}
	if !strings.Contains(state.StatusReason, "module-build") {
		t.Fatalf("expected status reason to reference module-build, got %q", state.StatusReason)
	}
}

func TestEngineDetectsArtifactInvalidations(t *testing.T) {
	eng, _, ctx, stubs, def := newEngineHarness(t)
	stubs["plan"].setComplete(true)
	stubs["plan"].setOutputs(artifact.ModulesDoc)
	writeArtifact(t, ctx, artifact.ModulesDoc, stubs["plan"].info.ID)
	if _, err := eng.Start(ctx, StartRequest{Definition: def}); err != nil {
		t.Fatalf("start: %v", err)
	}
	writeArtifact(t, ctx, artifact.ModulesDoc, "other-module")
	state, err := eng.Update(ctx, UpdateRequest{})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	plan := findModule(state, "anchor-plan")
	if plan.State != resolver.NodeStateReady {
		t.Fatalf("expected plan ready after invalidation, got %s", plan.State)
	}
	report, ok := plan.Artifacts[artifact.ModulesDoc.ID]
	if !ok || report.Status != module.ArtifactStatusInvalid {
		t.Fatalf("expected invalid artifact, got %+v", report)
	}
}

func TestEngineClaimAndReleaseRespectsParallelism(t *testing.T) {
	ctx := newTestModuleContext(t)
	def := workflow.WorkflowDefinition{
		ID:      "parallel-workflow",
		Runtime: workflow.WorkflowRuntimeConfig{MaxParallel: 2},
		Modules: []workflow.ModuleRef{
			{ID: "anchor-plan", ModuleID: "plan"},
			{ID: "module-build", ModuleID: "build", DependsOn: []string{"anchor-plan"}},
			{ID: "module-docs", ModuleID: "docs", DependsOn: []string{"anchor-plan"}},
		},
	}
	stubs := map[string]*stubModule{
		"plan":  newStubModule("plan"),
		"build": newStubModule("build"),
		"docs":  newStubModule("docs"),
	}
	stubs["plan"].setComplete(true)
	stubs["build"].setComplete(false)
	stubs["docs"].setComplete(false)
	eng, repo := newCustomEngine(t, ctx, def, stubs)
	if _, err := eng.Start(ctx, StartRequest{Definition: def}); err != nil {
		t.Fatalf("start: %v", err)
	}
	maxParallel := 1
	claim, err := eng.Claim(ctx, ClaimRequest{
		Runtime: &RuntimeOverrides{MaxParallel: &maxParallel},
		Limit:   2,
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claim.Claims) != 1 {
		t.Fatalf("expected single claim due to parallel limit, got %d", len(claim.Claims))
	}
	if len(claim.State.Runtime.Running) != 1 {
		t.Fatalf("expected runtime to track running module, got %+v", claim.State.Runtime.Running)
	}
	secondClaim, err := eng.Claim(ctx, ClaimRequest{Runtime: &RuntimeOverrides{MaxParallel: &maxParallel}, Limit: 1})
	if err != nil {
		t.Fatalf("claim while running: %v", err)
	}
	if len(secondClaim.Claims) != 0 {
		t.Fatalf("expected no claims while capacity exhausted, got %+v", secondClaim.Claims)
	}
	firstID := claim.Claims[0].ID
	if _, err := eng.Update(ctx, UpdateRequest{Results: []ModuleStatusUpdate{{
		ID:     firstID,
		Result: module.Result{Status: module.StatusCompleted},
	}}}); err != nil {
		t.Fatalf("update: %v", err)
	}
	state, err := repo.Load()
	if err != nil {
		t.Fatalf("load repo: %v", err)
	}
	if len(state.Runtime.Running) != 0 {
		t.Fatalf("expected running set cleared after completion, got %+v", state.Runtime.Running)
	}
	thirdClaim, err := eng.Claim(ctx, ClaimRequest{Limit: 1})
	if err != nil {
		t.Fatalf("claim remaining module: %v", err)
	}
	if len(thirdClaim.Claims) != 1 {
		t.Fatalf("expected to claim remaining module, got %d", len(thirdClaim.Claims))
	}
	if _, err := eng.Update(ctx, UpdateRequest{Results: []ModuleStatusUpdate{{
		ID:     thirdClaim.Claims[0].ID,
		Result: module.Result{Status: module.StatusFailed},
		Err:    errors.New("boom"),
	}}}); err != nil {
		t.Fatalf("update failure: %v", err)
	}
	state, err = repo.Load()
	if err != nil {
		t.Fatalf("load repo: %v", err)
	}
	if len(state.Runtime.Running) != 0 {
		t.Fatalf("expected running set empty after failure, got %+v", state.Runtime.Running)
	}
}

func TestEngineClaimFiltersRequestedModules(t *testing.T) {
	ctx := newTestModuleContext(t)
	def := workflow.WorkflowDefinition{
		ID: "fanout-workflow",
		Modules: []workflow.ModuleRef{
			{ID: "anchor-plan", ModuleID: "plan"},
			{ID: "module-build", ModuleID: "build", DependsOn: []string{"anchor-plan"}},
			{ID: "module-docs", ModuleID: "docs", DependsOn: []string{"anchor-plan"}},
		},
	}
	stubs := map[string]*stubModule{
		"plan":  newStubModule("plan"),
		"build": newStubModule("build"),
		"docs":  newStubModule("docs"),
	}
	stubs["plan"].setComplete(true)
	eng, repo := newCustomEngine(t, ctx, def, stubs)
	if _, err := eng.Start(ctx, StartRequest{Definition: def}); err != nil {
		t.Fatalf("start: %v", err)
	}
	claim, err := eng.Claim(ctx, ClaimRequest{Modules: []string{"module-docs"}, Limit: 2})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claim.Claims) != 1 || claim.Claims[0].ID != "module-docs" {
		t.Fatalf("expected single docs claim, got %+v", claim.Claims)
	}
	if len(claim.State.Runtime.Running) != 1 || claim.State.Runtime.Running[0] != "module-docs" {
		t.Fatalf("running set mismatch: %+v", claim.State.Runtime.Running)
	}
	if len(claim.State.Runnable) != 1 || claim.State.Runnable[0] != "module-build" {
		t.Fatalf("expected build to remain runnable, got %+v", claim.State.Runnable)
	}
	stored, err := repo.Load()
	if err != nil {
		t.Fatalf("load repo: %v", err)
	}
	if len(stored.Runtime.Running) != 1 || stored.Runtime.Running[0] != "module-docs" {
		t.Fatalf("persisted running set mismatch: %+v", stored.Runtime.Running)
	}
}

func TestEngineManualGateRequiresApproval(t *testing.T) {
	eng, _, ctx, stubs, def := newEngineHarness(t)
	stubs["plan"].setComplete(true)
	gate := map[string]scheduler.ManualGateState{
		"module-build": {Required: true, Approved: false, Note: "needs approval"},
	}
	state, err := eng.Start(ctx, StartRequest{Definition: def, Runtime: &RuntimeOverrides{ManualGates: &gate}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(state.Runnable) != 0 {
		t.Fatalf("expected no runnable modules while gate pending, got %+v", state.Runnable)
	}
	reason, ok := state.Skipped["module-build"]
	if !ok || reason.Reason != scheduler.SkipReasonManualGate {
		t.Fatalf("expected manual gate skip, got %+v", state.Skipped)
	}
	approved := map[string]scheduler.ManualGateState{
		"module-build": {Required: true, Approved: true},
	}
	state, err = eng.Update(ctx, UpdateRequest{Runtime: &RuntimeOverrides{ManualGates: &approved}})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(state.Runnable) != 1 || state.Runnable[0] != "module-build" {
		t.Fatalf("expected build runnable after approval, got %+v", state.Runnable)
	}
	if _, blocked := state.Skipped["module-build"]; blocked {
		t.Fatalf("expected manual gate cleared, got skips: %+v", state.Skipped)
	}
}

func TestEngineResumeHonorsTargetOverrides(t *testing.T) {
	eng, repo, ctx, stubs, def := newEngineHarness(t)
	stubs["plan"].setComplete(true)
	if _, err := eng.Start(ctx, StartRequest{Definition: def}); err != nil {
		t.Fatalf("start: %v", err)
	}
	stubs["build"].setComplete(true)
	targets := []string{"module-deploy"}
	batchSize := 1
	maxParallel := 1
	state, err := eng.Resume(ctx, ResumeRequest{Runtime: &RuntimeOverrides{
		Targets:     &targets,
		BatchSize:   &batchSize,
		MaxParallel: &maxParallel,
	}})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if len(state.Runnable) != 1 || state.Runnable[0] != "module-deploy" {
		t.Fatalf("expected deploy runnable, got %+v", state.Runnable)
	}
	if len(state.Runtime.Targets) != 1 || state.Runtime.Targets[0] != "module-deploy" {
		t.Fatalf("expected targets persisted, got %+v", state.Runtime.Targets)
	}
	if state.Runtime.BatchSize != 1 || state.Runtime.MaxParallel != 1 {
		t.Fatalf("runtime overrides missing: %+v", state.Runtime)
	}
	stored, err := repo.Load()
	if err != nil {
		t.Fatalf("load repo: %v", err)
	}
	if len(stored.Runtime.Targets) != 1 || stored.Runtime.Targets[0] != "module-deploy" {
		t.Fatalf("persisted targets mismatch: %+v", stored.Runtime.Targets)
	}
}

func findModule(state State, id string) ModuleStatus {
	for _, mod := range state.Nodes {
		if mod.ID == id {
			return mod
		}
	}
	return ModuleStatus{}
}

func newEngineHarness(t *testing.T) (*Engine, *Repository, *module.ModuleContext, map[string]*stubModule, workflow.WorkflowDefinition) {
	t.Helper()
	ctx := newTestModuleContext(t)
	repo := NewRepository(ctx.Workflow)
	reg := module.NewRegistry()
	stubs := map[string]*stubModule{
		"plan":   newStubModule("plan"),
		"build":  newStubModule("build"),
		"deploy": newStubModule("deploy"),
	}
	for id, stub := range stubs {
		stub := stub
		reg.MustRegister(id, func(module.Config) (module.Module, error) {
			return stub, nil
		})
	}
	clock := &testClock{value: time.Unix(0, 0)}
	eng, err := New(reg, repo, WithClock(clock.Now))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	def := workflow.WorkflowDefinition{
		ID: "test-workflow",
		Modules: []workflow.ModuleRef{
			{ID: "anchor-plan", ModuleID: "plan"},
			{ID: "module-build", ModuleID: "build", DependsOn: []string{"anchor-plan"}},
			{ID: "module-deploy", ModuleID: "deploy", DependsOn: []string{"module-build"}},
		},
	}
	return eng, repo, ctx, stubs, def
}

func newCustomEngine(t *testing.T, ctx *module.ModuleContext, def workflow.WorkflowDefinition, stubs map[string]*stubModule) (*Engine, *Repository) {
	reg := module.NewRegistry()
	for id, stub := range stubs {
		stub := stub
		id := id
		reg.MustRegister(id, func(module.Config) (module.Module, error) {
			return stub, nil
		})
	}
	repo := NewRepository(ctx.Workflow)
	clock := &testClock{value: time.Unix(0, 0)}
	eng, err := New(reg, repo, WithClock(clock.Now))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	return eng, repo
}

type testClock struct {
	value time.Time
}

func (c *testClock) Now() time.Time {
	c.value = c.value.Add(time.Second)
	return c.value
}

func newTestModuleContext(t *testing.T) *module.ModuleContext {
	t.Helper()
	tempDir := t.TempDir()
	cfg := &config.Config{ProjectDir: tempDir, LatticeProjectDir: filepath.Join(tempDir, ".lattice")}
	wf := workflow.New(cfg.LatticeProjectDir)
	return &module.ModuleContext{
		Config:    cfg,
		Workflow:  wf,
		Artifacts: artifact.NewStore(wf),
	}
}

type stubModule struct {
	info     module.Info
	complete bool
	err      error
	outputs  []artifact.ArtifactRef
}

func newStubModule(id string) *stubModule {
	return &stubModule{
		info: module.Info{
			ID:      id,
			Name:    "stub " + id,
			Version: "1.0.0",
		},
	}
}

func (m *stubModule) Info() module.Info { return m.info }

func (m *stubModule) Inputs() []artifact.ArtifactRef { return nil }

func (m *stubModule) Outputs() []artifact.ArtifactRef {
	if len(m.outputs) == 0 {
		return nil
	}
	out := make([]artifact.ArtifactRef, len(m.outputs))
	copy(out, m.outputs)
	return out
}

func (m *stubModule) IsComplete(*module.ModuleContext) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.complete, nil
}

func (m *stubModule) Run(*module.ModuleContext) (module.Result, error) {
	return module.Result{Status: module.StatusCompleted}, nil
}

func (m *stubModule) setComplete(value bool) {
	m.complete = value
}

func (m *stubModule) setOutputs(refs ...artifact.ArtifactRef) {
	m.outputs = append([]artifact.ArtifactRef{}, refs...)
}

func writeArtifact(t *testing.T, ctx *module.ModuleContext, ref artifact.ArtifactRef, moduleID string) {
	t.Helper()
	meta := artifact.Metadata{
		ArtifactID: ref.ID,
		ModuleID:   moduleID,
		Version:    "1.0.0",
		Workflow:   ctx.Workflow.Dir(),
	}
	if err := ctx.Artifacts.Write(ref, []byte("body"), meta); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
}
