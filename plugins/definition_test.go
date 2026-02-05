package plugins

import (
	"strings"
	"testing"
)

func TestModuleDefinitionValidate(t *testing.T) {
	def := ModuleDefinition{
		ID:      "custom-docs",
		Name:    "Custom Docs",
		Version: "1.0.0",
		Skill: SkillDefinition{
			Slug:   "lattice-planning",
			Prompt: "Load the planning skill and write docs",
		},
		Outputs: []ArtifactBinding{{Artifact: "commission-doc"}},
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("expected definition to validate, got %v", err)
	}
}

func TestModuleDefinitionValidateFailures(t *testing.T) {
	tests := []struct {
		name string
		def  ModuleDefinition
		msg  string
	}{
		{
			name: "missing id",
			def: ModuleDefinition{
				Version: "1.0.0",
				Skill:   SkillDefinition{Slug: "lattice-planning", Prompt: "run"},
				Outputs: []ArtifactBinding{{Artifact: "commission-doc"}},
			},
			msg: "id is required",
		},
		{
			name: "unknown artifact",
			def: ModuleDefinition{
				ID:      "custom-docs",
				Version: "1.0.0",
				Skill:   SkillDefinition{Slug: "lattice-planning", Prompt: "run"},
				Outputs: []ArtifactBinding{{Artifact: "does-not-exist"}},
			},
			msg: "does-not-exist",
		},
		{
			name: "missing skill reference",
			def: ModuleDefinition{
				ID:      "custom-docs",
				Version: "1.0.0",
				Skill:   SkillDefinition{Prompt: "run"},
				Outputs: []ArtifactBinding{{Artifact: "commission-doc"}},
			},
			msg: "slug or path",
		},
		{
			name: "duplicate outputs",
			def: ModuleDefinition{
				ID:      "custom-docs",
				Version: "1.0.0",
				Skill:   SkillDefinition{Slug: "lattice-planning", Prompt: "run"},
				Outputs: []ArtifactBinding{{Artifact: "commission-doc"}, {Artifact: "commission-doc"}},
			},
			msg: "duplicate",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.def.Validate(); err == nil || !strings.Contains(err.Error(), tc.msg) {
				t.Fatalf("expected error containing %q, got %v", tc.msg, err)
			}
		})
	}
}

func TestArtifactBindingResolve(t *testing.T) {
	binding := ArtifactBinding{Artifact: "commission-doc", Optional: true}
	ref, err := binding.Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !ref.Optional {
		t.Fatalf("expected optional override, got %+v", ref)
	}
}
