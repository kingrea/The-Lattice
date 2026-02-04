package resolver

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/config"
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/workflow"
)

func TestResolverRefreshSetsStates(t *testing.T) {
	stubs := map[string]*stubModule{
		"plan":   newStubModule("plan", true, nil),
		"build":  newStubModule("build", false, nil),
		"deploy": newStubModule("deploy", false, nil),
	}
	resolver := buildResolver(t, stubs)
	ctx := newTestModuleContext(t)

	if err := resolver.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	plan := mustNode(t, resolver, "anchor-plan")
	build := mustNode(t, resolver, "module-build")
	deploy := mustNode(t, resolver, "module-deploy")

	if plan.State != NodeStateComplete {
		t.Fatalf("expected plan complete, got %s", plan.State)
	}
	if build.State != NodeStateReady {
		t.Fatalf("expected build ready, got %s", build.State)
	}
	if deploy.State != NodeStateBlocked {
		t.Fatalf("expected deploy blocked, got %s", deploy.State)
	}
	if len(deploy.BlockedBy) != 1 || deploy.BlockedBy[0] != "module-build" {
		t.Fatalf("deploy blocked by %+v", deploy.BlockedBy)
	}

	ready := resolver.Ready()
	if len(ready) != 1 || ready[0].ID != "module-build" {
		t.Fatalf("unexpected ready set: %#v", ready)
	}
}

func TestResolverQueueTargetsOrdersDependencies(t *testing.T) {
	stubs := map[string]*stubModule{
		"plan":   newStubModule("plan", false, nil),
		"build":  newStubModule("build", false, nil),
		"deploy": newStubModule("deploy", false, nil),
	}
	resolver := buildResolver(t, stubs)
	ctx := newTestModuleContext(t)

	if err := resolver.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	queue, err := resolver.Queue("module-deploy")
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if len(queue) != 3 {
		t.Fatalf("expected 3 queued modules, got %d", len(queue))
	}
	if queue[0].ID != "anchor-plan" || queue[1].ID != "module-build" || queue[2].ID != "module-deploy" {
		t.Fatalf("unexpected order: %s -> %s -> %s", queue[0].ID, queue[1].ID, queue[2].ID)
	}
}

func TestResolverRefreshPropagatesErrors(t *testing.T) {
	stubs := map[string]*stubModule{
		"plan":   newStubModule("plan", true, nil),
		"build":  newStubModule("build", false, errors.New("boom")),
		"deploy": newStubModule("deploy", false, nil),
	}
	resolver := buildResolver(t, stubs)
	ctx := newTestModuleContext(t)

	if err := resolver.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	build := mustNode(t, resolver, "module-build")
	if build.State != NodeStateError {
		t.Fatalf("expected build error state, got %s", build.State)
	}
	if build.Err == nil || build.Err.Error() != "boom" {
		t.Fatalf("unexpected build error: %v", build.Err)
	}
	deploy := mustNode(t, resolver, "module-deploy")
	if deploy.State != NodeStateBlocked {
		t.Fatalf("expected deploy blocked by error, got %s", deploy.State)
	}
	if len(deploy.BlockedBy) != 1 || deploy.BlockedBy[0] != "module-build" {
		t.Fatalf("unexpected deploy blockers: %+v", deploy.BlockedBy)
	}
}

func TestResolverCheckArtifactFingerprintFresh(t *testing.T) {
	stubs := map[string]*stubModule{
		"plan":   newStubModule("plan", true, nil),
		"build":  newStubModule("build", false, nil),
		"deploy": newStubModule("deploy", false, nil),
	}
	fingerprint := "abc123"
	stubs["plan"].outputs = []artifact.ArtifactRef{artifact.ModulesDoc}
	stubs["plan"].fingerprints = map[string]string{artifact.ModulesDoc.ID: fingerprint}
	res := buildResolver(t, stubs)
	ctx := newTestModuleContext(t)
	meta := artifact.Metadata{
		ArtifactID: artifact.ModulesDoc.ID,
		ModuleID:   stubs["plan"].info.ID,
		Version:    stubs["plan"].info.Version,
		Workflow:   ctx.Workflow.Dir(),
		Notes: map[string]string{
			module.FingerprintNoteKey(artifact.ModulesDoc.ID): fingerprint,
		},
	}
	if err := ctx.Artifacts.Write(artifact.ModulesDoc, []byte("body"), meta); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if err := res.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	plan := mustNode(t, res, "anchor-plan")
	report, ok := plan.Artifacts[artifact.ModulesDoc.ID]
	if !ok {
		t.Fatalf("missing artifact report")
	}
	if report.Status != module.ArtifactStatusFresh {
		t.Fatalf("expected fresh artifact, got %s", report.Status)
	}
	if plan.State != NodeStateComplete {
		t.Fatalf("expected plan complete, got %s", plan.State)
	}
}

