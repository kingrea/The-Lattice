package plugins

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefinitionFile pairs a parsed module definition with its on-disk source.
type DefinitionFile struct {
	Definition ModuleDefinition
	Path       string
}

// ParseDefinitionYAML decodes and validates a single plugin definition payload.
func ParseDefinitionYAML(data []byte) (ModuleDefinition, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return ModuleDefinition{}, fmt.Errorf("plugin: definition payload is empty")
	}
	var def ModuleDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return ModuleDefinition{}, fmt.Errorf("plugin: decode definition: %w", err)
	}
	if err := def.Validate(); err != nil {
		return ModuleDefinition{}, err
	}
	return def.Normalized(), nil
}

// LoadDefinitionFile reads a YAML file from disk and returns the parsed module definition.
func LoadDefinitionFile(path string) (DefinitionFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return DefinitionFile{}, fmt.Errorf("plugin: stat %s: %w", path, err)
	}
	if info.IsDir() {
		return DefinitionFile{}, fmt.Errorf("plugin: %s is a directory", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return DefinitionFile{}, fmt.Errorf("plugin: read %s: %w", path, err)
	}
	def, err := ParseDefinitionYAML(data)
	if err != nil {
		return DefinitionFile{}, fmt.Errorf("plugin: %s: %w", path, err)
	}
	return DefinitionFile{Definition: def, Path: filepath.Clean(path)}, nil
}

// LoadDefinitionDir scans a directory for *.yaml modules and returns the parsed definitions.
// Missing directories are treated as "no plugins" to simplify startup.
func LoadDefinitionDir(dir string) ([]DefinitionFile, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(trimmed)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("plugin: read %s: %w", trimmed, err)
	}
	var defs []DefinitionFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isYAMLFile(name) {
			continue
		}
		path := filepath.Join(trimmed, name)
		def, err := LoadDefinitionFile(path)
		if err != nil {
			return nil, err
		}
		defs = append(defs, def)
	}
	if len(defs) == 0 {
		return nil, nil
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Path < defs[j].Path })
	return defs, nil
}

func isYAMLFile(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}
