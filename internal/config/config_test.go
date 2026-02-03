package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
