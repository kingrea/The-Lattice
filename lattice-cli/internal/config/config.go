// internal/config/config.go
//
// This package handles configuration and the .lattice directory structure.
// Every project that uses Lattice gets a .lattice/ folder created in its root.

package config

import (
	"os"
	"path/filepath"
)

// LatticeDir is the name of the directory we create in each project
const LatticeDir = ".lattice"

// Config holds the runtime configuration for Lattice
type Config struct {
	// ProjectDir is the directory where the user ran `lattice` from
	ProjectDir string

	// LatticeRoot is where the Lattice source code lives (G:\The Lattice)
	// This is where MCP servers, agent CVs, skills, etc. are stored
	LatticeRoot string

	// LatticeProjectDir is ProjectDir/.lattice
	LatticeProjectDir string
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
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

// NewConfig creates a new Config instance
func NewConfig(projectDir string) *Config {
	// TODO: Make this configurable via env var or config file
	// For now, hardcode your path. On Windows/WSL this would be something like:
	// /mnt/g/The Lattice
	latticeRoot := os.Getenv("LATTICE_ROOT")
	if latticeRoot == "" {
		latticeRoot = "/mnt/g/The Lattice" // Default for your setup
	}

	return &Config{
		ProjectDir:        projectDir,
		LatticeRoot:       latticeRoot,
		LatticeProjectDir: filepath.Join(projectDir, LatticeDir),
	}
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
