// internal/modes/mode.go
//
// Defines the Mode interface that all workflow modes implement.
// Each mode is self-contained and communicates via file artifacts.

package modes

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kingrea/The-Lattice/internal/config"
	"github.com/kingrea/The-Lattice/internal/logbook"
	"github.com/kingrea/The-Lattice/internal/orchestrator"
	"github.com/kingrea/The-Lattice/internal/workflow"
)

// ModeContext provides shared context for all modes
type ModeContext struct {
	Config       *config.Config
	Workflow     *workflow.Workflow
	Orchestrator *orchestrator.Orchestrator
	Logbook      *logbook.Logbook
}

// Mode defines the interface that all workflow modes must implement
type Mode interface {
	// Name returns the mode's display name
	Name() string

	// Phase returns which workflow phase this mode handles
	Phase() workflow.Phase

	// Init initializes the mode and returns a startup command
	Init(ctx *ModeContext) tea.Cmd

	// Update handles messages and returns the updated mode plus any commands
	// If the mode is complete, it should return a ModeCompleteMsg
	Update(msg tea.Msg) (Mode, tea.Cmd)

	// View renders the mode's current state
	View() string

	// IsComplete returns true if the mode has finished its work
	IsComplete() bool
}

// ModeCompleteMsg signals that a mode has finished and the workflow should advance
type ModeCompleteMsg struct {
	// NextPhase indicates which phase comes next
	NextPhase workflow.Phase
	// Error if the mode failed
	Error error
}

// ModeProgressMsg provides status updates during mode execution
type ModeProgressMsg struct {
	Status string
}

// ModeErrorMsg signals an error occurred during mode execution
type ModeErrorMsg struct {
	Error error
}

// BaseMode provides common functionality for all modes
type BaseMode struct {
	ctx       *ModeContext
	name      string
	phase     workflow.Phase
	complete  bool
	statusMsg string
}

// NewBaseMode creates a new BaseMode with the given name and phase
func NewBaseMode(name string, phase workflow.Phase) BaseMode {
	return BaseMode{
		name:  name,
		phase: phase,
	}
}

// Name returns the mode's display name
func (m *BaseMode) Name() string {
	return m.name
}

// Phase returns which workflow phase this mode handles
func (m *BaseMode) Phase() workflow.Phase {
	return m.phase
}

// IsComplete returns true if the mode has finished
func (m *BaseMode) IsComplete() bool {
	return m.complete
}

// SetComplete marks the mode as complete
func (m *BaseMode) SetComplete(complete bool) {
	m.complete = complete
}

// Context returns the mode context
func (m *BaseMode) Context() *ModeContext {
	return m.ctx
}

// SetContext sets the mode context
func (m *BaseMode) SetContext(ctx *ModeContext) {
	m.ctx = ctx
}

// StatusMsg returns the current status message
func (m *BaseMode) StatusMsg() string {
	return m.statusMsg
}

// SetStatusMsg sets the status message
func (m *BaseMode) SetStatusMsg(msg string) {
	m.statusMsg = msg
}
