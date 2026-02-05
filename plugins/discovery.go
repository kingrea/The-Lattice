package plugins

import (
	"fmt"
	"path/filepath"

	"github.com/kingrea/The-Lattice/internal/config"
	"github.com/kingrea/The-Lattice/internal/module"
)

// RegisterSkillPlugins discovers YAML and Go module definitions under .lattice/modules and registers them.
func RegisterSkillPlugins(reg *module.Registry, cfg *config.Config) error {
	if reg == nil || cfg == nil {
		return nil
	}
	dir := filepath.Join(cfg.LatticeProjectDir, "modules")
	defs, err := loadAllDefinitionFiles(dir)
	if err != nil {
		return err
	}
	if len(defs) == 0 {
		return nil
	}
	seen := make(map[string]string)
	for _, file := range defs {
		def := file.Definition
		if existing, ok := seen[def.ID]; ok {
			return fmt.Errorf("plugin: duplicate module id %s (%s and %s)", def.ID, existing, file.Path)
		}
		seen[def.ID] = file.Path
		defCopy := def
		if err := reg.Register(defCopy.ID, func(cfg module.Config) (module.Module, error) {
			return newSkillModule(defCopy, cfg)
		}); err != nil {
			return fmt.Errorf("plugin: register %s from %s: %w", def.ID, file.Path, err)
		}
	}
	return nil
}

func loadAllDefinitionFiles(dir string) ([]DefinitionFile, error) {
	yamlDefs, err := LoadDefinitionDir(dir)
	if err != nil {
		return nil, err
	}
	goDefs, err := LoadGoDefinitionDir(dir)
	if err != nil {
		return nil, err
	}
	return append(yamlDefs, goDefs...), nil
}
