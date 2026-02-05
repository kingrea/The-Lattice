// internal/tui/app.go
//
// This is the main TUI (Terminal User Interface) for Lattice.
// It uses bubbletea, which follows The Elm Architecture:
//
// 1. Model: Your application state
// 2. Update: A function that updates state based on messages
// 3. View: A function that renders state to a string
//
// The flow is: User Input -> Message -> Update -> New Model -> View -> Screen

package tui

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kingrea/The-Lattice/internal/config"
	"github.com/kingrea/The-Lattice/internal/logbook"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/orchestrator"
	"github.com/kingrea/The-Lattice/internal/workflow"
)

// appState represents which "screen" we're on
type appState int

const (
	stateMainMenu       appState = iota // Main menu with "Commission Work", etc.
	stateWorkflowSelect                 // Workflow picker before launching the engine
	stateCommissionWork                 // Running a commission workflow mode
	stateViewAgents                     // Viewing available agents (legacy)
)

const (
	boardRefreshInterval = 3 * time.Second
	statusWindowName     = "status"
	statusReturnHotkey   = "M-s"
)

// WorkflowDefinitionLoader resolves workflow definitions for the engine-backed view.
type WorkflowDefinitionLoader func(cfg *config.Config, workflowID string) (workflow.WorkflowDefinition, error)

// AppOption customizes App construction for tests and alternate runtimes.
type AppOption func(*App)

// WithWorkflowDefinitionLoader overrides the workflow definition loader used by the TUI.
func WithWorkflowDefinitionLoader(loader WorkflowDefinitionLoader) AppOption {
	return func(a *App) {
		if loader != nil {
			a.workflowLoader = loader
		}
	}
}

// WithModuleRegistryFactory allows tests to inject custom module registries.
func WithModuleRegistryFactory(factory func(*config.Config) (*module.Registry, error)) AppOption {
	return func(a *App) {
		if factory != nil {
			a.registryFactory = factory
		}
	}
}

var phaseOrder = []workflow.Phase{
	workflow.PhasePlanning,
	workflow.PhaseOrchestratorSelection,
	workflow.PhaseHiring,
	workflow.PhaseWorkProcess,
	workflow.PhaseRefinement,
	workflow.PhaseAgentRelease,
	workflow.PhaseWorkCleanup,
	workflow.PhaseOrchestratorRelease,
}

type boardFocus int

const (
	focusMenu boardFocus = iota
	focusSessions
)

type statusRefreshMsg struct {
	sessions []sessionItem
	cycle    orchestrator.CycleStatus
	hasCycle bool
	phase    workflow.Phase
	err      error
}

type sessionItem struct {
	Agent        string
	Worktree     string
	Number       int
	Points       int
	Beads        int
	Cycle        int
	Phase        string
	State        string
	Questions    int
	Waiting      int
	Window       string
	WindowActive bool
	LastUpdated  time.Time
}

type tmuxWindowInfo struct {
	Name   string
	Active bool
}

// App is the main application model. In bubbletea, this holds ALL your state.
type App struct {
	state        appState
	config       *config.Config
	orchestrator *orchestrator.Orchestrator
	workflow     *workflow.Workflow
	logbook      *logbook.Logbook

	workflowLoader        WorkflowDefinitionLoader
	registryFactory       func(*config.Config) (*module.Registry, error)
	workflowView          *workflowView
	workflowMenu          list.Model
	workflowChoices       []workflowOption
	selectedWorkflow      string
	pendingWorkflowResume bool

	// UI components
	mainMenu      list.Model // The main menu list
	statusMsg     string     // Status message to display
	err           error      // Any error to display
	lastLogStatus string

	// Window size (we get this from bubbletea)
	width  int
	height int

	// Status board data
	boardFocus       boardFocus
	sessionItems     []sessionItem
	sessionSelection int
	boardErr         string
	cycleStatus      orchestrator.CycleStatus
	hasCycleStatus   bool
	cachedPhase      workflow.Phase
	tmuxSession      string
	statusWindowName string
	statusReturnKey  string
}

