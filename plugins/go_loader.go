package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"gopkg.in/yaml.v3"
)

const goDefinitionFuncName = "ModuleDefinitions"

// LoadGoDefinitionDir evaluates every .go file in dir and collects module definitions declared via ModuleDefinitions().
func LoadGoDefinitionDir(dir string) ([]DefinitionFile, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(trimmed)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("plugin: read %s: %w", trimmed, err)
	}
	var defs []DefinitionFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		fileDefs, err := loadGoDefinitionFile(filepath.Join(trimmed, entry.Name()))
		if err != nil {
			return nil, err
		}
		defs = append(defs, fileDefs...)
	}
	if len(defs) == 0 {
		return nil, nil
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Path < defs[j].Path })
	return defs, nil
}

func loadGoDefinitionFile(path string) ([]DefinitionFile, error) {
	code, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("plugin: read %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(code))) == 0 {
		return nil, fmt.Errorf("plugin: %s is empty", path)
	}
	i := interp.New(interp.Options{})
	i.Use(stdlib.Symbols)
	if _, err := i.EvalPath(path); err != nil {
		return nil, fmt.Errorf("plugin: interpret %s: %w", path, err)
	}
	fnValue, err := i.Eval(goDefinitionFuncName)
	if err != nil {
		return nil, fmt.Errorf("plugin: %s must define %s() ([]map[string]any, error): %w", path, goDefinitionFuncName, err)
	}
	defs, callErr := invokeDefinitionFunc(fnValue)
	if callErr != nil {
		return nil, fmt.Errorf("plugin: %s: %w", path, callErr)
	}
	files := make([]DefinitionFile, 0, len(defs))
	for idx, raw := range defs {
		payload, err := yaml.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("plugin: %s definition[%d]: %w", path, idx, err)
		}
		parsed, err := ParseDefinitionYAML(payload)
		if err != nil {
			return nil, fmt.Errorf("plugin: %s definition[%d]: %w", path, idx, err)
		}
		files = append(files, DefinitionFile{Definition: parsed, Path: fmt.Sprintf("%s#%d", path, idx+1)})
	}
	return files, nil
}

func invokeDefinitionFunc(value reflect.Value) ([]map[string]any, error) {
	if !value.IsValid() {
		return nil, fmt.Errorf("missing %s function", goDefinitionFuncName)
	}
	fn := value
	if fn.Kind() != reflect.Func {
		return nil, fmt.Errorf("%s is not a function", goDefinitionFuncName)
	}
	results := fn.Call(nil)
	if len(results) == 0 || len(results) > 2 {
		return nil, fmt.Errorf("%s must return ([]map[string]any[, error])", goDefinitionFuncName)
	}
	defsVal := results[0]
	if len(results) == 2 {
		if !results[1].IsNil() {
			if e, ok := results[1].Interface().(error); ok && e != nil {
				return nil, e
			}
			return nil, fmt.Errorf("%s returned non-error second value", goDefinitionFuncName)
		}
	}
	defs, ok := defsVal.Interface().([]map[string]any)
	if ok {
		return defs, nil
	}
	if defsVal.Kind() == reflect.Slice {
		result := make([]map[string]any, defsVal.Len())
		for i := 0; i < defsVal.Len(); i++ {
			entry := defsVal.Index(i).Interface()
			m, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s[%d] is not map[string]any", goDefinitionFuncName, i)
			}
			result[i] = m
		}
		return result, nil
	}
	return nil, fmt.Errorf("%s must return []map[string]any", goDefinitionFuncName)
}
