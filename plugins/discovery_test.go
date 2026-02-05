package plugins

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kingrea/The-Lattice/internal/config"
	"github.com/kingrea/The-Lattice/internal/module"
)

const sampleYAML = `id: yaml-plugin
version: 1.0.0
skill:
  slug: lattice-planning
  prompt: "Run"
outputs:
  - artifact: commission-doc
`

func TestRegisterSkillPlugins(t *testing.T) {
	cfg := initTestConfig(t)
	modulesDir := filepath.Join(cfg.LatticeProjectDir, "modules")
	if err := os.WriteFile(filepath.Join(modulesDir, "plugin.yaml"), []byte(sampleYAML), 0644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}
	reg := module.NewRegistry()
	if err := RegisterSkillPlugins(reg, cfg); err != nil {
		t.Fatalf("register plugins: %v", err)
	}
	if _, err := reg.Resolve("yaml-plugin", nil); err != nil {
		t.Fatalf("resolve plugin: %v", err)
	}
}

func initTestConfig(t *testing.T) *config.Config {
	t.Helper()
	root := t.TempDir()
	if err := config.InitLatticeDir(root); err != nil {
		t.Fatalf("init lattice: %v", err)
	}
	return &config.Config{
		ProjectDir:        root,
		LatticeProjectDir: filepath.Join(root, ".lattice"),
	}
}