// menuItem implements list.Item interface for our menu items
type menuItem struct {
	title string
	desc  string
}

func (i menuItem) Title() string       { return i.title }
func (i menuItem) Description() string { return i.desc }
func (i menuItem) FilterValue() string { return i.title }

type workflowOption struct {
	id    string
	title string
	desc  string
}

func (o workflowOption) Title() string       { return o.title }
func (o workflowOption) Description() string { return o.desc }
func (o workflowOption) FilterValue() string { return o.id }

func (o workflowOption) ID() string { return o.id }

// NewApp creates a new App instance
func NewApp(projectDir string, opts ...AppOption) (*App, error) {
	cfg, err := config.NewConfig(projectDir)
	if err != nil {
		return nil, err
	}
	wf := workflow.New(cfg.LatticeProjectDir)
	orch := orchestrator.New(cfg)
	logPath := filepath.Join(cfg.LatticeProjectDir, "logs", "journey.log")
	lb, err := logbook.New(logPath)
	if err == nil {
		lb.Info("Session opened · workflow phase: %s", wf.CurrentPhase().FriendlyName())
	}

	// Build menu items based on workflow state
	menuItems := buildMainMenu(wf)

	// Create the list components
	mainMenu := list.New(menuItems, list.NewDefaultDelegate(), 0, 0)
	mainMenu.Title = "⬡ THE TERMINAL"
	mainMenu.SetShowStatusBar(false)
	mainMenu.SetFilteringEnabled(false)
	workflowMenu := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	workflowMenu.Title = "Select Workflow"
	workflowMenu.SetShowStatusBar(false)
	workflowMenu.SetFilteringEnabled(false)

	app := &App{
		state:            stateMainMenu,
		config:           cfg,
		orchestrator:     orch,
		workflow:         wf,
		logbook:          lb,
		workflowLoader:   defaultWorkflowLoader,
		registryFactory:  defaultModuleRegistryFactory,
		mainMenu:         mainMenu,
		workflowMenu:     workflowMenu,
		selectedWorkflow: cfg.DefaultWorkflow(),
		boardFocus:       focusMenu,
		statusWindowName: statusWindowName,
		statusReturnKey:  statusReturnHotkey,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(app)
		}
	}
	app.refreshWorkflowMenu()
	if session, windowIdx, err := detectTmuxContext(); err == nil {
		app.tmuxSession = session
		if err := ensureStatusWindow(session, windowIdx, statusWindowName); err == nil {
			_ = bindStatusReturnKey(session, statusWindowName, statusReturnHotkey)
		}
	}
	return app, nil
}

// buildMainMenu creates the main menu items based on workflow state
func buildMainMenu(wf *workflow.Workflow) []list.Item {
	phase := wf.CurrentPhase()
	items := []list.Item{}

	// Show "Resume Work" if there's an active workflow
	if phase.IsResumable() {
		items = append(items, menuItem{
			title: fmt.Sprintf("Resume Work (%s)", phase.FriendlyName()),
			desc:  "Continue from where you left off",
		})
	}

	// Always show Commission Work option
	if phase == workflow.PhaseNone || phase == workflow.PhaseComplete {
		items = append(items, menuItem{
			title: "Commission Work",
			desc:  "Start a new orchestration task",
		})
	}

	items = append(items,
		menuItem{title: "View Agents", desc: "Browse available agents"},
		menuItem{title: "Settings", desc: "Configure Lattice"},
		menuItem{title: "Exit", desc: "Quit Lattice"},
	)

	return items
}

