package workflow

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultWorkflowDir points to the conventional location for YAML workflow
// definitions when loading from disk.
const DefaultWorkflowDir = "workflows"

// ParseDefinitionYAML decodes a workflow definition from YAML/JSON bytes.
func ParseDefinitionYAML(data []byte) (WorkflowDefinition, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return WorkflowDefinition{}, fmt.Errorf("workflow: definition payload is empty")
	}
	var def WorkflowDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return WorkflowDefinition{}, fmt.Errorf("workflow: decode definition: %w", err)
	}
	return def.Normalized()
}

// LoadDefinitionReader reads workflow definition data from an io.Reader.
func LoadDefinitionReader(r io.Reader) (WorkflowDefinition, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return WorkflowDefinition{}, fmt.Errorf("workflow: read definition: %w", err)
	}
	return ParseDefinitionYAML(content)
}

// LoadDefinitionFile loads a workflow definition from an explicit file path.
func LoadDefinitionFile(path string) (WorkflowDefinition, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return WorkflowDefinition{}, fmt.Errorf("workflow: read %s: %w", path, err)
	}
	def, parseErr := ParseDefinitionYAML(content)
	if parseErr != nil {
		return WorkflowDefinition{}, fmt.Errorf("workflow: %s: %w", path, parseErr)
	}
	return def, nil
}

// LoadDefinitionRelative loads a definition from the workflows directory (or a
// custom baseDir if provided).
func LoadDefinitionRelative(baseDir, name string) (WorkflowDefinition, error) {
	if baseDir == "" {
		baseDir = DefaultWorkflowDir
	}
	path := filepath.Join(baseDir, name)
	return LoadDefinitionFile(path)
}
