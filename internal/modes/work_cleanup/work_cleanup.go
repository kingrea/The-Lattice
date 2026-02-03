// internal/modes/work_cleanup/work_cleanup.go
//
// Work Cleanup mode handles cleaning up after work completion.
// Input: .lattice/workflow/release/.agents-released
// Output: .lattice/workflow/release/.cleanup-done

package work_cleanup

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yourusername/lattice/internal/modes"
	"github.com/yourusername/lattice/internal/workflow"
)

// Mode handles the work cleanup phase
type Mode struct {
	modes.BaseMode
	width  int
	height int
}

// New creates a new Work Cleanup mode
func New() *Mode {
	return &Mode{
		BaseMode: modes.NewBaseMode("Work Cleanup", workflow.PhaseWorkCleanup),
	}
}

// Init initializes the work cleanup mode
func (m *Mode) Init(ctx *modes.ModeContext) tea.Cmd {
	m.SetContext(ctx)

	// Check if cleanup already done
	wf := ctx.Workflow
	cleanupMarker := filepath.Join(wf.ReleaseDir(), workflow.MarkerCleanupDone)
	if fileExists(cleanupMarker) {
		m.SetComplete(true)
		m.SetStatusMsg("Cleanup already complete, skipping to next phase")
		return func() tea.Msg {
			return modes.ModeCompleteMsg{NextPhase: workflow.PhaseOrchestratorRelease}
		}
	}

	m.SetStatusMsg("Cleaning up work artifacts...")
	return m.performCleanup()
}

// Update handles messages for the work cleanup mode
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
				return modes.ModeErrorMsg{Error: fmt.Errorf("cleanup cancelled")}
			}
		}

	case cleanupCompleteMsg:
		m.SetComplete(true)
		m.SetStatusMsg("Cleanup complete!")
		return m, func() tea.Msg {
			return modes.ModeCompleteMsg{NextPhase: workflow.PhaseOrchestratorRelease}
		}

	case modes.ModeErrorMsg:
		m.SetStatusMsg(fmt.Sprintf("Error: %v", msg.Error))
		return m, nil
	}

	return m, nil
}

// View renders the work cleanup mode
func (m *Mode) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#F39C12")).
		Padding(1)

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		MarginTop(1)

	content := titleStyle.Render(`
  🧹 WORK CLEANUP

  The cleanup process:
  1. Archive completed work artifacts
  2. Clean up temporary files
  3. Prepare final summary
  4. Restore any prior opencode config from opencode.old.json(c) if needed

  Waiting for cleanup to complete...

  Press ESC to cancel.
`)

	return fmt.Sprintf("%s\n%s", content, statusStyle.Render(m.StatusMsg()))
}

// Message types
type cleanupCompleteMsg struct{}

// performCleanup does the actual cleanup
func (m *Mode) performCleanup() tea.Cmd {
	return func() tea.Msg {
		ctx := m.Context()
		if ctx == nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("missing mode context")}
		}

		releaseDir := ctx.Workflow.ReleaseDir()
		if err := os.MkdirAll(releaseDir, 0755); err != nil {
			return modes.ModeErrorMsg{Error: err}
		}

		if err := archiveWorkLog(ctx, releaseDir); err != nil {
			return modes.ModeErrorMsg{Error: err}
		}

		if err := archiveLogs(ctx, releaseDir); err != nil {
			return modes.ModeErrorMsg{Error: err}
		}

		if err := purgeWorktreeDir(ctx.Config.WorktreeDir()); err != nil {
			return modes.ModeErrorMsg{Error: err}
		}

		marker := filepath.Join(releaseDir, workflow.MarkerCleanupDone)
		if err := os.WriteFile(marker, []byte{}, 0644); err != nil {
			return modes.ModeErrorMsg{Error: err}
		}

		return cleanupCompleteMsg{}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func archiveWorkLog(ctx *modes.ModeContext, releaseDir string) error {
	workLog := filepath.Join(ctx.Workflow.WorkDir(), workflow.FileWorkLog)
	data, err := os.ReadFile(workLog)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read work log: %w", err)
	}
	timestamp := time.Now().UTC().Format("20060102-150405")
	dest := filepath.Join(releaseDir, fmt.Sprintf("work-log-%s.md", timestamp))
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return fmt.Errorf("archive work log: %w", err)
	}
	if err := os.Remove(workLog); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func archiveLogs(ctx *modes.ModeContext, releaseDir string) error {
	logsDir := ctx.Config.LogsDir()
	ready, err := dirHasEntries(logsDir)
	if err != nil {
		return err
	}
	if !ready {
		return nil
	}
	timestamp := time.Now().UTC().Format("20060102-150405")
	dest := filepath.Join(releaseDir, fmt.Sprintf("logs-%s", timestamp))
	if err := copyDir(logsDir, dest); err != nil {
		return err
	}
	if err := os.RemoveAll(logsDir); err != nil {
		return err
	}
	return os.MkdirAll(logsDir, 0755)
}

func purgeWorktreeDir(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove worktree dir: %w", err)
	}
	return nil
}

func dirHasEntries(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return len(entries) > 0, nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target, d)
	})
}

func copyFile(src, dst string, d fs.DirEntry) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := d.Info()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	mode := info.Mode()
	target, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer target.Close()
	if _, err := io.Copy(target, source); err != nil {
		return err
	}
	return nil
}