func (a *App) refreshWorkflowMenu() {
	options := a.buildWorkflowOptions()
	items := make([]list.Item, len(options))
	for i := range options {
		items[i] = options[i]
	}
	a.workflowChoices = options
	a.workflowMenu.SetItems(items)
	if len(items) == 0 {
		return
	}
	idx := a.workflowOptionIndex(a.activeWorkflowID())
	if idx < 0 {
		idx = 0
	}
	a.workflowMenu.Select(idx)
}

func (a *App) buildWorkflowOptions() []workflowOption {
	ids := a.workflowIDs()
	opts := make([]workflowOption, 0, len(ids))
	loader := a.workflowLoader
	for _, id := range ids {
		option := workflowOption{
			id:    id,
			title: humanizeWorkflowID(id),
			desc:  fmt.Sprintf("Workflow ID: %s", id),
		}
		if loader != nil && a.config != nil {
			if def, err := loader(a.config, id); err == nil {
				if name := strings.TrimSpace(def.Name); name != "" {
					option.title = name
				}
				var parts []string
				if desc := strings.TrimSpace(def.Description); desc != "" {
					parts = append(parts, desc)
				}
				if def.Metadata != nil {
					if use := strings.TrimSpace(def.Metadata["recommended_use"]); use != "" {
						parts = append(parts, fmt.Sprintf("Use: %s", use))
					}
				}
				parts = append(parts, fmt.Sprintf("ID: %s", id))
				option.desc = strings.Join(parts, " · ")
			} else if err != nil {
				a.logWarn("Workflow %s metadata unavailable: %v", id, err)
			}
		}
		opts = append(opts, option)
	}
	return opts
}

func (a *App) workflowIDs() []string {
	if a.config == nil {
		return []string{"commission-work", "quick-start", "solo"}
	}
	seen := map[string]struct{}{}
	ordered := []string{}
	appendID := func(values ...string) {
		for _, value := range values {
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			key := strings.ToLower(trimmed)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			ordered = append(ordered, trimmed)
		}
	}
	appendID(a.config.Project.Workflows.Available...)
	appendID(a.config.DefaultWorkflow())
	appendID(a.selectedWorkflow)
	appendID("commission-work", "quick-start", "solo")
	return ordered
}

func (a *App) workflowOptionIndex(id string) int {
	target := strings.ToLower(strings.TrimSpace(id))
	if target == "" {
		return -1
	}
	for idx, option := range a.workflowChoices {
		candidate := strings.ToLower(strings.TrimSpace(option.ID()))
		if candidate == target {
			return idx
		}
	}
	return -1
}

func (a *App) activeWorkflowID() string {
	if id := strings.TrimSpace(a.selectedWorkflow); id != "" {
		return id
	}
	if a.config != nil {
		if id := strings.TrimSpace(a.config.DefaultWorkflow()); id != "" {
			return id
		}
	}
	return "commission-work"
}

func (a *App) logInfo(format string, args ...any) {
	if a.logbook == nil {
		return
	}
	a.logbook.Info(format, args...)
}

func (a *App) logWarn(format string, args ...any) {
	if a.logbook == nil {
		return
	}
	a.logbook.Warn(format, args...)
}

func (a *App) logError(format string, args ...any) {
	if a.logbook == nil {
		return
	}
	a.logbook.Error(format, args...)
}

func (a *App) logProgress(status string) {
	status = strings.TrimSpace(status)
	if status == "" || status == a.lastLogStatus {
		return
	}
	a.lastLogStatus = status
	a.logInfo(status)
}

// Init is called once when the program starts.
func (a *App) Init() tea.Cmd {
	return a.fetchStatusSnapshot()
}

