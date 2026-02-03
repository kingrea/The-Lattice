// internal/modes/work_process/work_process.go
//
// Work Process mode handles the actual execution of the commissioned work.
// Input: .lattice/workflow/team/workers.json, .lattice/workflow/plan.md
// Output: .lattice/workflow/work/.complete marker

package work_process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yourusername/lattice/internal/modes"
	"github.com/yourusername/lattice/internal/orchestrator"
	"github.com/yourusername/lattice/internal/workflow"
)

// Mode handles the work process phase
type Mode struct {
	modes.BaseMode
	width    int
	height   int
	sessions []orchestrator.WorktreeSession
}

// New creates a new Work Process mode
func New() *Mode {
	return &Mode{
		BaseMode: modes.NewBaseMode("Work Process", workflow.PhaseWorkProcess),
	}
}

// Init initializes the work process mode
func (m *Mode) Init(ctx *modes.ModeContext) tea.Cmd {
	m.SetContext(ctx)

	// Check if work already complete
	wf := ctx.Workflow
	completeMarker := filepath.Join(wf.WorkDir(), workflow.MarkerWorkComplete)
	if fileExists(completeMarker) {
		m.SetComplete(true)
		m.SetStatusMsg("Work already complete, skipping to next phase")
		return func() tea.Msg {
			return modes.ModeCompleteMsg{NextPhase: workflow.PhaseAgentRelease}
		}
	}

	m.SetStatusMsg("Starting work process...")
	return m.startWork()
}

// Update handles messages for the work process mode
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
				return modes.ModeErrorMsg{Error: fmt.Errorf("work process cancelled")}
			}
		}

	case workPreparedMsg:
		m.sessions = msg.sessions
		summary := fmt.Sprintf("Prepared %d worktree session(s)", len(msg.sessions))
		m.SetStatusMsg(summary + " — launching agents...")
		return m, m.runUpCycle()

	case upCycleCompleteMsg:
		if msg.err != nil {
			return m, func() tea.Msg {
				return modes.ModeErrorMsg{Error: msg.err}
			}
		}
		m.SetComplete(true)
		m.SetStatusMsg("Agent sessions complete. Moving to release.")
		return m, func() tea.Msg {
			return modes.ModeCompleteMsg{NextPhase: workflow.PhaseAgentRelease}
		}

	case modes.ModeErrorMsg:
		m.SetStatusMsg(fmt.Sprintf("Error: %v", msg.Error))
		return m, nil
	}

	return m, nil
}

// View renders the work process mode
func (m *Mode) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FF6B6B")).
		Padding(1)

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		MarginTop(1)

	sessionLines := []string{"  Waiting for sessions..."}
	if len(m.sessions) > 0 {
		sessionLines = []string{"  Prepared sessions:"}
		for _, session := range m.sessions {
			line := fmt.Sprintf("  - %s → %s (%d pt, %d bead)",
				session.Agent.Name,
				session.Name,
				session.TotalPoints(),
				len(session.Beads),
			)
			sessionLines = append(sessionLines, line)
		}
	}
	content := titleStyle.Render(strings.Join([]string{
		"  ⚙️  WORK IN PROGRESS",
		"",
		"  The work process:",
		"  1. Orchestrator delegates tasks to workers",
		"  2. Workers execute their assigned tasks",
		"  3. Progress is logged to work-log.md",
		"",
		strings.Join(sessionLines, "\n"),
		"",
		"  Press ESC to cancel.",
	}, "\n"))

	return fmt.Sprintf("%s\n%s", content, statusStyle.Render(m.StatusMsg()))
}

// Message types
type workPreparedMsg struct {
	sessions []orchestrator.WorktreeSession
}

type upCycleCompleteMsg struct {
	err error
}

// startWork begins the work process
func (m *Mode) startWork() tea.Cmd {
	return func() tea.Msg {
		ctx := m.Context()
		if ctx == nil || ctx.Orchestrator == nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("orchestrator not available")}
		}
		sessions, err := ctx.Orchestrator.PrepareWorkCycle()
		if err != nil {
			if errors.Is(err, orchestrator.ErrNoReadyBeads) {
				if err := m.markRefinementNeeded(); err != nil {
					return modes.ModeErrorMsg{Error: err}
				}
				m.SetStatusMsg("No ready beads remain. Entering refinement mode for audits.")
				return modes.ModeCompleteMsg{NextPhase: workflow.PhaseRefinement}
			}
			return modes.ModeErrorMsg{Error: err}
		}

		if err := m.ensureWorkInProgressMarker(); err != nil {
			return modes.ModeErrorMsg{Error: err}
		}

		return workPreparedMsg{sessions: sessions}
	}
}

func (m *Mode) markWorkComplete() error {
	ctx := m.Context()
	if ctx == nil {
		return fmt.Errorf("missing mode context")
	}
	workDir := ctx.Workflow.WorkDir()
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return err
	}
	inProgress := filepath.Join(workDir, workflow.MarkerWorkInProgress)
	_ = os.Remove(inProgress)
	completeMarker := filepath.Join(workDir, workflow.MarkerWorkComplete)
	return os.WriteFile(completeMarker, []byte{}, 0644)
}

func (m *Mode) ensureWorkInProgressMarker() error {
	ctx := m.Context()
	if ctx == nil {
		return fmt.Errorf("missing mode context")
	}
	workDir := ctx.Workflow.WorkDir()
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return err
	}
	marker := filepath.Join(workDir, workflow.MarkerWorkInProgress)
	return os.WriteFile(marker, []byte("active\n"), 0644)
}

func (m *Mode) markRefinementNeeded() error {
	ctx := m.Context()
	if ctx == nil {
		return fmt.Errorf("missing mode context")
	}
	workDir := ctx.Workflow.WorkDir()
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return err
	}
	marker := filepath.Join(workDir, workflow.MarkerRefinementNeeded)
	return os.WriteFile(marker, []byte("pending audits\n"), 0644)
}

func (m *Mode) runUpCycle() tea.Cmd {
	sessions := append([]orchestrator.WorktreeSession(nil), m.sessions...)
	return func() tea.Msg {
		ctx := context.Background()
		modeCtx := m.Context()
		if modeCtx == nil || modeCtx.Orchestrator == nil {
			return upCycleCompleteMsg{err: fmt.Errorf("orchestrator not available")}
		}
		if err := modeCtx.Orchestrator.RunUpCycle(ctx, sessions); err != nil {
			return upCycleCompleteMsg{err: err}
		}
		if err := m.markWorkComplete(); err != nil {
			return upCycleCompleteMsg{err: err}
		}
		return upCycleCompleteMsg{}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
