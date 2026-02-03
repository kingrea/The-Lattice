package contracts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAgentFile(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantValid bool
	}{
		{
			name: "valid-memory-manager",
			yaml: `lattice:
  type: core-agent
  role: memory-manager
  version: 1
name: Anam
description: Memory distiller
community: the-lumen
model: inherit
skills:
  - id: memory-distill
    path: skills/memory-distill/SKILL.md
  - id: memory-summary
    path: skills/memory-summary/SKILL.md
`,
			wantValid: true,
		},
		{
			name: "missing-required-skill",
			yaml: `lattice:
  type: core-agent
  role: community-memory
  version: 1
name: Koinos
description: Community memory
community: the-lumen
model: inherit
skills:
  - id: community-read
    path: skills/community-read/SKILL.md
`,
			wantValid: false,
		},
		{
			name: "unknown-role",
			yaml: `lattice:
  type: core-agent
  role: unknown
  version: 1
name: Echo
description: Unknown
community: the-lumen
model: inherit
skills:
  - id: something
    path: skills/something/SKILL.md
`,
			wantValid: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "agent.yaml")
			if err := os.WriteFile(path, []byte(test.yaml), 0644); err != nil {
				t.Fatalf("write temp agent: %v", err)
			}
			report, err := ValidateAgentFile(path)
			if err != nil {
				t.Fatalf("validate agent file: %v", err)
			}
			if report.IsValid() != test.wantValid {
				t.Fatalf("valid=%v want=%v errors=%v", report.IsValid(), test.wantValid, report.Errors)
			}
		})
	}
}
