package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadProjectConfigDefaultsWhenMissing(t *testing.T) {
	projectDir := t.TempDir()
	latticeDir := filepath.Join(projectDir, ".lattice")
	if err := os.MkdirAll(latticeDir, 0755); err != nil {
		t.Fatal(err)
	}
	c := &Config{ProjectDir: projectDir, LatticeProjectDir: latticeDir, Project: defaultProjectConfig()}
	if err := c.loadProjectConfig(); err != nil {
		t.Fatalf("loadProjectConfig returned error: %v", err)
	}
	if c.Project.Version != 1 {
		t.Fatalf("expected default version == 1, got %d", c.Project.Version)
	}
	if c.DefaultWorkflow() != defaultWorkflowID {
		t.Fatalf("expected default workflow %q, got %q", defaultWorkflowID, c.DefaultWorkflow())
	}
}

func TestLoadProjectConfigParsesYaml(t *testing.T) {
	projectDir := t.TempDir()
	latticeDir := filepath.Join(projectDir, ".lattice")
	if err := os.MkdirAll(latticeDir, 0755); err != nil {
		t.Fatal(err)
	}
	configYAML := strings.TrimSpace(`
version: 1
communities:
  - name: the-lumen
    source: local
    path: communities/the-lumen
  - name: remote-lumen
    source: github
    repository: https://github.com/example/the-lumen
core_agents:
  memory-manager:
    source: custom
    path: agents/anam
workflows:
  default: commission-work
  available:
    - commission-work
    - audit-practice
session:
  idle_watchdog:
    enabled: false
    timeout: 10m
`)
	if err := os.WriteFile(filepath.Join(latticeDir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}
	c := &Config{ProjectDir: projectDir, LatticeProjectDir: latticeDir, Project: defaultProjectConfig()}
	if err := c.loadProjectConfig(); err != nil {
		t.Fatalf("loadProjectConfig returned error: %v", err)
	}
	if len(c.Communities()) != 2 {
		t.Fatalf("expected 2 communities, got %d", len(c.Communities()))
	}
	local := c.Communities()[0]
	if !strings.HasPrefix(local.Path, projectDir) {
		t.Fatalf("expected local path to be resolved, got %s", local.Path)
	}
	if _, ok := c.CoreAgentOverride("memory-manager"); !ok {
		t.Fatalf("expected custom memory-manager override")
	}
	mm, _ := c.CoreAgentOverride("memory-manager")
	if !strings.HasPrefix(mm.Path, projectDir) {
		t.Fatalf("expected memory-manager path to be absolute, got %s", mm.Path)
	}
	if c.DefaultWorkflow() != "commission-work" {
		t.Fatalf("wrong default workflow: %s", c.DefaultWorkflow())
	}
	settings := c.IdleWatchdogSettings()
	if settings.Enabled {
		t.Fatalf("expected idle watchdog to be disabled")
	}
	if settings.Timeout != 10*time.Minute {
		t.Fatalf("expected idle timeout 10m, got %s", settings.Timeout)
	}
}

func TestLoadProjectConfigValidation(t *testing.T) {
	projectDir := t.TempDir()
	latticeDir := filepath.Join(projectDir, ".lattice")
	if err := os.MkdirAll(latticeDir, 0755); err != nil {
		t.Fatal(err)
	}
	configYAML := strings.TrimSpace(`
version: 1
communities:
  - name: remote-lumen
    source: github
`)
	if err := os.WriteFile(filepath.Join(latticeDir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}
	c := &Config{ProjectDir: projectDir, LatticeProjectDir: latticeDir, Project: defaultProjectConfig()}
	if err := c.loadProjectConfig(); err == nil {
		t.Fatalf("expected validation error but got none")
	}
}

func TestInitLatticeDirCreatesProjectConfigTemplate(t *testing.T) {
	projectDir := t.TempDir()
	if err := InitLatticeDir(projectDir); err != nil {
		t.Fatalf("InitLatticeDir failed: %v", err)
	}
	configPath := filepath.Join(projectDir, ".lattice", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected config.yaml to exist: %v", err)
	}
	contents := string(data)
	if !strings.Contains(contents, "version: 1") {
		t.Fatalf("expected default config template, got %s", contents)
	}
}

func TestSetDefaultWorkflowPersistsSelection(t *testing.T) {
	projectDir := t.TempDir()
	latticeDir := filepath.Join(projectDir, ".lattice")
	if err := os.MkdirAll(latticeDir, 0o755); err != nil {
		t.Fatalf("mkdir lattice dir: %v", err)
	}
	c := &Config{ProjectDir: projectDir, LatticeProjectDir: latticeDir, Project: defaultProjectConfig()}
	if err := c.SetDefaultWorkflow("quick-start"); err != nil {
		t.Fatalf("SetDefaultWorkflow: %v", err)
	}
	if got := c.DefaultWorkflow(); got != "quick-start" {
		t.Fatalf("expected default workflow to persist, got %s", got)
	}
	if !contains(c.Project.Workflows.Available, "quick-start") {
		t.Fatalf("expected available workflows to include quick-start")
	}
	configPath := filepath.Join(latticeDir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading persisted config: %v", err)
	}
	if !strings.Contains(string(data), "quick-start") {
		t.Fatalf("expected persisted config to mention quick-start, got %s", data)
	}
}

func TestSetDefaultWorkflowRequiresID(t *testing.T) {
	projectDir := t.TempDir()
	latticeDir := filepath.Join(projectDir, ".lattice")
	if err := os.MkdirAll(latticeDir, 0o755); err != nil {
		t.Fatalf("mkdir lattice dir: %v", err)
	}
	c := &Config{ProjectDir: projectDir, LatticeProjectDir: latticeDir, Project: defaultProjectConfig()}
	if err := c.SetDefaultWorkflow(" "); err == nil {
		t.Fatalf("expected error when workflow id is blank")
	}
}

func TestIdleWatchdogSettingsDefaults(t *testing.T) {
	c := &Config{}
	settings := c.IdleWatchdogSettings()
	if !settings.Enabled {
		t.Fatalf("expected idle watchdog default enabled")
	}
	if settings.Timeout != 5*time.Minute {
		t.Fatalf("expected default timeout 5m, got %s", settings.Timeout)
	}
}

func TestNewConfigUsesEmbeddedDefaultRoot(t *testing.T) {
	projectDir := t.TempDir()
	prevEnv := os.Getenv("LATTICE_ROOT")
	prevDefault := defaultLatticeRoot
	t.Cleanup(func() {
		defaultLatticeRoot = prevDefault
		if prevEnv == "" {
			_ = os.Unsetenv("LATTICE_ROOT")
			return
		}
		if err := os.Setenv("LATTICE_ROOT", prevEnv); err != nil {
			t.Fatalf("restore env: %v", err)
		}
	})
	_ = os.Unsetenv("LATTICE_ROOT")
	defaultLatticeRoot = filepath.Join(t.TempDir(), "lattice-root")
	cfg, err := NewConfig(projectDir)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	if cfg.LatticeRoot != defaultLatticeRoot {
		t.Fatalf("expected lattice root %s, got %s", defaultLatticeRoot, cfg.LatticeRoot)
	}
}
