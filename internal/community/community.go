package community

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const configFilename = "community.yaml"

// LatticeMeta captures the required frontmatter for lattice-managed files.
type LatticeMeta struct {
	Type    string `yaml:"type"`
	Version int    `yaml:"version"`
}

// Paths defines where key community assets live relative to the community root.
type Paths struct {
	Identities string `yaml:"identities"`
	Denizens   string `yaml:"denizens"`
	CVs        string `yaml:"cvs"`
	Gods       string `yaml:"gods,omitempty"`
	Sparks     string `yaml:"sparks,omitempty"`
	Mythology  string `yaml:"mythology,omitempty"`
	Rituals    string `yaml:"rituals,omitempty"`
	Memory     string `yaml:"memory,omitempty"`
}

// Config models the on-disk community.yaml schema.
type Config struct {
	Lattice     LatticeMeta `yaml:"lattice"`
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Repository  string      `yaml:"repository,omitempty"`
	Paths       Paths       `yaml:"paths"`
}

// Community represents a loaded community repository.
type Community struct {
	Root   string
	Config Config
}

// Load reads and validates a community.yaml file from the provided directory.
func Load(root string) (*Community, error) {
	configPath := filepath.Join(root, configFilename)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("community: read %s: %w", configPath, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("community: parse %s: %w", configPath, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("community: %w", err)
	}
	cfg.normalize()

	return &Community{Root: root, Config: cfg}, nil
}

// ResolvePath converts a relative community path into an absolute path.
func (c *Community) ResolvePath(rel string) string {
	trimmed := strings.TrimSpace(rel)
	if trimmed == "" {
		return c.Root
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}
	return filepath.Join(c.Root, filepath.FromSlash(trimmed))
}

// CVsPath returns the absolute path to the directory containing denizen CVs.
func (c *Community) CVsPath() string {
	return c.ResolvePath(c.Config.Paths.CVs)
}

// DenizensPath returns the absolute path to denizen identity folders.
func (c *Community) DenizensPath() string {
	return c.ResolvePath(c.Config.Paths.Denizens)
}

func (cfg *Config) validate() error {
	if strings.ToLower(strings.TrimSpace(cfg.Lattice.Type)) != "community" {
		return fmt.Errorf("lattice.type must be 'community'")
	}
	if cfg.Lattice.Version < 1 {
		return fmt.Errorf("lattice.version must be >= 1")
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(cfg.Paths.Identities) == "" {
		return fmt.Errorf("paths.identities is required")
	}
	if strings.TrimSpace(cfg.Paths.Denizens) == "" {
		return fmt.Errorf("paths.denizens is required")
	}
	if strings.TrimSpace(cfg.Paths.CVs) == "" {
		return fmt.Errorf("paths.cvs is required")
	}
	return nil
}

func (cfg *Config) normalize() {
	cfg.Paths.Identities = cleanPath(cfg.Paths.Identities)
	cfg.Paths.Denizens = cleanPath(cfg.Paths.Denizens)
	cfg.Paths.CVs = cleanPath(cfg.Paths.CVs)
	cfg.Paths.Gods = cleanPath(cfg.Paths.Gods)
	cfg.Paths.Sparks = cleanPath(cfg.Paths.Sparks)
	cfg.Paths.Mythology = cleanPath(cfg.Paths.Mythology)
	cfg.Paths.Rituals = cleanPath(cfg.Paths.Rituals)
	cfg.Paths.Memory = cleanPath(cfg.Paths.Memory)
}

func cleanPath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(filepath.FromSlash(trimmed))
}
