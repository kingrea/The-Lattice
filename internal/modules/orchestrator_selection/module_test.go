package orchestrator_selection_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/config"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/modules/orchestrator_selection"
	"github.com/kingrea/The-Lattice/internal/workflow"
)

func TestOrchestratorSelectionRunWritesArtifacts(t *testing.T) {
	ctx := newOrchestratorModuleContext(t)
	seedPlanningArtifacts(t, ctx)
	seedCommunityCVs(t, ctx.Config, []agentStub{
		{Name: "Lyra", Precision: 7, Autonomy: 8, Experience: 6},
		{Name: "Cass", Precision: 8, Autonomy: 9, Experience: 9},
	})
	clockTime := time.Date(2026, 2, 4, 8, 0, 0, 0, time.UTC)
	mod := orchestrator_selection.New(orchestrator_selection.WithClock(func() time.Time { return clockTime }))
	result, err := mod.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != module.StatusCompleted {
		t.Fatalf("unexpected status: %+v", result)
	}
	if complete, err := mod.IsComplete(ctx); err != nil || !complete {
		if err != nil {
			t.Fatalf("IsComplete: %v", err)
		}
		t.Fatalf("expected completion")
	}

	orchMeta := readJSON(t, ctx.Workflow.OrchestratorPath())
	if got := orchMeta["name"].(string); got != "Cass" {
		t.Fatalf("expected Cass orchestrator, got %s", got)
	}
	selection := orchMeta["selection"].(map[string]any)
	if score := int(selection["score"].(float64)); score != (8*3 + 9*2 + 9) {
		t.Fatalf("unexpected selection score %d", score)
	}
	meta := orchMeta["_lattice"].(map[string]any)
	if meta["module"].(string) != "orchestrator-selection" {
		t.Fatalf("unexpected module metadata: %+v", meta)
	}
	if meta["version"].(string) != "1.0.0" {
		t.Fatalf("unexpected version metadata")
	}

	workersMeta := readJSON(t, ctx.Workflow.WorkersPath())
	orchestratorObj := workersMeta["orchestrator"].(map[string]any)
	if orchestratorObj["name"].(string) != "Cass" {
		t.Fatalf("unexpected roster orchestrator: %+v", orchestratorObj)
	}
	if workersMeta["updatedAt"].(string) != clockTime.Format(time.RFC3339) {
		t.Fatalf("unexpected updatedAt: %s", workersMeta["updatedAt"])
	}
	workersMetaBlock := workersMeta["_lattice"].(map[string]any)
	if workersMetaBlock["module"].(string) != "orchestrator-selection" {
		t.Fatalf("workers metadata missing module: %+v", workersMetaBlock)
	}
}

type agentStub struct {
	Name       string
	Precision  int
	Autonomy   int
	Experience int
}

func newOrchestratorModuleContext(t *testing.T) *module.ModuleContext {
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

func seedPlanningArtifacts(t *testing.T, ctx *module.ModuleContext) {
	t.Helper()
	writeDoc(t, ctx.Workflow, artifact.ModulesDoc)
	writeDoc(t, ctx.Workflow, artifact.ActionPlanDoc)
	touch(t, ctx.Workflow.ReviewsAppliedPath())
	touch(t, ctx.Workflow.BeadsCreatedPath())
}

func seedCommunityCVs(t *testing.T, cfg *config.Config, specs []agentStub) {
	t.Helper()
	communityRoot := filepath.Join(cfg.LatticeRoot, "communities", "atlas")
	if err := os.MkdirAll(filepath.Join(communityRoot, "cvs"), 0o755); err != nil {
		t.Fatalf("mkdir community cvs: %v", err)
	}
	for _, dir := range []string{"identities", "denizens"} {
		if err := os.MkdirAll(filepath.Join(communityRoot, dir), 0o755); err != nil {
			t.Fatalf("mkdir community dir: %v", err)
		}
	}
	communityConfig := `lattice:
  type: community
  version: 1
name: Atlas Collective
paths:
  identities: identities
  denizens: denizens
  cvs: cvs
`
	if err := os.WriteFile(filepath.Join(communityRoot, "community.yaml"), []byte(communityConfig), 0o644); err != nil {
		t.Fatalf("write community.yaml: %v", err)
	}
	for _, spec := range specs {
		slug := slugify(spec.Name)
		agentDir := filepath.Join(communityRoot, "cvs", slug)
		if err := os.MkdirAll(agentDir, 0o755); err != nil {
			t.Fatalf("mkdir agent dir: %v", err)
		}
		cvContent := fmt.Sprintf(`---
name: %s
byline: orchestrator
community: Atlas Collective
---
precision: %d
autonomy: %d
experience: %d
`, spec.Name, spec.Precision, spec.Autonomy, spec.Experience)
		if err := os.WriteFile(filepath.Join(agentDir, "cv.md"), []byte(cvContent), 0o644); err != nil {
			t.Fatalf("write cv: %v", err)
		}
	}
}

func writeDoc(t *testing.T, wf *workflow.Workflow, ref artifact.ArtifactRef) {
	t.Helper()
	path := ref.Path(wf)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir doc dir: %v", err)
	}
	meta := artifact.Metadata{
		ArtifactID: ref.ID,
		ModuleID:   "test",
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

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("touch marker: %v", err)
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	return payload
}

func slugify(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	return strings.ReplaceAll(trimmed, " ", "-")
}
