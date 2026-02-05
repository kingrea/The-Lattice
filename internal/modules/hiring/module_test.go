package hiring

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
	"github.com/kingrea/The-Lattice/internal/orchestrator"
	"github.com/kingrea/The-Lattice/internal/workflow"
)

func TestHiringModuleRunWritesRosterAndAgents(t *testing.T) {
	ctx := newHiringTestContext(t)
	seedPlanningArtifacts(t, ctx)
	seedOrchestratorState(t, ctx)
	seedCommunityCVs(t, ctx.Config, []agentFixture{
		{Name: "Lyra", Precision: 7, Autonomy: 8, Experience: 6},
		{Name: "Cass", Precision: 8, Autonomy: 9, Experience: 9},
		{Name: "Mira", Precision: 6, Autonomy: 7, Experience: 8},
		{Name: "Noor", Precision: 5, Autonomy: 8, Experience: 7},
	})
	ctx.Orchestrator = orchestrator.New(ctx.Config)
	fixTime := time.Date(2026, 2, 4, 9, 0, 0, 0, time.UTC)
	runner := &fakeCommandRunner{}
	agentWriter := func(_ *module.ModuleContext, entry workflow.WorkerEntry, _ string, targetFile, roleContext string) error {
		content := fmt.Sprintf("# %s\nRole: %s\n", entry.Name, roleContext)
		return os.WriteFile(targetFile, []byte(content), 0o644)
	}
	mod := New(
		WithCommandRunner(runner.Run),
		WithAgentBriefWriter(agentWriter),
		WithClock(func() time.Time { return fixTime }),
	)
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
		t.Fatalf("expected module completion")
	}
	payload := readJSONFile(t, ctx.Workflow.WorkersPath())
	meta := payload["_lattice"].(map[string]any)
	if meta["module"].(string) != moduleID {
		t.Fatalf("workers.json metadata module mismatch: %+v", meta)
	}
	workers := payload["workers"].([]any)
	if len(workers) == 0 {
		t.Fatalf("expected workers in roster")
	}
	analysis := payload["analysis"].(map[string]any)
	if int(analysis["sparkCount"].(float64)) == 0 {
		t.Fatalf("expected SPARK hires in analysis: %+v", analysis)
	}
	if int(analysis["totalHires"].(float64)) != len(workers) {
		t.Fatalf("analysis totalHires mismatch: %+v", analysis)
	}
	first := workers[0].(map[string]any)["name"].(string)
	slug := slugifyName(first)
	agentPath := filepath.Join(ctx.Config.AgentsDir(), "workers", slug, "AGENT.md")
	supPath := filepath.Join(ctx.Config.AgentsDir(), "workers", slug, "AGENT_SUP.md")
	if _, err := os.Stat(agentPath); err != nil {
		t.Fatalf("agent file missing: %v", err)
	}
	supBody, err := os.ReadFile(supPath)
	if err != nil {
		t.Fatalf("support packet missing: %v", err)
	}
	if !strings.Contains(string(supBody), "Support Packet") {
		t.Fatalf("support packet missing header: %s", supBody)
	}
	expectedCreates := len(workers) + 1 // hire epic + per-agent beads
	if runner.createCount != expectedCreates {
		t.Fatalf("bd create calls: got %d want %d", runner.createCount, expectedCreates)
	}
	if runner.readyCount != 1 {
		t.Fatalf("bd ready call count mismatch: %d", runner.readyCount)
	}
}

type fakeCommandRunner struct {
	createCount int
	readyCount  int
}

func (f *fakeCommandRunner) Run(_ string, name string, args ...string) ([]byte, error) {
	if name != "bd" {
		return nil, fmt.Errorf("unexpected command %s", name)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("missing args")
	}
	switch args[0] {
	case "ready":
		f.readyCount++
		return []byte(`[{"id":"task-1","points":5}]`), nil
	case "create":
		f.createCount++
		payload := fmt.Sprintf(`{"id":"bead-%d"}`, f.createCount)
		return []byte(payload), nil
	default:
		return nil, fmt.Errorf("unsupported bd command: %s", args[0])
	}
}

type agentFixture struct {
	Name       string
	Precision  int
	Autonomy   int
	Experience int
}

func newHiringTestContext(t *testing.T) *module.ModuleContext {
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
	writeDoc(t, ctx.Workflow, artifact.ModulesDoc)
	writeDoc(t, ctx.Workflow, artifact.ActionPlanDoc)
	touch(t, ctx.Workflow.BeadsCreatedPath())
}

func seedOrchestratorState(t *testing.T, ctx *module.ModuleContext) {
	payload := map[string]any{
		"name":      "Cass",
		"community": "Atlas Collective",
		"cvPath":    filepath.Join(ctx.Config.CVsDir(), "atlas", "Cass", "cv.md"),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal orchestrator state: %v", err)
	}
	meta := artifact.Metadata{
		ArtifactID: artifact.OrchestratorState.ID,
		ModuleID:   "orchestrator-selection",
		Version:    "1.0.0",
		Workflow:   ctx.Workflow.Dir(),
	}
	if err := ctx.Artifacts.Write(artifact.OrchestratorState, body, meta); err != nil {
		t.Fatalf("write orchestrator state: %v", err)
	}
}

func seedCommunityCVs(t *testing.T, cfg *config.Config, specs []agentFixture) {
	communityRoot := filepath.Join(cfg.LatticeRoot, "communities", "atlas")
	for _, dir := range []string{"cvs", "identities", "denizens"} {
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
		slug := slugifyName(spec.Name)
		agentDir := filepath.Join(communityRoot, "cvs", slug)
		if err := os.MkdirAll(agentDir, 0o755); err != nil {
			t.Fatalf("mkdir agent dir: %v", err)
		}
		cv := fmt.Sprintf(`---
name: %s
byline: worker
community: Atlas Collective
---
precision: %d
autonomy: %d
experience: %d
`, spec.Name, spec.Precision, spec.Autonomy, spec.Experience)
		if err := os.WriteFile(filepath.Join(agentDir, "cv.md"), []byte(cv), 0o644); err != nil {
			t.Fatalf("write cv: %v", err)
		}
	}
}

func writeDoc(t *testing.T, wf *workflow.Workflow, ref artifact.ArtifactRef) {
	path := ref.Path(wf)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir artifact dir: %v", err)
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("touch marker: %v", err)
	}
}

func readJSONFile(t *testing.T, path string) map[string]any {
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