// Update is called when a message is received.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.mainMenu.SetSize(max(0, msg.Width-6), max(0, msg.Height-10))
		a.workflowMenu.SetSize(max(0, msg.Width-6), max(0, msg.Height-10))
		if a.state == stateCommissionWork && a.workflowView != nil {
			return a, a.workflowView.Update(msg)
		}
		return a, nil

	case statusRefreshMsg:
		if msg.err != nil {
			a.boardErr = msg.err.Error()
		} else {
			a.boardErr = ""
			a.sessionItems = msg.sessions
			if len(a.sessionItems) == 0 {
				a.sessionSelection = 0
			} else if a.sessionSelection >= len(a.sessionItems) {
				a.sessionSelection = len(a.sessionItems) - 1
			}
			a.cycleStatus = msg.cycle
			a.hasCycleStatus = msg.hasCycle
			a.cachedPhase = msg.phase
		}
		return a, a.scheduleStatusRefresh()

	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
			return a, tea.Quit
		case "q":
			if a.state == stateMainMenu {
				return a, tea.Quit
			}
		case "esc":
			if a.state != stateMainMenu {
				return a.returnToMainMenu()
			}
		case "r":
			a.statusMsg = "Refreshing status board..."
			return a, a.fetchStatusSnapshot()
		case "tab":
			if a.state == stateMainMenu {
				if a.boardFocus == focusMenu && len(a.sessionItems) > 0 {
					a.boardFocus = focusSessions
				} else {
					a.boardFocus = focusMenu
				}
			}
		case "right", "l":
			if a.state == stateMainMenu && len(a.sessionItems) > 0 {
				a.boardFocus = focusSessions
			}
		case "left", "h":
			if a.state == stateMainMenu {
				a.boardFocus = focusMenu
			}
		case "up", "k":
			if a.state == stateMainMenu && a.boardFocus == focusSessions && len(a.sessionItems) > 0 {
				if a.sessionSelection > 0 {
					a.sessionSelection--
				}
				return a, nil
			}
		case "down", "j":
			if a.state == stateMainMenu && a.boardFocus == focusSessions && len(a.sessionItems) > 0 {
				if a.sessionSelection < len(a.sessionItems)-1 {
					a.sessionSelection++
				}
				return a, nil
			}
		case "enter":
			switch a.state {
			case stateWorkflowSelect:
				return a.confirmWorkflowSelection()
			case stateMainMenu:
				if a.boardFocus == focusSessions {
					if cmd := a.openSelectedSessionWindow(); cmd != nil {
						return a, cmd
					}
					return a, nil
				}
				return a.handleMainMenuSelection()
			}
		}

	}

	var cmds []tea.Cmd
	switch a.state {
	case stateMainMenu:
		if a.boardFocus == focusMenu {
			var menuCmd tea.Cmd
			a.mainMenu, menuCmd = a.mainMenu.Update(msg)
			if menuCmd != nil {
				cmds = append(cmds, menuCmd)
			}
		}
	case stateWorkflowSelect:
		var menuCmd tea.Cmd
		a.workflowMenu, menuCmd = a.workflowMenu.Update(msg)
		if menuCmd != nil {
			cmds = append(cmds, menuCmd)
		}
	case stateCommissionWork:
		if a.workflowView != nil {
			if cmd := a.workflowView.Update(msg); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	return a, tea.Batch(cmds...)
}

// handleMainMenuSelection processes menu item selection
func (a *App) handleMainMenuSelection() (tea.Model, tea.Cmd) {
	item, ok := a.mainMenu.SelectedItem().(menuItem)
	if !ok {
		return a, nil
	}

	switch {
	case item.title == "Commission Work":
		a.logInfo("Menu · Commission Work selected")
		return a.beginWorkflowSelection(false)

	case strings.HasPrefix(item.title, "Resume Work"):
		a.logInfo("Menu · Resume Work selected (%s)", a.workflow.CurrentPhase().FriendlyName())
		return a.startWorkflowRun(true)

	case item.title == "View Agents":
		a.logInfo("Menu · View Agents selected")
		a.statusMsg = "Not implemented yet"
		return a, nil

	case item.title == "Settings":
		a.logInfo("Menu · Settings selected")
		a.statusMsg = "Not implemented yet"
		return a, nil

	case item.title == "Exit":
		a.logInfo("Menu · Exit selected")
		return a, tea.Quit
	}

	return a, nil
}

func (a *App) beginWorkflowSelection(resume bool) (tea.Model, tea.Cmd) {
	if len(a.workflowChoices) == 0 {
		a.refreshWorkflowMenu()
	}
	a.state = stateWorkflowSelect
	a.pendingWorkflowResume = resume
	a.boardFocus = focusMenu
	if a.width > 0 && a.height > 0 {
		a.workflowMenu.SetSize(max(0, a.width-6), max(0, a.height-10))
	}
	a.statusMsg = "Select a workflow to launch"
	return a, nil
}

func (a *App) confirmWorkflowSelection() (tea.Model, tea.Cmd) {
	item, ok := a.workflowMenu.SelectedItem().(workflowOption)
	if !ok {
		a.statusMsg = "Workflow selection unavailable"
		return a, nil
	}
	id := item.ID()
	if err := a.setWorkflowSelection(id); err != nil {
		a.statusMsg = fmt.Sprintf("Workflow selection failed: %v", err)
		a.logError("Workflow selection failed: %v", err)
		return a, nil
	}
	a.logInfo("Workflow · %s selected", id)
	return a.startWorkflowRun(a.pendingWorkflowResume)
}

func (a *App) setWorkflowSelection(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("workflow id is required")
	}
	if err := a.config.SetDefaultWorkflow(id); err != nil {
		return err
	}
	a.selectedWorkflow = a.config.DefaultWorkflow()
	a.refreshWorkflowMenu()
	return nil
}