func TestResolverCheckArtifactFingerprintMismatch(t *testing.T) {
	stubs := map[string]*stubModule{
		"plan":   newStubModule("plan", true, nil),
		"build":  newStubModule("build", false, nil),
		"deploy": newStubModule("deploy", false, nil),
	}
	stubs["plan"].outputs = []artifact.ArtifactRef{artifact.ModulesDoc}
	stubs["plan"].fingerprints = map[string]string{artifact.ModulesDoc.ID: "new"}
	res := buildResolver(t, stubs)
	ctx := newTestModuleContext(t)
	meta := artifact.Metadata{
		ArtifactID: artifact.ModulesDoc.ID,
		ModuleID:   stubs["plan"].info.ID,
		Version:    stubs["plan"].info.Version,
		Workflow:   ctx.Workflow.Dir(),
		Notes: map[string]string{
			module.FingerprintNoteKey(artifact.ModulesDoc.ID): "old",
		},
	}
	if err := ctx.Artifacts.Write(artifact.ModulesDoc, []byte("body"), meta); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if err := res.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	plan := mustNode(t, res, "anchor-plan")
	report := plan.Artifacts[artifact.ModulesDoc.ID]
	if report.Status != module.ArtifactStatusOutdated {
		t.Fatalf("expected outdated, got %s", report.Status)
	}
	if plan.State == NodeStateComplete {
		t.Fatalf("expected plan to be rerun due to invalid artifact")
	}
	if len(stubs["plan"].invalidations) != 1 {
		t.Fatalf("expected invalidation event")
	}
	event := stubs["plan"].invalidations[0]
	if event.Reason != module.InvalidationReasonFingerprint {
		t.Fatalf("unexpected invalidation reason: %s", event.Reason)
	}
}

func buildResolver(t *testing.T, stubs map[string]*stubModule) *Resolver {
	t.Helper()
	reg := module.NewRegistry()
	for id, stub := range stubs {
		id := id
		stub := stub
		reg.MustRegister(id, func(module.Config) (module.Module, error) {
			return stub, nil
		})
	}
	def := workflow.WorkflowDefinition{
		ID: "test-workflow",
		Modules: []workflow.ModuleRef{
			{ID: "anchor-plan", ModuleID: "plan"},
			{ID: "module-build", ModuleID: "build", DependsOn: []string{"anchor-plan"}},
			{ID: "module-deploy", ModuleID: "deploy", DependsOn: []string{"module-build"}},
		},
	}
	resolver, err := New(def, reg)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}
	return resolver
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

func mustNode(t *testing.T, resolver *Resolver, id string) *Node {
	t.Helper()
	node, ok := resolver.Node(id)
	if !ok {
		t.Fatalf("missing node %s", id)
	}
	return node
}

type stubModule struct {
	info          module.Info
	complete      bool
	err           error
	outputs       []artifact.ArtifactRef
	fingerprints  map[string]string
	invalidations []module.ArtifactInvalidation
}

func newStubModule(id string, complete bool, err error) *stubModule {
	return &stubModule{
		info: module.Info{
			ID:      id,
			Name:    "stub " + id,
			Version: "1.0.0",
		},
		complete: complete,
		err:      err,
	}
}

func (m *stubModule) Info() module.Info {
	return m.info
}

func (m *stubModule) Inputs() []artifact.ArtifactRef {
	return nil
}

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

func (m *stubModule) OnArtifactInvalidation(_ *module.ModuleContext, event module.ArtifactInvalidation) error {
	m.invalidations = append(m.invalidations, event)
	return nil
}
