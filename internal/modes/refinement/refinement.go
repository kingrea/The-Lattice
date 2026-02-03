package refinement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yourusername/lattice/internal/modes"
	"github.com/yourusername/lattice/internal/orchestrator"
	"github.com/yourusername/lattice/internal/workflow"
)

type Mode struct {
	modes.BaseMode
	width  int
	height int
	stage  refinementStage

	options          []string
	selection        int
	assignments      []stakeholderAssignment
	profile          projectProfile
	followUpSummary  string
	stakeholdersPath string
	auditDir         string
	synthesisPath    string
	manualWindowName string
}

type refinementStage int

const (
	stageRunning refinementStage = iota
	stageAwaitChoice
)

type stakeholderAssignment struct {
	Role   string
	Agent  orchestrator.ProjectAgent
	Reused bool
	Repeat bool
}

type refinementPipelineCompleteMsg struct {
	profile         projectProfile
	assignments     []stakeholderAssignment
	followUpSummary string
	synthesisPath   string
}

type manualReviewLaunchedMsg struct {
	window string
}

func New() *Mode {
	return &Mode{
		BaseMode: modes.NewBaseMode("Refinement", workflow.PhaseRefinement),
		stage:    stageRunning,
		options:  []string{"Manually review", "Close"},
	}
}

func (m *Mode) Init(ctx *modes.ModeContext) tea.Cmd {
	m.SetContext(ctx)
	m.SetStatusMsg("Running post-work refinement audits...")
	return m.runPipeline()
}

func (m *Mode) Update(msg tea.Msg) (modes.Mode, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case refinementPipelineCompleteMsg:
		m.stage = stageAwaitChoice
		m.assignments = msg.assignments
		m.profile = msg.profile
		m.followUpSummary = msg.followUpSummary
		m.synthesisPath = msg.synthesisPath
		m.SetStatusMsg("Stakeholder audits complete. Choose the next step.")
		return m, nil
	case manualReviewLaunchedMsg:
		m.manualWindowName = msg.window
		m.SetStatusMsg("Manual review session opened in tmux. Return here when done.")
		return m, nil
	case modes.ModeErrorMsg:
		return m, nil
	}
	return m, nil
}

func (m *Mode) View() string {
	if m.stage == stageRunning {
		return m.renderRunning()
	}
	return m.renderChoices()
}

func (m *Mode) renderRunning() string {
	style := lipgloss.NewStyle().Padding(1)
	lines := []string{
		"  🔄  Refinement",
		"",
		"  - Generating stakeholder slate",
		"  - Collecting audits",
		"  - Synthesizing beads",
		"  - Scheduling follow-up work",
		"",
		fmt.Sprintf("  %s", m.StatusMsg()),
	}
	return style.Render(strings.Join(lines, "\n"))
}

func (m *Mode) renderChoices() string {
	body := lipgloss.NewStyle().Padding(1)
	var sections []string
	sections = append(sections, "  ✅ Refinement complete")
	sections = append(sections, fmt.Sprintf("  Project profile: %s", m.profile.Summary()))
	sections = append(sections, fmt.Sprintf("  Follow-up: %s", m.nonEmpty(m.followUpSummary)))
	if m.stakeholdersPath != "" {
		sections = append(sections, fmt.Sprintf("  Stakeholders: %s", m.stakeholdersPath))
	}
	if m.auditDir != "" {
		sections = append(sections, fmt.Sprintf("  Audit folder: %s", m.auditDir))
	}
	if m.synthesisPath != "" {
		sections = append(sections, fmt.Sprintf("  Synthesis: %s", m.synthesisPath))
	}
	sections = append(sections, "", "  Stakeholder coverage:")
	for _, assignment := range m.assignments {
		badge := ""
		if assignment.Reused {
			badge = " · (returning)"
		}
		sections = append(sections, fmt.Sprintf("  - %s → %s%s", assignment.Role, assignment.Agent.Name, badge))
	}
	sections = append(sections, "", "  What next?")
	for i, option := range m.options {
		cursor := "  "
		if m.selection == i {
			cursor = "➤ "
		}
		sections = append(sections, fmt.Sprintf("%s%s", cursor, option))
	}
	if m.manualWindowName != "" {
		sections = append(sections, "", fmt.Sprintf("  Manual review window: %s", m.manualWindowName))
	}
	sections = append(sections, "", fmt.Sprintf("  %s", m.StatusMsg()))
	return body.Render(strings.Join(sections, "\n"))
}

