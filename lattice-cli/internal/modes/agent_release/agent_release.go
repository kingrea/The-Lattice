// internal/modes/agent_release/agent_release.go
//
// Agent Release mode handles releasing worker agents after work completion.
// Input: .lattice/workflow/work/.complete
// Output: .lattice/workflow/release/.agents-released

package agent_release

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yourusername/lattice/internal/modes"
	"github.com/yourusername/lattice/internal/workflow"
)

var cleanupWindowPrefixes = []string{
	"worktree-",
	"summary-",
	"dream-",
	"down-cycle-",
	"land-",
	"agent-file-",
	"reviewer-",
	"staff-engineer-",
	"lattice-planning-",
	"lattice-action-",
	"consolidation-",
	"bead-creation-",
	"worktree-agent-",
	"worktree-orchestrator-",
}

// Mode handles the agent release phase
type Mode struct {
	modes.BaseMode
	width  int
	height int
}

// New creates a new Agent Release mode
func New() *Mode {
	return &Mode{
		BaseMode: modes.NewBaseMode("Agent Release", workflow.PhaseAgentRelease),
	}
}

// Init initializes the agent release mode
func (m *Mode) Init(ctx *modes.ModeContext) tea.Cmd {
	m.SetContext(ctx)

	// Check if agents already released
	wf := ctx.Workflow
	releasedMarker := filepath.Join(wf.ReleaseDir(), workflow.MarkerAgentsReleased)
	if fileExists(releasedMarker) {
		m.SetComplete(true)
		m.SetStatusMsg("Agents already released, skipping to next phase")
		return func() tea.Msg {
			return modes.ModeCompleteMsg{NextPhase: workflow.PhaseWorkCleanup}
		}
	}

	m.SetStatusMsg("Releasing worker agents...")
	return m.releaseAgents()
}

// Update handles messages for the agent release mode
func (m *Mode) Update(msg tea.Msg) (modes.Mode, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			return m, func() tea.Msg {
				return modes.ModeErrorMsg{Error: fmt.Errorf("agent release cancelled")}
			}
		}

	case agentsReleasedMsg:
		m.SetComplete(true)
		m.SetStatusMsg("Workers released!")
		return m, func() tea.Msg {
			return modes.ModeCompleteMsg{NextPhase: workflow.PhaseWorkCleanup}
		}

	case modes.ModeErrorMsg:
		m.SetStatusMsg(fmt.Sprintf("Error: %v", msg.Error))
		return m, nil
	}

	return m, nil
}

// View renders the agent release mode
func (m *Mode) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#9B59B6")).
		Padding(1)

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		MarginTop(1)

	content := titleStyle.Render(`
  🔓 RELEASING WORKERS

  The release process:
  1. Thank worker agents for their service
  2. Archive their work logs
  3. Remove them from active duty

  Waiting for release to complete...

  Press ESC to cancel.
`)

	return fmt.Sprintf("%s\n%s", content, statusStyle.Render(m.StatusMsg()))
}

// Message types
type agentsReleasedMsg struct{}

// releaseAgents performs the agent release
func (m *Mode) releaseAgents() tea.Cmd {
	return func() tea.Msg {
		ctx := m.Context()
		if ctx == nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("missing mode context")}
		}

		releaseDir := ctx.Workflow.ReleaseDir()
		if err := os.MkdirAll(releaseDir, 0755); err != nil {
			return modes.ModeErrorMsg{Error: err}
		}

		if err := killWorkflowTmuxWindows(); err != nil {
			return modes.ModeErrorMsg{Error: err}
		}

		if err := archiveAndResetWorkerList(ctx, releaseDir); err != nil {
			return modes.ModeErrorMsg{Error: err}
		}

		if err := clearWorkState(ctx); err != nil {
			return modes.ModeErrorMsg{Error: err}
		}

		marker := filepath.Join(releaseDir, workflow.MarkerAgentsReleased)
		if err := os.WriteFile(marker, []byte{}, 0644); err != nil {
			return modes.ModeErrorMsg{Error: err}
		}

		return agentsReleasedMsg{}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func archiveAndResetWorkerList(ctx *modes.ModeContext, releaseDir string) error {
	workerListPath := ctx.Config.WorkerListPath()
	data, err := os.ReadFile(workerListPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read worker list: %w", err)
	}
	timestamp := time.Now().UTC().Format("20060102-150405")
	dest := filepath.Join(releaseDir, fmt.Sprintf("workers-%s.json", timestamp))
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return fmt.Errorf("archive worker list: %w", err)
	}
	if err := os.WriteFile(workerListPath, []byte("[]\n"), 0644); err != nil {
		return fmt.Errorf("reset worker list: %w", err)
	}
	return nil
}

func clearWorkState(ctx *modes.ModeContext) error {
	workDir := ctx.Workflow.WorkDir()
	paths := []string{
		filepath.Join(workDir, workflow.MarkerWorkInProgress),
		filepath.Join(workDir, "current-cycle.json"),
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", filepath.Base(path), err)
		}
	}
	return nil
}

func killWorkflowTmuxWindows() error {
	cmd := exec.Command("tmux", "list-windows", "-F", "#W")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("list tmux windows: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	toKill := map[string]struct{}{}
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		for _, prefix := range cleanupWindowPrefixes {
			if strings.HasPrefix(name, prefix) {
				toKill[name] = struct{}{}
				break
			}
		}
	}
	for name := range toKill {
		if err := exec.Command("tmux", "kill-window", "-t", name).Run(); err != nil {
			return fmt.Errorf("kill tmux window %s: %w", name, err)
		}
	}
	return nil
}
