package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/config"
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/workflow"
	"github.com/yourusername/lattice/internal/workflow/resolver"
)

func TestSchedulerReturnsConcurrentReadyNodes(t *testing.T) {
	stubs := map[string]*stubModule{
		"plan":  newStubModule("plan", true, nil),
		"build": newStubModule("build", false, nil),
		"docs":  newStubModule("docs", false, nil),
	}
	def := workflow.WorkflowDefinition{
		ID: "test",
		Modules: []workflow.ModuleRef{
			{ID: "anchor-plan", ModuleID: "plan"},
			{ID: "module-build", ModuleID: "build", DependsOn: []string{"anchor-plan"}},
			{ID: "module-docs", ModuleID: "docs", DependsOn: []string{"anchor-plan"}},
		},
	}
	sched := buildScheduler(t, stubs, def)
	batch, err := sched.Runnable(RunnableRequest{BatchSize: 2})
	if err != nil {
		t.Fatalf("runnable: %v", err)
	}
	if len(batch.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(batch.Nodes))
	}
	if batch.Nodes[0].ID != "module-build" || batch.Nodes[1].ID != "module-docs" {
		t.Fatalf("unexpected order: %v", []string{batch.Nodes[0].ID, batch.Nodes[1].ID})
	}
}

func TestSchedulerSkipsInvalidArtifacts(t *testing.T) {
	stubs := map[string]*stubModule{
		"plan":  newStubModule("plan", true, nil),
		"build": newStubModule("build", false, nil),
	}
	stubs["plan"].outputs = []artifact.ArtifactRef{artifact.ModulesDoc}
	def := workflow.WorkflowDefinition{
		ID: "test",
		Modules: []workflow.ModuleRef{
			{ID: "anchor-plan", ModuleID: "plan"},
			{ID: "module-build", ModuleID: "build", DependsOn: []string{"anchor-plan"}},
		},
	}
	res, ctx := buildResolverForTest(t, stubs, def)
	meta := artifact.Metadata{
		ArtifactID: artifact.ModulesDoc.ID,
		ModuleID:   "other-module",
		Version:    stubs["plan"].info.Version,
		Workflow:   ctx.Workflow.Dir(),
	}
	if err := ctx.Artifacts.Write(artifact.ModulesDoc, []byte("body"), meta); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if err := res.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	sched, err := New(res)
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	node, ok := res.Node("anchor-plan")
	if !ok {
		t.Fatalf("missing anchor-plan node")
	}
	report, ok := node.Artifacts[artifact.ModulesDoc.ID]
	if !ok {
		t.Fatalf("expected artifact report for modules doc")
	}
	if report.Status != module.ArtifactStatusInvalid {
		t.Fatalf("expected invalid artifact status, got %s", report.Status)
	}
	if node.State != resolver.NodeStateReady {
		t.Fatalf("expected anchor-plan marked ready for rerun, got %s", node.State)
	}
	batch, err := sched.Runnable(RunnableRequest{Targets: []string{"anchor-plan"}})
	if err != nil {
		t.Fatalf("runnable: %v", err)
	}
	if len(batch.Nodes) != 1 || batch.Nodes[0].ID != "anchor-plan" {
		t.Fatalf("expected anchor-plan to rerun, got %+v", batch.Nodes)
	}
	if len(batch.Skipped) != 0 {
		t.Fatalf("expected no skips for invalid artifact rerun, got %+v", batch.Skipped)
	}
}