// startWorkflowRun bootstraps the workflow engine UI in either start or resume mode.
func (a *App) startWorkflowRun(resume bool) (tea.Model, tea.Cmd) {
	a.state = stateCommissionWork
	a.pendingWorkflowResume = false
	a.workflowView = newWorkflowView(a, a.activeWorkflowID())
	cmd := a.workflowView.Init(resume)
	return a, cmd
}

// returnToMainMenu transitions back to the main menu
func (a *App) returnToMainMenu() (tea.Model, tea.Cmd) {
	a.state = stateMainMenu
	a.workflowView = nil
	a.pendingWorkflowResume = false
	a.logInfo("Returned to main menu (phase: %s)", a.workflow.CurrentPhase().FriendlyName())

	// Refresh menu items (workflow state may have changed)
	a.mainMenu.SetItems(buildMainMenu(a.workflow))
	a.refreshWorkflowMenu()

	return a, nil
}

// View renders the current state to a string.
func (a *App) View() string {
	width := a.width
	if width <= 0 {
		width = 100
	}
	rightWidth := max(32, width/3)
	leftWidth := width - rightWidth - 4
	if leftWidth < 40 {
		leftWidth = width - 4
	}
	if leftWidth < 20 {
		leftWidth = width
		rightWidth = 0
	}
	if a.state == stateMainMenu && a.boardFocus == focusMenu {
		a.mainMenu.SetSize(max(20, leftWidth-4), max(10, a.height-10))
	}
	var content string
	switch a.state {
	case stateMainMenu:
		content = a.mainMenu.View()
	case stateWorkflowSelect:
		content = a.renderWorkflowSelection()
	case stateCommissionWork:
		if a.workflowView != nil {
			content = a.workflowView.View()
		} else {
			content = "Loading sessions..."
		}
	case stateViewAgents:
		content = "Agent viewer not implemented"
	}
	return a.renderStatusBoard(content, leftWidth, rightWidth)
}

func (a *App) renderLogPanel() string {
	if a.logbook == nil {
		return ""
	}
	lines := a.logbook.Tail(8)
	if len(lines) == 0 {
		return ""
	}
	fileName := filepath.Base(a.logbook.Path())
	if fileName == "." || fileName == "" {
		fileName = "log"
	}
	head := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#5B8DEF")).
		Render(fmt.Sprintf("LOG · %s", fileName))
	body := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#AAAAAA")).
		Render(strings.Join(lines, "\n"))
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#444444")).
		Padding(0, 1).
		Render(fmt.Sprintf("%s\n%s", head, body))
	return box
}

