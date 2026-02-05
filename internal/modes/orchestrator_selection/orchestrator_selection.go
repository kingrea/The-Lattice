// internal/modes/orchestrator_selection/orchestrator_selection.go
//
// Orchestrator Selection mode handles choosing an agent to orchestrate the work.
// Input: .lattice/workflow/plan.md
// Output: .lattice/workflow/orchestrator.json

package orchestrator_selection

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kingrea/The-Lattice/internal/modes"
	"github.com/kingrea/The-Lattice/internal/orchestrator"
	"github.com/kingrea/The-Lattice/internal/workflow"
)

// Mode handles orchestrator selection phase
type Mode struct {
	modes.BaseMode
	agentList list.Model
	agents    []orchestrator.Agent
	width     int
	height    int
	listWidth int
	showSide  bool
	errorMsg  string
}

// agentItem wraps an Agent for the list display
type agentItem struct {
	agent orchestrator.Agent
}

func (i agentItem) Title() string       { return i.agent.Name }
func (i agentItem) Description() string { return i.agent.Byline }
func (i agentItem) FilterValue() string { return i.agent.Name }

// New creates a new Orchestrator Selection mode
func New() *Mode {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(2)
	delegate.SetSpacing(0)
	agentList := list.New([]list.Item{}, delegate, 0, 0)
	agentList.Title = "Select an Orchestrator"
	agentList.SetShowStatusBar(false)
	agentList.SetFilteringEnabled(true)

	return &Mode{
		BaseMode:  modes.NewBaseMode("Orchestrator Selection", workflow.PhaseOrchestratorSelection),
		agentList: agentList,
	}
}

// Init initializes the orchestrator selection mode
func (m *Mode) Init(ctx *modes.ModeContext) tea.Cmd {
	m.SetContext(ctx)
	if ctx != nil && ctx.Logbook != nil {
		ctx.Logbook.Info("Starting orchestrator selection")
	}

	// Check if orchestrator already selected
	wf := ctx.Workflow
	if fileExists(wf.OrchestratorPath()) {
		m.SetComplete(true)
		m.SetStatusMsg("Orchestrator already selected, skipping to next phase")
		return func() tea.Msg {
			return modes.ModeCompleteMsg{NextPhase: workflow.PhaseHiring}
		}
	}

	m.SetStatusMsg("Loading available agents...")
	return m.loadAgents()
}

// Update handles messages for the orchestrator selection mode
func (m *Mode) Update(msg tea.Msg) (modes.Mode, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		availableWidth := msg.Width - 4
		if availableWidth < 20 {
			availableWidth = msg.Width
		}
		m.showSide = msg.Width >= 90
		if m.showSide {
			m.listWidth = int(float64(availableWidth) * 0.45)
			if m.listWidth < 36 {
				m.listWidth = 36
			}
		} else {
			m.listWidth = availableWidth
		}
		listHeight := msg.Height - 8
		if listHeight < 5 {
			listHeight = msg.Height - 2
		}
		m.agentList.SetSize(m.listWidth, listHeight)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			return m, func() tea.Msg {
				return modes.ModeErrorMsg{Error: fmt.Errorf("orchestrator selection cancelled")}
			}
		case "enter":
			return m.selectAgent()
		}

	case agentsLoadedMsg:
		m.errorMsg = ""
		m.agents = msg.agents
		items := make([]list.Item, len(msg.agents))
		for i, agent := range msg.agents {
			items[i] = agentItem{agent: agent}
		}
		m.agentList.SetItems(items)
		m.SetStatusMsg(fmt.Sprintf("Found %d agents", len(msg.agents)))
		m.logInfo("Loaded %d orchestrator candidates", len(msg.agents))
		return m, nil

	case selectionFailedMsg:
		if msg.err != nil {
			m.errorMsg = msg.err.Error()
			m.SetStatusMsg("Failed to select orchestrator. See log for details.")
			m.logWarn("Orchestrator selection failed: %v", msg.err)
		}
		return m, nil

	case orchestratorSavedMsg:
		m.errorMsg = ""
		m.logInfo("Selected orchestrator: %s", msg.name)
		m.SetComplete(true)
		m.SetStatusMsg(fmt.Sprintf("Selected %s as orchestrator", msg.name))
		return m, func() tea.Msg {
			return modes.ModeCompleteMsg{NextPhase: workflow.PhaseHiring}
		}

	case modes.ModeErrorMsg:
		m.SetStatusMsg(fmt.Sprintf("Error: %v", msg.Error))
		return m, nil
	}

	// Pass to list
	var cmd tea.Cmd
	m.agentList, cmd = m.agentList.Update(msg)
	return m, cmd
}