func TestSchedulerHonorsManualGates(t *testing.T) {
	stubs := map[string]*stubModule{
		"plan":   newStubModule("plan", true, nil),
		"deploy": newStubModule("deploy", false, nil),
	}
	def := workflow.WorkflowDefinition{
		ID: "test",
		Modules: []workflow.ModuleRef{
			{ID: "anchor-plan", ModuleID: "plan"},
			{ID: "module-deploy", ModuleID: "deploy", DependsOn: []string{"anchor-plan"}},
		},
	}
	sched := buildScheduler(t, stubs, def)
	batch, err := sched.Runnable(RunnableRequest{ManualGates: map[string]ManualGateState{
		"module-deploy": {Required: true, Approved: false},
	}})
	if err != nil {
		t.Fatalf("runnable: %v", err)
	}
	if len(batch.Nodes) != 0 {
		t.Fatalf("expected no runnable nodes while gated, got %d", len(batch.Nodes))
	}
	reason, ok := batch.Skipped["module-deploy"]
	if !ok || reason.Reason != SkipReasonManualGate {
		t.Fatalf("expected manual gate skip, got %+v", reason)
	}
	batch, err = sched.Runnable(RunnableRequest{ManualGates: map[string]ManualGateState{
		"module-deploy": {Required: true, Approved: true},
	}})
	if err != nil {
		t.Fatalf("runnable: %v", err)
	}
	if len(batch.Nodes) != 1 || batch.Nodes[0].ID != "module-deploy" {
		t.Fatalf("expected deploy to run after approval, got %+v", batch.Nodes)
	}
}

func TestSchedulerEnforcesParallelLimit(t *testing.T) {
	stubs := map[string]*stubModule{
		"plan":  newStubModule("plan", true, nil),
		"build": newStubModule("build", false, nil),
		"docs":  newStubModule("docs", false, nil),
	}
	def := workflow.WorkflowDefinition{
		ID: "test",
		Modules: []workflow.ModuleRef{
			{ID: "anchor-plan", ModuleID: "plan"},
			{ID: "module-build", ModuleID: "build", DependsOn: []string{"anchor-plan"}},
			{ID: "module-docs", ModuleID: "docs", DependsOn: []string{"anchor-plan"}},
		},
	}
	sched := buildScheduler(t, stubs, def)
	batch, err := sched.Runnable(RunnableRequest{BatchSize: 2, MaxParallel: 1})
	if err != nil {
		t.Fatalf("runnable: %v", err)
	}
	if len(batch.Nodes) != 1 || batch.Nodes[0].ID != "module-build" {
		t.Fatalf("expected single runnable node respecting limit, got %+v", batch.Nodes)
	}
	batch, err = sched.Runnable(RunnableRequest{MaxParallel: 1, Running: []string{"module-build"}})
	if err != nil {
		t.Fatalf("runnable: %v", err)
	}
	if len(batch.Nodes) != 0 {
		t.Fatalf("expected zero runnable nodes when capacity exhausted")
	}
	if len(batch.Skipped) == 0 {
		t.Fatalf("expected concurrency skip reason when capacity exhausted")
	}
}

func buildScheduler(t *testing.T, stubs map[string]*stubModule, def workflow.WorkflowDefinition) *Scheduler {
	t.Helper()
	res, ctx := buildResolverForTest(t, stubs, def)
	if err := res.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	sched, err := New(res)
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	return sched
}

func buildResolverForTest(t *testing.T, stubs map[string]*stubModule, def workflow.WorkflowDefinition) (*resolver.Resolver, *module.ModuleContext) {
	t.Helper()
	reg := module.NewRegistry()
	for id, stub := range stubs {
		id := id
		stub := stub
		reg.MustRegister(id, func(module.Config) (module.Module, error) {
			return stub, nil
		})
	}
	res, err := resolver.New(def, reg)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}
	return res, newTestModuleContext(t)
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
	info         module.Info
	complete     bool
	err          error
	outputs      []artifact.ArtifactRef
	fingerprints map[string]string
}

func newStubModule(id string, complete bool, err error) *stubModule {
	return &stubModule{
		info:     module.Info{ID: id, Name: "stub " + id, Version: "1.0.0"},
		complete: complete,
		err:      err,
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

func (m *stubModule) ArtifactFingerprints(*module.ModuleContext) (map[string]string, error) {
	if len(m.fingerprints) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(m.fingerprints))
	for key, value := range m.fingerprints {
		out[key] = value
	}
	return out, nil
}
