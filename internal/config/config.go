// internal/config/config.go
//
// This package handles configuration and the .lattice directory structure.
// Every project that uses Lattice gets a .lattice/ folder created in its root.

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// LatticeDir is the name of the directory we create in each project
	LatticeDir = ".lattice"

	defaultWorkflowID = "commission-work"
)

const defaultProjectConfigYAML = `# lattice project configuration
version: 1

# Communities to load. Use source: github with a repository URL or source: local with a relative path.
communities:
  - name: the-lumen
    source: github
    repository: https://github.com/yourusername/the-lumen
    # Example local override:
    # source: local
    # path: ../communities/the-lumen

# Core agent overrides. Leave as default unless you have custom implementations.
core_agents:
  memory-manager:
    source: default
  orchestration:
    source: default
  community-memory:
    source: default
  emergence:
    source: default

workflows:
  default: commission-work
`

// CommunityRef declares one community source entry inside .lattice/config.yaml.
type CommunityRef struct {
	Name       string `yaml:"name"`
	Source     string `yaml:"source"`
	Repository string `yaml:"repository,omitempty"`
	Path       string `yaml:"path,omitempty"`
}

// CoreAgentOverride defines how a core role should be fulfilled.
type CoreAgentOverride struct {
	Source string `yaml:"source"`
	Path   string `yaml:"path,omitempty"`
}

// WorkflowConfig captures workflow preferences.
type WorkflowConfig struct {
	Default   string   `yaml:"default"`
	Available []string `yaml:"available,omitempty"`
}

// ProjectConfig models .lattice/config.yaml.
type ProjectConfig struct {
	Version     int                          `yaml:"version"`
	Communities []CommunityRef               `yaml:"communities"`
	CoreAgents  map[string]CoreAgentOverride `yaml:"core_agents"`
	Workflows   WorkflowConfig               `yaml:"workflows"`
}

// Config holds the runtime configuration for Lattice.
type Config struct {
	// ProjectDir is the directory where the user ran `lattice` from
	ProjectDir string

	// LatticeRoot is where the Lattice source code lives (G:\The Lattice)
	// This is where MCP servers, agent CVs, skills, etc. are stored
	LatticeRoot string

	// LatticeProjectDir is ProjectDir/.lattice
	LatticeProjectDir string

	Project ProjectConfig
}

