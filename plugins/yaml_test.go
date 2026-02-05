package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleDefinition = `id: custom-docs
version: 1.0.0
name: Custom Docs
skill:
  slug: lattice-planning
  prompt: |
    Use the planning skill to produce custom docs.
outputs:
  - artifact: commission-doc
`

func TestParseDefinitionYAML(t *testing.T) {
	def, err := ParseDefinitionYAML([]byte(sampleDefinition))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if def.ID != "custom-docs" || def.Skill.Slug != "lattice-planning" {
		t.Fatalf("unexpected definition: %+v", def)
	}
}

func TestParseDefinitionYAMLErrors(t *testing.T) {
	if _, err := ParseDefinitionYAML([]byte("")); err == nil {
		t.Fatalf("expected empty payload to fail validation")
	}
}

func TestLoadDefinitionDir(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plugin.yaml")
	if err := os.WriteFile(path, []byte(sampleDefinition), 0644); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	defs, err := LoadDefinitionDir(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].Path != path {
		t.Fatalf("expected path %s, got %s", path, defs[0].Path)
	}
	if defs[0].Definition.ID != "custom-docs" {
		t.Fatalf("unexpected id: %+v", defs[0].Definition)
	}
}

func TestLoadDefinitionDirMissing(t *testing.T) {
	defs, err := LoadDefinitionDir(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if defs != nil {
		t.Fatalf("expected nil slice for missing dir, got %v", defs)
	}
}
