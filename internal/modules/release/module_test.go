package release

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/config"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/workflow"
)

func TestReleaseRunProducesNotesAndMarkers(t *testing.T) {
	ctx := newReleaseTestContext(t)
	seedReleaseInputs(t, ctx)
	logsFile := filepath.Join(ctx.Config.LogsDir(), "run.log")
	if err := os.WriteFile(logsFile, []byte("log"), 0o644); err != nil {
		t.Fatalf("seed logs: %v", err)
	}
	worktreeFile := filepath.Join(ctx.Config.WorktreeDir(), "state.txt")
	if err := os.WriteFile(worktreeFile, []byte("state"), 0o644); err != nil {
		t.Fatalf("seed worktree: %v", err)
	}
	fixed := time.Date(2026, 2, 4, 10, 0, 0, 0, time.UTC)
	beads := []beadSummary{{ID: "lattice-123", Title: "Follow-up", Priority: 1, Status: "open"}}
	mod := New(WithClock(func() time.Time { return fixed }), WithBeadLister(stubBeadLister{beads: beads}))
	result, err := mod.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != module.StatusCompleted {
		t.Fatalf("unexpected status: %+v", result)
	}
	releaseNotes := artifact.ReleaseNotesDoc.Path(ctx.Workflow)
	ensureExists(t, releaseNotes)
	data, err := os.ReadFile(releaseNotes)
	if err != nil {
		t.Fatalf("read release notes: %v", err)
	}
	meta, body, err := artifact.ParseFrontMatter(data)
	if err != nil {
		t.Fatalf("parse release notes: %v", err)
	}
	if meta.ModuleID != moduleID {
		t.Fatalf("unexpected module metadata: %+v", meta)
	}
	if !strings.Contains(string(body), "Follow-up") {
		t.Fatalf("release notes missing bead summary: %s", body)
	}
	packagesRoot := artifact.ReleasePackagesDir.Path(ctx.Workflow)
	expectedPackage := filepath.Join(packagesRoot, "20260204-100000")
	ensureDirExists(t, expectedPackage)
	ensureExists(t, filepath.Join(expectedPackage, "work-log.md"))
	ensureExists(t, filepath.Join(expectedPackage, "logs", "run.log"))
	ensureExists(t, artifact.AgentsReleasedMarker.Path(ctx.Workflow))
	ensureExists(t, artifact.CleanupDoneMarker.Path(ctx.Workflow))
	ensureExists(t, artifact.OrchestratorReleasedMarker.Path(ctx.Workflow))
	workersData, err := os.ReadFile(ctx.Config.WorkerListPath())
	if err != nil {
		t.Fatalf("read workers reset: %v", err)
	}
	if strings.TrimSpace(string(workersData)) != "[]" {
		t.Fatalf("workers manifest not reset: %s", workersData)
	}
	if entries, err := os.ReadDir(ctx.Config.LogsDir()); err != nil || len(entries) != 0 {
		t.Fatalf("logs dir not cleared: %v entries=%d", err, len(entries))
	}
	if _, err := os.Stat(ctx.Workflow.OrchestratorPath()); !os.IsNotExist(err) {
		t.Fatalf("orchestrator config still present")
	}
}

func TestReleaseRunRequiresCompleteMarker(t *testing.T) {
	ctx := newReleaseTestContext(t)
	writeDocArtifact(t, ctx, artifact.WorkLogDoc, "# work\n")
	writeJSONArtifact(t, ctx, artifact.WorkersJSON, []byte(`{"workers":[]}`))
	writeJSONArtifact(t, ctx, artifact.OrchestratorState, []byte(`{"name":"Vela"}`))
	mod := New(WithBeadLister(stubBeadLister{}))
	result, err := mod.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != module.StatusNeedsInput {
		t.Fatalf("expected needs-input got %+v", result)
	}
}

func TestReleaseRunFailsWhenPackagesDirBlocked(t *testing.T) {
	ctx := newReleaseTestContext(t)
	seedReleaseInputs(t, ctx)
	blocked := artifact.ReleasePackagesDir.Path(ctx.Workflow)
	if err := os.MkdirAll(ctx.Workflow.ReleaseDir(), 0o755); err != nil {
		t.Fatalf("mkdir release: %v", err)
	}
	if err := os.WriteFile(blocked, []byte("blocked"), 0o644); err != nil {
		t.Fatalf("seed block: %v", err)
	}
	mod := New(WithBeadLister(stubBeadLister{}))
	result, err := mod.Run(ctx)
	if err == nil {
		t.Fatalf("expected error")
	}
	if result.Status != module.StatusFailed {
		t.Fatalf("expected failed status got %+v", result)
	}
}

func newReleaseTestContext(t *testing.T) *module.ModuleContext {
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
		t.Fatalf("init workflow: %v", err)
	}
	ctx := &module.ModuleContext{
		Config:    cfg,
		Workflow:  wf,
		Artifacts: artifact.NewStore(wf),
	}
	return ctx
}

func seedReleaseInputs(t *testing.T, ctx *module.ModuleContext) {
	writeDocArtifact(t, ctx, artifact.WorkLogDoc, "## Cycle\n- Work done")
	writeDocArtifact(t, ctx, artifact.AuditSynthesisDoc, "## Audit\n- Fixes")
	writeJSONArtifact(t, ctx, artifact.WorkersJSON, []byte(`{"workers":[{"name":"Aster"},{"name":"Kai"}]}`))
	writeJSONArtifact(t, ctx, artifact.OrchestratorState, []byte(`{"name":"Nova"}`))
	if err := ctx.Artifacts.Write(artifact.WorkCompleteMarker, nil, artifact.Metadata{}); err != nil {
		t.Fatalf("write complete marker: %v", err)
	}
}

func writeDocArtifact(t *testing.T, ctx *module.ModuleContext, ref artifact.ArtifactRef, body string) {
	meta := artifact.Metadata{ArtifactID: ref.ID, ModuleID: "test", Version: "0.0.1", Workflow: ctx.Workflow.Dir()}
	if err := ctx.Artifacts.Write(ref, []byte(body), meta); err != nil {
		t.Fatalf("write %s: %v", ref.ID, err)
	}
}

func writeJSONArtifact(t *testing.T, ctx *module.ModuleContext, ref artifact.ArtifactRef, body []byte) {
	meta := artifact.Metadata{ArtifactID: ref.ID, ModuleID: "test", Version: "0.0.1", Workflow: ctx.Workflow.Dir()}
	if err := ctx.Artifacts.Write(ref, body, meta); err != nil {
		t.Fatalf("write json %s: %v", ref.ID, err)
	}
}

func ensureExists(t *testing.T, path string) {
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func ensureDirExists(t *testing.T, path string) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected dir %s: %v", path, err)
	}
}

type stubBeadLister struct {
	beads []beadSummary
	err   error
}

func (s stubBeadLister) Ready(context.Context) ([]beadSummary, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]beadSummary(nil), s.beads...), nil
}