// InitLatticeDir creates the .lattice directory structure in the given project directory.
// This is called when the TUI starts up.
//
// Structure created:
// .lattice/
// ├── setup/
// │   └── cvs/      <- Agent CVs will be written here
// ├── logs/         <- For logging orchestration activity
// ├── state/        <- For persisting state between runs
// ├── plan/         <- Anchor documents from lattice-planning skill
// ├── action/       <- Action plan (MODULES.md, PLAN.md)
// └── workflow/     <- Commission workflow state (git-trackable)
//
//	├── team/     <- Hired workers
//	├── work/     <- Work artifacts
//	└── release/  <- Release markers
func InitLatticeDir(projectDir string) error {
	latticeDir := filepath.Join(projectDir, LatticeDir)

	// Create all the subdirectories we need
	// os.MkdirAll creates parent directories as needed (like mkdir -p)
	dirs := []string{
		filepath.Join(latticeDir, "setup", "cvs"),
		filepath.Join(latticeDir, "logs"),
		filepath.Join(latticeDir, "state"),
		filepath.Join(latticeDir, "plan"),
		filepath.Join(latticeDir, "action"),
		filepath.Join(latticeDir, "workflow"),
		filepath.Join(latticeDir, "workflow", "team"),
		filepath.Join(latticeDir, "workflow", "work"),
		filepath.Join(latticeDir, "workflow", "release"),
		filepath.Join(latticeDir, "agents"),
		filepath.Join(latticeDir, "agents", "workers"),
		filepath.Join(latticeDir, "agents", "specialists"),
		filepath.Join(latticeDir, "skills"),
		filepath.Join(latticeDir, "worktree"),
		filepath.Join(latticeDir, "modules"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	if err := ensureProjectConfig(filepath.Join(latticeDir, "config.yaml")); err != nil {
		return err
	}

	return nil
}

// NewConfig creates a new Config instance populated with project settings.
func NewConfig(projectDir string) (*Config, error) {
	// LATTICE_ROOT must be set to the directory containing the lattice CLI source.
	// This is where agents/, skills/, and defaults/ are located.
	latticeRoot := os.Getenv("LATTICE_ROOT")
	if latticeRoot == "" {
		return nil, fmt.Errorf("LATTICE_ROOT environment variable is not set; see README.md for setup instructions")
	}

	cfg := &Config{
		ProjectDir:        projectDir,
		LatticeRoot:       latticeRoot,
		LatticeProjectDir: filepath.Join(projectDir, LatticeDir),
		Project:           defaultProjectConfig(),
	}

	if err := cfg.loadProjectConfig(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// CVsDir returns the path to the CVs directory for the current project
func (c *Config) CVsDir() string {
	return filepath.Join(c.LatticeProjectDir, "setup", "cvs")
}

// LogsDir returns the path to the logs directory
func (c *Config) LogsDir() string {
	return filepath.Join(c.LatticeProjectDir, "logs")
}

// StateDir returns the path to the state directory
func (c *Config) StateDir() string {
	return filepath.Join(c.LatticeProjectDir, "state")
}

// WorkerListPath returns the path to the worker-list.json file
func (c *Config) WorkerListPath() string {
	return filepath.Join(c.WorkflowDir(), "team", "workers.json")
}

// CommunitiesDir returns the path to the communities directory in the Lattice
func (c *Config) CommunitiesDir() string {
	return filepath.Join(c.LatticeRoot, "communities")
}

// WorkflowDir returns the path to the workflow directory
func (c *Config) WorkflowDir() string {
	return filepath.Join(c.LatticeProjectDir, "workflow")
}

// AgentsDir returns the path that holds generated agent files
func (c *Config) AgentsDir() string {
	return filepath.Join(c.LatticeProjectDir, "agents")
}

// WorktreeDir returns the root directory where worktree sessions are materialized
func (c *Config) WorktreeDir() string {
	return filepath.Join(c.LatticeProjectDir, "worktree")
}

// SkillsDir returns the directory where bundled skills are installed per project
func (c *Config) SkillsDir() string {
	return filepath.Join(c.LatticeProjectDir, "skills")
}

// ProjectConfigPath returns the on-disk location for the project config file.
func (c *Config) ProjectConfigPath() string {
	return filepath.Join(c.LatticeProjectDir, "config.yaml")
}

// Communities returns the list of configured community references.
func (c *Config) Communities() []CommunityRef {
	return c.Project.Communities
}

// CoreAgentOverride returns override configuration for a given role.
func (c *Config) CoreAgentOverride(role string) (CoreAgentOverride, bool) {
	ovr, ok := c.Project.CoreAgents[role]
	return ovr, ok
}

// DefaultWorkflow returns the configured default workflow identifier.
func (c *Config) DefaultWorkflow() string {
	return c.Project.Workflows.Default
}

// SetDefaultWorkflow updates the default workflow identifier and persists the
// value back to .lattice/config.yaml. The workflow ID is also appended to the
// available list so the selector can display it on future launches.
func (c *Config) SetDefaultWorkflow(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("config: workflow id is required")
	}
	c.Project.Workflows.Default = id
	if !contains(c.Project.Workflows.Available, id) {
		c.Project.Workflows.Available = append(c.Project.Workflows.Available, id)
	}
	return c.saveProjectConfig()
}

func (c *Config) loadProjectConfig() error {
	path := c.ProjectConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("config: read %s: %w", path, err)
	}

	var parsed ProjectConfig
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("config: parse %s: %w", path, err)
	}

	parsed.applyDefaults()
	parsed.normalize(c.ProjectDir)
	if err := parsed.validate(); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	c.Project = parsed
	return nil
}

func defaultProjectConfig() ProjectConfig {
	return ProjectConfig{
		Version:    1,
		CoreAgents: map[string]CoreAgentOverride{},
		Workflows: WorkflowConfig{
			Default: defaultWorkflowID,
		},
	}
}

func (pc *ProjectConfig) applyDefaults() {
	if pc.Version == 0 {
		pc.Version = 1
	}
	if pc.CoreAgents == nil {
		pc.CoreAgents = map[string]CoreAgentOverride{}
	}
}

func (pc *ProjectConfig) normalize(base string) {
	for i := range pc.Communities {
		pc.Communities[i].normalize(base)
	}
	for role, override := range pc.CoreAgents {
		override.normalize(base)
		pc.CoreAgents[role] = override
	}
	pc.Workflows.Default = strings.TrimSpace(pc.Workflows.Default)
	if pc.Workflows.Default == "" {
		pc.Workflows.Default = defaultWorkflowID
	}
	if len(pc.Workflows.Available) > 0 && !contains(pc.Workflows.Available, pc.Workflows.Default) {
		pc.Workflows.Available = append(pc.Workflows.Available, pc.Workflows.Default)
	}
}

func (pc *ProjectConfig) validate() error {
	if pc.Version < 1 {
		return fmt.Errorf("config version must be >= 1")
	}
	for i := range pc.Communities {
		if err := pc.Communities[i].validate(); err != nil {
			return fmt.Errorf("communities[%d]: %w", i, err)
		}
	}
	for role, override := range pc.CoreAgents {
		if err := override.validate(); err != nil {
			return fmt.Errorf("core_agents[%s]: %w", role, err)
		}
	}
	if strings.TrimSpace(pc.Workflows.Default) == "" {
		return fmt.Errorf("workflows.default is required")
	}
	return nil
}

func (ref *CommunityRef) normalize(base string) {
	ref.Name = strings.TrimSpace(ref.Name)
	ref.Source = normalizeSource(ref.Source)
	ref.Repository = strings.TrimSpace(ref.Repository)
	ref.Path = resolvePath(base, ref.Path)
}

func (ref CommunityRef) validate() error {
	if ref.Name == "" {
		return fmt.Errorf("name is required")
	}
	switch ref.Source {
	case "github":
		if ref.Repository == "" {
			return fmt.Errorf("repository is required for github communities")
		}
	case "local":
		if ref.Path == "" {
			return fmt.Errorf("path is required for local communities")
		}
	default:
		return fmt.Errorf("source must be 'github' or 'local'")
	}
	return nil
}

func (ovr *CoreAgentOverride) normalize(base string) {
	ovr.Source = normalizeSource(ovr.Source)
	ovr.Path = resolvePath(base, ovr.Path)
}

func (ovr CoreAgentOverride) validate() error {
	switch ovr.Source {
	case "", "default":
		return nil
	case "custom":
		if ovr.Path == "" {
			return fmt.Errorf("path is required for custom core agents")
		}
		return nil
	default:
		return fmt.Errorf("source must be 'default' or 'custom'")
	}
}

func normalizeSource(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), target) {
			return true
		}
	}
	return false
}

func resolvePath(base, candidate string) string {
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" {
		return ""
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}
	return filepath.Clean(filepath.Join(base, trimmed))
}

func ensureProjectConfig(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, []byte(defaultProjectConfigYAML), 0644)
}

func (c *Config) saveProjectConfig() error {
	if c == nil {
		return fmt.Errorf("config: nil receiver")
	}
	c.Project.applyDefaults()
	c.Project.normalize(c.ProjectDir)
	if err := c.Project.validate(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := os.MkdirAll(c.LatticeProjectDir, 0o755); err != nil {
		return fmt.Errorf("config: ensure lattice dir: %w", err)
	}
	data, err := yaml.Marshal(c.Project)
	if err != nil {
		return fmt.Errorf("config: encode config: %w", err)
	}
	if err := os.WriteFile(c.ProjectConfigPath(), data, 0644); err != nil {
		return fmt.Errorf("config: write project config: %w", err)
	}
	return nil
}
