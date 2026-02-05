package plugins

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/config"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/workflow"
)

func TestNewSkillModule(t *testing.T) {
	def := ModuleDefinition{
		ID:      "skill-module",
		Name:    "Skill Module",
		Version: "1.0.0",
		Skill: SkillDefinition{
			Slug:   "lattice-planning",
			Prompt: "Load {{.SkillPath}}",
		},
		Outputs: []ArtifactBinding{{Artifact: "commission-doc"}},
	}
	m, err := newSkillModule(def, nil)
	if err != nil {
		t.Fatalf("newSkillModule: %v", err)
	}
	if m.Info().ID != "skill-module" || len(m.Outputs()) != 1 {
		t.Fatalf("unexpected module info: %+v", m.Info())
	}
}

func TestRenderPrompt(t *testing.T) {
	m := mustSkillModule(t)
	ctx := newTestContext(t)
	text, err := m.renderPrompt("/tmp/skill", ctx)
	if err != nil {
		t.Fatalf("render prompt: %v", err)
	}
	if text == "" || text != "Load /tmp/skill" {
		t.Fatalf("unexpected prompt: %s", text)
	}
}

func TestResolveSkillPathFromSlug(t *testing.T) {
	m := mustSkillModule(t)
	ctx := newTestContext(t)
	path, err := m.resolveSkillPath(ctx)
	if err != nil {
		t.Fatalf("resolve skill path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected skill path to exist: %v", err)
	}
}

func TestMergeConfigs(t *testing.T) {
	base := module.Config{"foo": "bar"}
	over := module.Config{"foo": "override", "baz": 42}
	merged := mergeConfigs(base, over)
	if merged["foo"].(string) != "override" || merged["baz"].(int) != 42 {
		t.Fatalf("unexpected merge: %#v", merged)
	}
}

func TestSanitizeWindowName(t *testing.T) {
	if got := sanitizeWindowName("My Window!"); got != "My-Window" {
		t.Fatalf("unexpected sanitize result: %s", got)
	}
}

func mustSkillModule(t *testing.T) *skillModule {
	def := ModuleDefinition{
		ID:      "skill-module",
		Name:    "Skill Module",
		Version: "1.0.0",
		Skill: SkillDefinition{
			Slug:   "create-agent-file",
			Prompt: "Load {{.SkillPath}}",
		},
		Outputs: []ArtifactBinding{{Artifact: "commission-doc"}},
	}
	m, err := newSkillModule(def, nil)
	if err != nil {
		t.Fatalf("newSkillModule: %v", err)
	}
	return m
}

func newTestContext(t *testing.T) *module.ModuleContext {
	t.Helper()
	root := t.TempDir()
	if err := config.InitLatticeDir(root); err != nil {
		t.Fatalf("init lattice: %v", err)
	}
	cfg := &config.Config{
		ProjectDir:        root,
		LatticeProjectDir: filepath.Join(root, ".lattice"),
	}
	wf := workflow.New(cfg.LatticeProjectDir)
	ctx := &module.ModuleContext{
		Config:    cfg,
		Workflow:  wf,
		Artifacts: artifact.NewStore(wf),
	}
	return ctx
}
