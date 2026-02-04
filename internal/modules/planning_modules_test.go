package modules_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/config"
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/modules/action_plan"
	"github.com/yourusername/lattice/internal/modules/bead_creation"
	"github.com/yourusername/lattice/internal/modules/consolidation"
	"github.com/yourusername/lattice/internal/modules/parallel_reviews"
	"github.com/yourusername/lattice/internal/modules/staff_incorporate"
	"github.com/yourusername/lattice/internal/modules/staff_review"
	"github.com/yourusername/lattice/internal/workflow"
)

func TestActionPlanModuleWritesMetadata(t *testing.T) {
	ctx := newTestContext(t)
	mod := action_plan.New()
	writeDoc(t, ctx.Workflow, artifact.ModulesDoc)
	writeDoc(t, ctx.Workflow, artifact.ActionPlanDoc)

	for i := 0; i < 2; i++ {
		if _, err := mod.IsComplete(ctx); err != nil {
			t.Fatalf("IsComplete pass %d: %v", i, err)
		}
	}

	meta := readMetadata(t, ctx.Workflow.ModulesPath())
	if meta.ModuleID != "action-plan" {
		t.Fatalf("expected module-id action-plan, got %s", meta.ModuleID)
	}

	if complete, err := mod.IsComplete(ctx); err != nil || !complete {
		if err != nil {
			t.Fatalf("IsComplete second pass: %v", err)
		}
		t.Fatalf("expected complete after metadata available")
	}
}

func TestStaffReviewModuleWritesMetadata(t *testing.T) {
	ctx := newTestContext(t)
	mod := staff_review.New()
	writeDoc(t, ctx.Workflow, artifact.StaffReviewDoc)

	if _, err := mod.IsComplete(ctx); err != nil {
		t.Fatalf("IsComplete: %v", err)
	}

	meta := readMetadata(t, ctx.Workflow.StaffReviewPath())
	if meta.ModuleID != "staff-review" {
		t.Fatalf("unexpected module id %s", meta.ModuleID)
	}
}

func TestStaffIncorporateModuleRequiresMarker(t *testing.T) {
	ctx := newTestContext(t)
	mod := staff_incorporate.New()
	writeDoc(t, ctx.Workflow, artifact.ModulesDoc)
	writeDoc(t, ctx.Workflow, artifact.ActionPlanDoc)

	if _, err := mod.IsComplete(ctx); err != nil {
		t.Fatalf("IsComplete: %v", err)
	}

	touch(t, ctx.Workflow.StaffFeedbackAppliedPath())
	if complete, err := mod.IsComplete(ctx); err != nil || !complete {
		if err != nil {
			t.Fatalf("IsComplete after marker: %v", err)
		}
		t.Fatalf("expected completion after marker exists")
	}
}

func TestParallelReviewsModuleWritesMetadata(t *testing.T) {
	ctx := newTestContext(t)
	mod := parallel_reviews.New()
	refs := []artifact.ArtifactRef{
		artifact.ReviewPragmatistDoc,
		artifact.ReviewSimplifierDoc,
		artifact.ReviewAdvocateDoc,
		artifact.ReviewSkepticDoc,
	}
	for _, ref := range refs {
		writeDoc(t, ctx.Workflow, ref)
	}

	if complete, err := mod.IsComplete(ctx); err != nil || complete {
		if err != nil {
			t.Fatalf("IsComplete: %v", err)
		}
		t.Fatalf("expected incomplete while metadata fixing runs")
	}

	meta := readMetadata(t, refs[0].Path(ctx.Workflow))
	if meta.ModuleID != "parallel-reviews" {
		t.Fatalf("unexpected module id %s", meta.ModuleID)
	}
}

func TestConsolidationModuleRequiresMarker(t *testing.T) {
	ctx := newTestContext(t)
	mod := consolidation.New()
	writeDoc(t, ctx.Workflow, artifact.ModulesDoc)
	writeDoc(t, ctx.Workflow, artifact.ActionPlanDoc)

	if complete, err := mod.IsComplete(ctx); err != nil || complete {
		if err != nil {
			t.Fatalf("IsComplete: %v", err)
		}
		t.Fatalf("expected consolidation to wait for marker")
	}

	meta := readMetadata(t, ctx.Workflow.ModulesPath())
	if meta.ModuleID != "consolidation" {
		t.Fatalf("unexpected module id %s", meta.ModuleID)
	}

	touch(t, ctx.Workflow.ReviewsAppliedPath())
	if complete, err := mod.IsComplete(ctx); err != nil || !complete {
		if err != nil {
			t.Fatalf("IsComplete after marker: %v", err)
		}
		t.Fatalf("expected completion once marker exists")
	}
}

func TestBeadCreationModuleRequiresMarker(t *testing.T) {
	ctx := newTestContext(t)
	mod := bead_creation.New()

	if complete, err := mod.IsComplete(ctx); err != nil || complete {
		if err != nil {
			t.Fatalf("IsComplete: %v", err)
		}
		t.Fatalf("expected bead creation to wait for marker")
	}

	touch(t, ctx.Workflow.BeadsCreatedPath())
	if complete, err := mod.IsComplete(ctx); err != nil || !complete {
		if err != nil {
			t.Fatalf("IsComplete after marker: %v", err)
		}
		t.Fatalf("expected completion once beads marker exists")
	}
}

func newTestContext(t *testing.T) *module.ModuleContext {
	t.Helper()
	projectDir := t.TempDir()
	if err := config.InitLatticeDir(projectDir); err != nil {
		t.Fatalf("init lattice dir: %v", err)
	}
	cfg := &config.Config{
		ProjectDir:        projectDir,
		LatticeProjectDir: filepath.Join(projectDir, config.LatticeDir),
		LatticeRoot:       projectDir,
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

func writeDoc(t *testing.T, wf *workflow.Workflow, ref artifact.ArtifactRef) {
	t.Helper()
	path := ref.Path(wf)
	if path == "" {
		t.Fatalf("missing path for %s", ref.ID)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	meta := artifact.Metadata{
		ArtifactID: ref.ID,
		ModuleID:   "placeholder",
		Version:    "0.0.0",
		Workflow:   wf.Dir(),
	}
	content, err := artifact.WriteFrontMatter(meta, []byte("body"))
	if err != nil {
		t.Fatalf("write frontmatter: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
}

func readMetadata(t *testing.T, path string) artifact.Metadata {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	meta, _, err := artifact.ParseFrontMatter(data)
	if err != nil {
		t.Fatalf("parse frontmatter: %v", err)
	}
	return meta
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("touch marker: %v", err)
	}
}
