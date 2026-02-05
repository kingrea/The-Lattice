package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

const goPluginSource = `package main

func ModuleDefinitions() ([]map[string]any, error) {
	return []map[string]any{
		{
			"id":      "go-plugin",
			"version": "1.0.0",
			"skill": map[string]any{
				"slug":   "lattice-planning",
				"prompt": "Use planning skill",
			},
			"outputs": []map[string]any{
				{"artifact": "commission-doc"},
			},
		},
	}, nil
}`

func TestLoadGoDefinitionDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go-plugin.go"), []byte(goPluginSource), 0644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}
	defs, err := LoadGoDefinitionDir(dir)
	if err != nil {
		t.Fatalf("load go defs: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].Definition.ID != "go-plugin" {
		t.Fatalf("unexpected id: %+v", defs[0].Definition)
	}
}

func TestLoadGoDefinitionDirMissingFunc(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatalf("write broken plugin: %v", err)
	}
	if _, err := LoadGoDefinitionDir(dir); err == nil {
		t.Fatalf("expected error for missing ModuleDefinitions function")
	}
}