func (a *App) renderStatusBoard(mainContent string, leftWidth, rightWidth int) string {
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FF6B6B")).
		MarginBottom(1).
		Render("⬡ LATTICE")
	left := lipgloss.JoinVertical(lipgloss.Left,
		a.renderPhasePanel(leftWidth-4),
		"",
		a.renderMainArea(mainContent, leftWidth-4),
	)
	leftBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#444444")).
		Padding(0, 1).
		Width(max(20, leftWidth)).
		Render(left)
	var body string
	if rightWidth > 0 {
		right := a.renderSessionsPanel(rightWidth - 4)
		rightBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#444444")).
			Padding(0, 1).
			Width(max(20, rightWidth)).
			Render(right)
		body = lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)
	} else {
		body = leftBox
	}
	sections := []string{header, body}
	if logPanel := a.renderLogPanel(); logPanel != "" {
		sections = append(sections, logPanel)
	}
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		MarginTop(1).
		Render(a.statusMsg)
	sections = append(sections, footer)
	return strings.Join(sections, "\n")
}

func (a *App) renderPhasePanel(width int) string {
	phase := a.cachedPhase
	if phase == 0 {
		phase = a.workflow.CurrentPhase()
	}
	pos, total := phasePosition(phase)
	phaseLine := fmt.Sprintf("%s (%d/%d)", phase.FriendlyName(), pos+1, total)
	nextPhases := upcomingPhases(phase)
	nextLine := ""
	if len(nextPhases) > 0 {
		var names []string
		for _, p := range nextPhases {
			names = append(names, p.FriendlyName())
		}
		nextLine = fmt.Sprintf("Next: %s", strings.Join(names, " → "))
	}
	cycleLine := "Cycle: none scheduled"
	if a.hasCycleStatus {
		cycleLine = fmt.Sprintf(
			"Cycle %d · %s · %d session(s)",
			a.cycleStatus.Cycle,
			strings.TrimSpace(a.cycleStatus.Status),
			a.cycleStatus.SessionCount,
		)
	}
	lines := []string{
		fmt.Sprintf("Phase: %s", phaseLine),
	}
	if nextLine != "" {
		lines = append(lines, nextLine)
	}
	lines = append(lines, cycleLine)
	if a.boardErr != "" {
		lines = append(lines, fmt.Sprintf("⚠ %s", a.boardErr))
	}
	return lipgloss.NewStyle().Width(max(20, width)).Render(strings.Join(lines, "\n"))
}

func (a *App) renderMainArea(content string, width int) string {
	if strings.TrimSpace(content) == "" {
		content = "Ready to commission work."
	}
	return lipgloss.NewStyle().Width(max(20, width)).Render(content)
}

func (a *App) renderWorkflowSelection() string {
	view := a.workflowMenu.View()
	if strings.TrimSpace(view) == "" {
		view = "No workflows available"
	}
	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#AAAAAA")).
		MarginTop(1).
		Render("Enter → launch workflow    Esc → cancel")
	return lipgloss.JoinVertical(lipgloss.Left, view, hint)
}

func (a *App) renderSessionsPanel(width int) string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#5B8DEF")).
		Render(fmt.Sprintf("Sessions (%d)", len(a.sessionItems)))
	if len(a.sessionItems) == 0 {
		note := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("No active worktrees. Commission work to launch agents.")
		return lipgloss.JoinVertical(lipgloss.Left, title, note, a.renderSessionInstructions())
	}
	var rows []string
	for i, item := range a.sessionItems {
		selected := a.boardFocus == focusSessions && i == a.sessionSelection
		rows = append(rows, a.renderSessionItem(item, selected, width))
	}
	body := strings.Join(rows, "\n")
	return lipgloss.JoinVertical(lipgloss.Left, title, body, a.renderSessionInstructions())
}

