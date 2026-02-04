package work_process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/config"
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/orchestrator"
	"github.com/yourusername/lattice/internal/workflow"
)

func TestWorkProcessRunWritesArtifacts(t *testing.T) {
	ctx := newWorkProcessTestContext(t)
	seedWorkProcessInputs(t, ctx)
	sessions := []orchestrator.WorktreeSession{
		{
			Number: 1,
			Name:   "tree-1-alpha",
			Agent:  orchestrator.ProjectAgent{Name: "Aster"},
			Beads: []orchestrator.Bead{
				{ID: "task-1", Title: "Build feature", Points: 3},
			},
		},
	}
	runner := &stubCycleRunner{sessions: sessions}
	fixed := time.Date(2026, 2, 4, 10, 0, 0, 0, time.UTC)
	mod := New(WithRunner(runner), WithClock(func() time.Time { return fixed }))
	result, err := mod.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != module.StatusCompleted {
		t.Fatalf("unexpected status: %+v", result)
	}
	if !runner.executed {
		t.Fatalf("expected runner.Execute to be called")
	}
	ensureExists(t, artifact.WorkCompleteMarker.Path(ctx.Workflow))
	ensureMissing(t, artifact.WorkInProgressMarker.Path(ctx.Workflow))
	ensureMissing(t, artifact.RefinementNeededMarker.Path(ctx.Workflow))
	workLog := readDoc(t, artifact.WorkLogDoc.Path(ctx.Workflow))
	if !strings.Contains(workLog.body, "Aster") {
		t.Fatalf("work log missing agent entry: %s", workLog.body)
	}
	if workLog.meta.ModuleID != moduleID {
		t.Fatalf("work log metadata mismatch: %+v", workLog.meta)
	}
	tasks := readDoc(t, artifact.WorkTasksDoc.Path(ctx.Workflow))
	if !strings.Contains(tasks.body, "tree-1-alpha") {
		t.Fatalf("tasks doc missing session: %s", tasks.body)
	}
}

func TestWorkProcessRunNoReadyBeadsMarksRefinement(t *testing.T) {
	ctx := newWorkProcessTestContext(t)
	seedWorkProcessInputs(t, ctx)
	runner := &stubCycleRunner{prepareErr: orchestrator.ErrNoReadyBeads}
	mod := New(WithRunner(runner))
	result, err := mod.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != module.StatusNeedsInput {
		t.Fatalf("unexpected status: %+v", result)
	}
	ensureExists(t, artifact.RefinementNeededMarker.Path(ctx.Workflow))
	ensureMissing(t, artifact.WorkCompleteMarker.Path(ctx.Workflow))
	ensureMissing(t, artifact.WorkInProgressMarker.Path(ctx.Workflow))
}

func TestWorkProcessRunPropagatesRunnerError(t *testing.T) {
	ctx := newWorkProcessTestContext(t)
	seedWorkProcessInputs(t, ctx)
	sessions := []orchestrator.WorktreeSession{{
		Number: 1,
		Name:   "tree-err",
		Agent:  orchestrator.ProjectAgent{Name: "Kai"},
		Beads:  []orchestrator.Bead{{ID: "task-9", Title: "Investigate", Points: 2}},
	}}
	runner := &stubCycleRunner{
		sessions:   sessions,
		executeErr: fmt.Errorf("boom"),
	}
	mod := New(WithRunner(runner))
	result, err := mod.Run(ctx)
	if err == nil {
		t.Fatalf("expected error")
	}
	if result.Status != module.StatusFailed {
		t.Fatalf("unexpected status: %+v", result)
	}
	ensureMissing(t, artifact.WorkInProgressMarker.Path(ctx.Workflow))
	ensureMissing(t, artifact.WorkCompleteMarker.Path(ctx.Workflow))
}

type stubCycleRunner struct {
	sessions   []orchestrator.WorktreeSession
	prepareErr error
	executeErr error
	executed   bool
}

func (s *stubCycleRunner) Prepare(*module.ModuleContext) ([]orchestrator.WorktreeSession, error) {
	if s.prepareErr != nil {
		return nil, s.prepareErr
	}
	return append([]orchestrator.WorktreeSession(nil), s.sessions...), nil
}

func (s *stubCycleRunner) Execute(context.Context, *module.ModuleContext, []orchestrator.WorktreeSession) error {
	s.executed = true
	if s.executeErr != nil {
		return s.executeErr
	}
	return nil
}

func newWorkProcessTestContext(t *testing.T) *module.ModuleContext {
	t.Helper()
	projectDir := t.TempDir()
	if err := config.InitLatticeDir(projectDir); err != nil {
		t.Fatalf("init lattice dir: %v", err)
	}
	cfg := &config.Config{
		ProjectDir:        projectDir,
		LatticeRoot:       projectDir,
		LatticeProjectDir: filepath.Join(projectDir, config.LatticeDir),
	}
	wf := workflow.New(cfg.LatticeProjectDir)
	if err := wf.Initialize(); err != nil {
		t.Fatalf("initialize workflow: %v", err)
	}
	return &module.ModuleContext{
		Config:    cfg,
		Workflow:  wf,
		Artifacts: artifact.NewStore(wf),
	}
}

func seedWorkProcessInputs(t *testing.T, ctx *module.ModuleContext) {
	writeJSONArtifact(t, ctx, artifact.WorkersJSON, []byte(`{"workers":[{"name":"Aster"}]}`))
	writeJSONArtifact(t, ctx, artifact.OrchestratorState, []byte(`{"name":"Aster"}`))
	if err := ctx.Artifacts.Write(artifact.BeadsCreatedMarker, nil, artifact.Metadata{}); err != nil {
		t.Fatalf("write beads marker: %v", err)
	}
}

func writeJSONArtifact(t *testing.T, ctx *module.ModuleContext, ref artifact.ArtifactRef, body []byte) {
	t.Helper()
	meta := artifact.Metadata{
		ArtifactID: ref.ID,
		ModuleID:   "test",
		Version:    "0.0.1",
		Workflow:   ctx.Workflow.Dir(),
	}
	if err := ctx.Artifacts.Write(ref, body, meta); err != nil {
		t.Fatalf("write %s: %v", ref.ID, err)
	}
}

type docPayload struct {
	meta artifact.Metadata
	body string
}

func readDoc(t *testing.T, path string) docPayload {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read doc: %v", err)
	}
	meta, body, err := artifact.ParseFrontMatter(data)
	if err != nil {
		t.Fatalf("parse frontmatter: %v", err)
	}
	return docPayload{meta: meta, body: string(body)}
}

func ensureExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func ensureMissing(t *testing.T, path string) {
	t.Helper()
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected %s to be absent", path)
	}
}
