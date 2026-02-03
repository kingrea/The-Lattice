package skills

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

// Slug identifies a bundled skill by its canonical folder name.
type Slug string

const (
	CvDistill       Slug = "cv-distill"
	LatticePlanning Slug = "lattice-planning"
	DownCycleAgent  Slug = "down-cycle-agent-summarise"
	DownCycle       Slug = "down-cycle-summarise"
	LocalDreaming   Slug = "local-dreaming"
	FinalSession    Slug = "final-session-prompt"
	CreateAgentFile Slug = "create-agent-file"
)

type descriptor struct {
	source string
	target string
}

var skillFiles = map[Slug]descriptor{
	CvDistill:       {source: "cv-distill/SKILL.md", target: "SKILL.md"},
	LatticePlanning: {source: "lattice-planning/SKILL.md", target: "SKILL.md"},
	DownCycleAgent:  {source: "down-cycle-agent-summarise/SKILL.md", target: "SKILL.md"},
	DownCycle:       {source: "down-cycle-summarise/SKILL.md", target: "SKILL.md"},
	LocalDreaming:   {source: "local-dreaming/SKILL.md", target: "SKILL.md"},
	FinalSession:    {source: "final-session-prompt/SKILL.md", target: "SKILL.md"},
	CreateAgentFile: {source: "create-agent-file/SKILL.md", target: "SKILL.md"},
}

//go:embed library/* library/*/*
var bundled embed.FS

// Ensure writes the requested skill into the provided base directory and returns the on-disk path.
func Ensure(baseDir string, slug Slug) (string, error) {
	if baseDir == "" {
		return "", fmt.Errorf("skills: base directory is empty")
	}
	desc, ok := skillFiles[slug]
	if !ok {
		return "", fmt.Errorf("skill %s is not bundled", slug)
	}
	data, err := bundled.ReadFile(path.Join("library", desc.source))
	if err != nil {
		return "", fmt.Errorf("failed to read embedded skill %s: %w", slug, err)
	}
	targetDir := filepath.Join(baseDir, string(slug))
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to prepare skill directory %s: %w", targetDir, err)
	}
	targetPath := filepath.Join(targetDir, desc.target)
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write skill %s: %w", slug, err)
	}
	return targetPath, nil
}

// EnsureAll installs every bundled skill under the provided base directory.
func EnsureAll(baseDir string) error {
	for slug := range skillFiles {
		if _, err := Ensure(baseDir, slug); err != nil {
			return err
		}
	}
	return nil
}