func (a *App) renderSessionInstructions() string {
	key := hotkeyLabel(a.statusReturnKey)
	instructions := fmt.Sprintf("Enter → follow session    %s → return to status", key)
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#AAAAAA")).
		MarginTop(1).
		Render(instructions)
}

func (a *App) renderSessionItem(item sessionItem, selected bool, width int) string {
	state := titleCase(item.State)
	phase := titleCase(item.Phase)
	line1 := fmt.Sprintf("%s · %s", item.Agent, item.Worktree)
	line2 := fmt.Sprintf("Cycle %d · %s (%s)", item.Cycle, state, phase)
	line3 := fmt.Sprintf("%d bead(s) · %d pt", item.Beads, item.Points)
	if item.Window != "" {
		windowLabel := item.Window
		if item.WindowActive {
			windowLabel += " · active"
		}
		line3 += fmt.Sprintf(" · tmux %s", windowLabel)
	} else {
		line3 += " · idle"
	}
	if item.Waiting > 0 {
		line3 += fmt.Sprintf(" · ⚠ waiting on %d response(s)", item.Waiting)
	}
	if !item.LastUpdated.IsZero() {
		line3 += fmt.Sprintf(" · updated %s ago", humanizeDuration(time.Since(item.LastUpdated)))
	}
	content := strings.Join([]string{line1, line2, line3}, "\n")
	style := lipgloss.NewStyle().Width(max(20, width)).Padding(0, 0, 1, 0)
	if selected {
		style = style.Bold(true).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#5B8DEF")).Padding(0, 1)
	}
	return style.Render(content)
}

func (a *App) fetchStatusSnapshot() tea.Cmd {
	return func() tea.Msg {
		return a.buildStatusSnapshot()
	}
}

func (a *App) scheduleStatusRefresh() tea.Cmd {
	return tea.Tick(boardRefreshInterval, func(time.Time) tea.Msg {
		return a.buildStatusSnapshot()
	})
}

func (a *App) buildStatusSnapshot() statusRefreshMsg {
	phase := a.workflow.CurrentPhase()
	snapshots, err := a.orchestrator.SessionSnapshots()
	if err != nil {
		return statusRefreshMsg{phase: phase, err: err}
	}
	windowMap := a.tmuxWindows()
	items := make([]sessionItem, 0, len(snapshots))
	for _, snap := range snapshots {
		ses := sessionItem{
			Agent:       snap.Worktree.Agent.Name,
			Worktree:    snap.Worktree.Name,
			Number:      snap.Worktree.Number,
			Points:      snap.Worktree.TotalPoints(),
			Beads:       len(snap.Worktree.Beads),
			Cycle:       snap.Status.Cycle,
			Phase:       snap.Status.Phase,
			State:       snap.Status.State,
			Questions:   snap.QuestionsTotal,
			Waiting:     snap.QuestionsWaiting,
			LastUpdated: snap.LastUpdated,
		}
		for _, candidate := range candidateWindowNames(snap) {
			if info, ok := windowMap[candidate]; ok {
				ses.Window = info.Name
				ses.WindowActive = info.Active
				break
			}
		}
		items = append(items, ses)
	}
	cycle, cerr := a.orchestrator.CurrentCycleStatus()
	hasCycle := true
	if cerr != nil {
		if errors.Is(cerr, orchestrator.ErrNoTrackedSessions) {
			hasCycle = false
		} else {
			return statusRefreshMsg{phase: phase, err: cerr}
		}
	}
	return statusRefreshMsg{
		sessions: items,
		cycle:    cycle,
		hasCycle: hasCycle,
		phase:    phase,
	}
}

