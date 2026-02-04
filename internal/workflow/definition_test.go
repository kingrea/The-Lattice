package workflow

import (
	"strings"
	"testing"
)

func TestParseDefinitionYAMLRejectsMissingModules(t *testing.T) {
	const payload = `
id: missing-modules
modules: []
`
	_, err := ParseDefinitionYAML([]byte(payload))
	if err == nil {
		t.Fatalf("expected error when modules are missing")
	}
	if !strings.Contains(err.Error(), "at least one module is required") {
		t.Fatalf("unexpected error for missing modules: %v", err)
	}
}

func TestParseDefinitionYAMLRejectsInvalidDependencyReferences(t *testing.T) {
	const payload = `
id: invalid-dependency
modules:
  - id: start
    module: anchor-docs
    depends_on: [missing]
`
	_, err := ParseDefinitionYAML([]byte(payload))
	if err == nil {
		t.Fatalf("expected error when dependency references unknown module")
	}
	if !strings.Contains(err.Error(), "references unknown module") {
		t.Fatalf("unexpected error for dependency reference: %v", err)
	}
}

func TestParseDefinitionYAMLClampsNegativeParallelSettings(t *testing.T) {
	const payload = `
id: clamp-runtime
runtime:
  max_parallel: -4
modules:
  - module: anchor-docs
`
	def, err := ParseDefinitionYAML([]byte(payload))
	if err != nil {
		t.Fatalf("unexpected error parsing runtime clamp: %v", err)
	}
	if def.Runtime.MaxParallel != 0 {
		t.Fatalf("max_parallel should clamp to 0, got %d", def.Runtime.MaxParallel)
	}
}