func (m *Mode) handleKey(key tea.KeyMsg) (modes.Mode, tea.Cmd) {
	switch m.stage {
	case stageRunning:
		if key.String() == "esc" {
			return m, func() tea.Msg {
				return modes.ModeErrorMsg{Error: fmt.Errorf("refinement cancelled")}
			}
		}
	case stageAwaitChoice:
		switch key.String() {
		case "up", "k":
			if m.selection > 0 {
				m.selection--
			}
		case "down", "j":
			if m.selection < len(m.options)-1 {
				m.selection++
			}
		case "enter":
			if m.selection == 0 {
				if m.manualWindowName != "" {
					m.SetStatusMsg("Manual review already running.")
					return m, nil
				}
				return m, m.launchManualReview()
			}
			return m, m.finishRefinement()
		case "esc":
			return m, m.finishRefinement()
		}
	}
	return m, nil
}

func (m *Mode) runPipeline() tea.Cmd {
	return func() tea.Msg {
		ctx := m.Context()
		if ctx == nil || ctx.Orchestrator == nil || ctx.Workflow == nil || ctx.Config == nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("refinement context incomplete")}
		}
		profile := detectProjectProfile(ctx.Config.ProjectDir)
		assignments, stakeholdersPath, auditDir, err := m.planStakeholders(profile)
		if err != nil {
			return modes.ModeErrorMsg{Error: err}
		}
		m.stakeholdersPath = stakeholdersPath
		m.auditDir = auditDir
		if err := m.runAudits(profile, assignments); err != nil {
			return modes.ModeErrorMsg{Error: err}
		}
		summaryPath, err := ctx.Orchestrator.RunAuditSynthesis(auditDir, profile.Summary())
		if err != nil {
			return modes.ModeErrorMsg{Error: err}
		}
		var followUp string
		if followUp, err = m.runFollowUpCycle(); err != nil {
			if !errors.Is(err, orchestrator.ErrNoReadyBeads) {
				return modes.ModeErrorMsg{Error: err}
			}
			followUp = "No new beads were created"
		}
		return refinementPipelineCompleteMsg{
			profile:         profile,
			assignments:     assignments,
			followUpSummary: followUp,
			synthesisPath:   summaryPath,
		}
	}
}

func (m *Mode) planStakeholders(profile projectProfile) ([]stakeholderAssignment, string, string, error) {
	ctx := m.Context()
	if ctx == nil || ctx.Workflow == nil || ctx.Orchestrator == nil {
		return nil, "", "", fmt.Errorf("missing context")
	}
	agents, err := ctx.Orchestrator.LoadProjectAgents()
	if err != nil {
		return nil, "", "", err
	}
	workerList := ctx.Orchestrator.CurrentWorkerList()
	used := make(map[string]struct{})
	if workerList.Orchestrator != nil {
		used[strings.ToLower(strings.TrimSpace(workerList.Orchestrator.Name))] = struct{}{}
	}
	for _, worker := range workerList.Workers {
		used[strings.ToLower(strings.TrimSpace(worker.Name))] = struct{}{}
	}
	assignments, err := assignRoles(generateRoles(profile), agents, used)
	if err != nil {
		return nil, "", "", err
	}
	teamDir := ctx.Workflow.TeamDir()
	if err := os.MkdirAll(teamDir, 0755); err != nil {
		return nil, "", "", err
	}
	stakeholdersPath := filepath.Join(teamDir, "stakeholders.json")
	if err := writeStakeholderFile(stakeholdersPath, profile, assignments); err != nil {
		return nil, "", "", err
	}
	auditDir := filepath.Join(ctx.Workflow.Dir(), "audit")
	if err := os.MkdirAll(auditDir, 0755); err != nil {
		return nil, "", "", err
	}
	return assignments, stakeholdersPath, auditDir, nil
}

func assignRoles(roles []string, agents []orchestrator.ProjectAgent, used map[string]struct{}) ([]stakeholderAssignment, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("no agents available for stakeholder audits")
	}
	var unused, returning []orchestrator.ProjectAgent
	for _, agent := range agents {
		key := strings.ToLower(strings.TrimSpace(agent.Name))
		if _, ok := used[key]; ok {
			returning = append(returning, agent)
			continue
		}
		unused = append(unused, agent)
	}
	candidates := append(unused, returning...)
	if len(candidates) == 0 {
		candidates = returning
	}
	assignCount := make(map[string]int)
	assignments := make([]stakeholderAssignment, 0, len(roles))
	for i, role := range roles {
		agent := candidates[i%len(candidates)]
		key := strings.ToLower(strings.TrimSpace(agent.Name))
		assignCount[key]++
		_, reused := used[key]
		assignments = append(assignments, stakeholderAssignment{
			Role:   role,
			Agent:  agent,
			Reused: reused,
			Repeat: assignCount[key] > 1,
		})
	}
	sort.SliceStable(assignments, func(i, j int) bool { return assignments[i].Role < assignments[j].Role })
	return assignments, nil
}