func (a *App) openSelectedSessionWindow() tea.Cmd {
	if a.tmuxSession == "" || len(a.sessionItems) == 0 {
		return nil
	}
	item := a.sessionItems[a.sessionSelection]
	if item.Window == "" {
		a.statusMsg = fmt.Sprintf("%s · %s is idle", item.Agent, item.Worktree)
		return nil
	}
	target := fmt.Sprintf("%s:%s", a.tmuxSession, item.Window)
	return func() tea.Msg {
		_ = exec.Command("tmux", "select-window", "-t", target).Run()
		return nil
	}
}

func (a *App) tmuxWindows() map[string]tmuxWindowInfo {
	if a.tmuxSession == "" {
		return map[string]tmuxWindowInfo{}
	}
	cmd := exec.Command("tmux", "list-windows", "-t", a.tmuxSession, "-F", "#{window_name}::#{window_active}")
	output, err := cmd.Output()
	if err != nil {
		return map[string]tmuxWindowInfo{}
	}
	result := make(map[string]tmuxWindowInfo)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "::")
		if len(parts) != 2 {
			continue
		}
		result[parts[0]] = tmuxWindowInfo{Name: parts[0], Active: strings.TrimSpace(parts[1]) == "1"}
	}
	return result
}

func detectTmuxContext() (string, string, error) {
	cmd := exec.Command("tmux", "display-message", "-p", "#S::#I")
	output, err := cmd.Output()
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(strings.TrimSpace(string(output)), "::")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected tmux context: %s", strings.TrimSpace(string(output)))
	}
	return parts[0], parts[1], nil
}

func ensureStatusWindow(session, windowIdx, name string) error {
	if session == "" || windowIdx == "" {
		return fmt.Errorf("tmux session context missing")
	}
	cmd := exec.Command("tmux", "rename-window", "-t", fmt.Sprintf("%s:%s", session, windowIdx), name)
	return cmd.Run()
}

func bindStatusReturnKey(session, windowName, hotkey string) error {
	if session == "" || windowName == "" || hotkey == "" {
		return fmt.Errorf("missing tmux binding context")
	}
	target := fmt.Sprintf("%s:%s", session, windowName)
	cmd := exec.Command("tmux", "bind-key", "-n", hotkey, "select-window", "-t", target)
	return cmd.Run()
}

func candidateWindowNames(snapshot orchestrator.SessionSnapshot) []string {
	num := snapshot.Worktree.Number
	cycle := snapshot.Status.Cycle
	if cycle <= 0 {
		cycle = 1
	}
	return []string{
		fmt.Sprintf("worktree-agent-%d-%d", num, cycle),
		fmt.Sprintf("worktree-orchestrator-%d-%d", num, cycle),
	}
}

func titleCase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	return strings.ToUpper(lower[:1]) + lower[1:]
}

func humanizeDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func hotkeyLabel(key string) string {
	if strings.HasPrefix(strings.ToLower(key), "m-") {
		return fmt.Sprintf("Alt+%s", strings.ToUpper(strings.TrimPrefix(key, "M-")))
	}
	return strings.ToUpper(key)
}

func humanizeWorkflowID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "Workflow"
	}
	replacer := strings.NewReplacer("-", " ", "_", " ")
	sanitized := replacer.Replace(trimmed)
	parts := strings.Fields(sanitized)
	if len(parts) == 0 {
		return "Workflow"
	}
	for i, part := range parts {
		lower := strings.ToLower(part)
		parts[i] = strings.ToUpper(lower[:1]) + lower[1:]
	}
	return strings.Join(parts, " ")
}

func phasePosition(p workflow.Phase) (int, int) {
	for i, phase := range phaseOrder {
		if p == phase {
			return i, len(phaseOrder)
		}
	}
	return len(phaseOrder), len(phaseOrder)
}

func upcomingPhases(p workflow.Phase) []workflow.Phase {
	pos, _ := phasePosition(p)
	if pos+1 >= len(phaseOrder) {
		return nil
	}
	return phaseOrder[pos+1:]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