// View renders the orchestrator selection mode
func (m *Mode) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#6BCB77")).
		MarginBottom(1)

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		MarginTop(1)

	header := titleStyle.Render("⬡ SELECT ORCHESTRATOR")
	content := m.renderContent()
	if m.errorMsg != "" {
		errBlock := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF6B6B")).
			Padding(0, 1).
			Render(fmt.Sprintf("⚠ %s", m.errorMsg))
		content = fmt.Sprintf("%s\n\n%s", content, errBlock)
	}
	footer := statusStyle.Render(m.StatusMsg())

	return fmt.Sprintf("%s\n%s\n%s", header, content, footer)
}

func (m *Mode) renderContent() string {
	listView := m.agentList.View()
	detail := m.renderSelectedAgentDetail()
	if detail == "" {
		return listView
	}
	if m.showSide {
		return lipgloss.JoinHorizontal(lipgloss.Top, listView, detail)
	}
	return fmt.Sprintf("%s\n\n%s", listView, detail)
}

func (m *Mode) renderSelectedAgentDetail() string {
	agent := m.selectedAgent()
	if agent == nil {
		return ""
	}
	detailWidth := m.width - m.listWidth - 6
	if !m.showSide {
		detailWidth = m.width - 4
	}
	if detailWidth < 36 {
		detailWidth = 36
	}
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFD479"))
	sectionTitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6BCB77")).
		Bold(true)
	bodyStyle := lipgloss.NewStyle().
		Width(detailWidth).
		Padding(0, 1)
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#555555")).
		Padding(1, 2).
		Width(detailWidth + 4)

	stats := fmt.Sprintf("Precision %d   Autonomy %d   Experience %d", agent.Precision, agent.Autonomy, agent.Experience)
	var sections []string
	sections = append(sections, titleStyle.Render(fmt.Sprintf("%s · %s", agent.Name, agent.Community)))
	if strings.TrimSpace(agent.Byline) != "" {
		sections = append(sections, agent.Byline)
	}
	sections = append(sections, stats)
	if agent.Summary != "" {
		sections = append(sections, fmt.Sprintf("%s\n%s", sectionTitle.Render("Summary"), agent.Summary))
	}
	if agent.WorkStyle != "" {
		sections = append(sections, fmt.Sprintf("%s\n%s", sectionTitle.Render("Working Style"), agent.WorkStyle))
	}
	if agent.Edges != "" {
		sections = append(sections, fmt.Sprintf("%s\n%s", sectionTitle.Render("Edges"), agent.Edges))
	}
	body := bodyStyle.Render(strings.Join(sections, "\n\n"))
	return borderStyle.Render(body)
}

func (m *Mode) selectedAgent() *orchestrator.Agent {
	item, ok := m.agentList.SelectedItem().(agentItem)
	if !ok {
		return nil
	}
	return &item.agent
}

// Message types
type agentsLoadedMsg struct {
	agents []orchestrator.Agent
}

type orchestratorSavedMsg struct {
	name string
}

type selectionFailedMsg struct {
	err error
}

// loadAgents fetches available agents
func (m *Mode) loadAgents() tea.Cmd {
	return func() tea.Msg {
		ctx := m.Context()
		orch := orchestrator.New(ctx.Config)
		agents, err := orch.LoadDenizenCVs()
		if err != nil {
			return modes.ModeErrorMsg{Error: err}
		}
		return agentsLoadedMsg{agents: agents}
	}
}

// selectAgent saves the selected agent as orchestrator
func (m *Mode) selectAgent() (modes.Mode, tea.Cmd) {
	item, ok := m.agentList.SelectedItem().(agentItem)
	if !ok {
		return m, nil
	}

	return m, func() tea.Msg {
		ctx := m.Context()

		// Save orchestrator to workflow file
		data := map[string]string{
			"name":      item.agent.Name,
			"community": item.agent.Community,
			"cvPath":    item.agent.CVPath,
		}

		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return modes.ModeErrorMsg{Error: err}
		}

		if err := os.WriteFile(ctx.Workflow.OrchestratorPath(), jsonData, 0644); err != nil {
			return modes.ModeErrorMsg{Error: err}
		}

		if ctx.Orchestrator == nil {
			return selectionFailedMsg{err: fmt.Errorf("orchestrator engine unavailable")}
		}
		if err := ctx.Orchestrator.ApplyOrchestratorSelection(item.agent); err != nil {
			return selectionFailedMsg{err: err}
		}

		return orchestratorSavedMsg{name: item.agent.Name}
	}
}

func (m *Mode) logInfo(format string, args ...any) {
	ctx := m.Context()
	if ctx == nil || ctx.Logbook == nil {
		return
	}
	ctx.Logbook.Info(format, args...)
}

func (m *Mode) logWarn(format string, args ...any) {
	ctx := m.Context()
	if ctx == nil || ctx.Logbook == nil {
		return
	}
	ctx.Logbook.Warn(format, args...)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