func writeStakeholderFile(path string, profile projectProfile, assignments []stakeholderAssignment) error {
	payload := struct {
		ProjectType string                    `json:"projectType"`
		Tags        []string                  `json:"tags,omitempty"`
		GeneratedAt string                    `json:"generatedAt"`
		Roles       map[string]map[string]any `json:"roles"`
	}{
		ProjectType: profile.Type,
		Tags:        profile.Tags,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Roles:       make(map[string]map[string]any),
	}
	for _, assignment := range assignments {
		record := map[string]any{
			"name": assignment.Agent.Name,
		}
		if assignment.Agent.Path != "" {
			record["agentPath"] = assignment.Agent.Path
		}
		if assignment.Agent.Memory != "" {
			record["memoryPath"] = assignment.Agent.Memory
		}
		if assignment.Reused {
			record["reused"] = true
		}
		if assignment.Repeat {
			record["repeat"] = true
		}
		payload.Roles[assignment.Role] = record
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (m *Mode) runAudits(profile projectProfile, assignments []stakeholderAssignment) error {
	ctx := m.Context()
	if ctx == nil || ctx.Orchestrator == nil {
		return fmt.Errorf("missing orchestrator")
	}
	for _, assignment := range assignments {
		auditPath := filepath.Join(m.auditDir, fmt.Sprintf("%s-audit.md", slugify(assignment.Role)))
		if err := ctx.Orchestrator.RunStakeholderAudit(assignment.Role, assignment.Agent, auditPath, profile.Summary()); err != nil {
			return err
		}
	}
	return nil
}

func (m *Mode) runFollowUpCycle() (string, error) {
	ctx := m.Context()
	if ctx == nil || ctx.Orchestrator == nil || ctx.Workflow == nil {
		return "", fmt.Errorf("missing context")
	}
	sessions, err := ctx.Orchestrator.PrepareWorkCycle()
	if err != nil {
		return "", err
	}
	if err := m.ensureWorkInProgressMarker(); err != nil {
		return "", err
	}
	if err := ctx.Orchestrator.RunUpCycle(context.Background(), sessions); err != nil {
		return "", err
	}
	if err := m.markWorkComplete(); err != nil {
		return "", err
	}
	total := 0
	for _, session := range sessions {
		total += session.TotalPoints()
	}
	return fmt.Sprintf("Processed %d session(s), %d pt", len(sessions), total), nil
}

func (m *Mode) ensureWorkInProgressMarker() error {
	ctx := m.Context()
	if ctx == nil {
		return fmt.Errorf("missing context")
	}
	workDir := ctx.Workflow.WorkDir()
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(workDir, workflow.MarkerWorkInProgress), []byte("audit-cycle\n"), 0644)
}

func (m *Mode) markWorkComplete() error {
	ctx := m.Context()
	if ctx == nil {
		return fmt.Errorf("missing context")
	}
	workDir := ctx.Workflow.WorkDir()
	inProgress := filepath.Join(workDir, workflow.MarkerWorkInProgress)
	_ = os.Remove(inProgress)
	return os.WriteFile(filepath.Join(workDir, workflow.MarkerWorkComplete), []byte{}, 0644)
}

func (m *Mode) finishRefinement() tea.Cmd {
	manualWindow := m.manualWindowName
	return func() tea.Msg {
		ctx := m.Context()
		if ctx == nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("missing context")}
		}
		if manualWindow != "" {
			_ = ctx.Orchestrator.CloseWindow(manualWindow)
		}
		marker := filepath.Join(ctx.Workflow.WorkDir(), workflow.MarkerRefinementNeeded)
		_ = os.Remove(marker)
		return modes.ModeCompleteMsg{NextPhase: workflow.PhaseAgentRelease}
	}
}

func (m *Mode) launchManualReview() tea.Cmd {
	profile := m.profile
	return func() tea.Msg {
		ctx := m.Context()
		if ctx == nil || ctx.Orchestrator == nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("missing orchestrator")}
		}
		window, err := ctx.Orchestrator.LaunchManualReviewSession(profile.Summary())
		if err != nil {
			return modes.ModeErrorMsg{Error: err}
		}
		return manualReviewLaunchedMsg{window: window}
	}
}

func (m *Mode) nonEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(none)"
	}
	return value
}

func slugify(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return "role"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_':
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "role"
	}
	return result
}
