// internal/modes/hiring/hiring.go
//
// Hiring mode handles recruiting worker agents for the commission.
// If not enough agents are available, creates SPARK agents (SPARK-01, SPARK-02, etc.)
// Input: .lattice/workflow/orchestrator.json, .lattice/workflow/plan.md
// Output: .lattice/workflow/team/workers.json

package hiring

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kingrea/The-Lattice/internal/modes"
	"github.com/kingrea/The-Lattice/internal/orchestrator"
	"github.com/kingrea/The-Lattice/internal/skills"
	"github.com/kingrea/The-Lattice/internal/workflow"
)

// Mode handles the hiring phase
type Mode struct {
	modes.BaseMode
	workers []workflow.WorkerEntry
	width   int
	height  int
}

const (
	minWorkersRequired    = 10
	defaultSpecialists    = 2
	maxAgentStoryPoints   = 8
	specialistStoryPoints = 4
	sparkNameFormat       = "[spark-%02d]"
	workerRole            = "worker"
	specialistRole        = "specialist"
	bdCreateTimeout       = 2 * time.Minute
	opencodeSkillTimeout  = 5 * time.Minute
)

type hiredAgent struct {
	entry  workflow.WorkerEntry
	source string
}

// New creates a new Hiring mode
func New() *Mode {
	return &Mode{
		BaseMode: modes.NewBaseMode("Hiring", workflow.PhaseHiring),
		workers:  []workflow.WorkerEntry{},
	}
}

// Init initializes the hiring mode
func (m *Mode) Init(ctx *modes.ModeContext) tea.Cmd {
	m.SetContext(ctx)

	// Check if workers already hired
	wf := ctx.Workflow
	if fileExists(wf.WorkersPath()) {
		m.SetComplete(true)
		m.SetStatusMsg("Workers already hired, skipping to next phase")
		return func() tea.Msg {
			return modes.ModeCompleteMsg{NextPhase: workflow.PhaseWorkProcess}
		}
	}

	m.SetStatusMsg("Starting hiring process...")
	return m.startHiring()
}

// Update handles messages for the hiring mode
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
				return modes.ModeErrorMsg{Error: fmt.Errorf("hiring cancelled")}
			}
		}

	case hiringCompleteMsg:
		m.workers = msg.workers
		m.SetComplete(true)
		m.SetStatusMsg(fmt.Sprintf("Hired %d workers", len(msg.workers)))
		return m, func() tea.Msg {
			return modes.ModeCompleteMsg{NextPhase: workflow.PhaseWorkProcess}
		}

	case modes.ModeErrorMsg:
		m.SetStatusMsg(fmt.Sprintf("Error: %v", msg.Error))
		return m, nil
	}

	return m, nil
}

// View renders the hiring mode
func (m *Mode) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#4D96FF")).
		Padding(1)

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		MarginTop(1)

	content := titleStyle.Render(`
  👥 HIRING WORKERS

  The hiring process will:
  1. Analyze the work plan
  2. Determine required skills
  3. Select available agents or create SPARK agents

  Waiting for hiring to complete...

  Press ESC to cancel.
`)

	return fmt.Sprintf("%s\n%s", content, statusStyle.Render(m.StatusMsg()))
}

// Message types
type hiringCompleteMsg struct {
	workers []workflow.WorkerEntry
}

// startHiring begins the hiring process
func (m *Mode) startHiring() tea.Cmd {
	return func() tea.Msg {
		ctx := m.Context()
		workers, err := m.performHiring(ctx)
		if err != nil {
			return modes.ModeErrorMsg{Error: err}
		}
		return hiringCompleteMsg{workers: workers}
	}
}

func (m *Mode) performHiring(ctx *modes.ModeContext) ([]workflow.WorkerEntry, error) {
	projectDir := ctx.Config.ProjectDir
	totalPoints, beadCount, err := analyzeWorkload(projectDir)
	if err != nil {
		beadCount = minWorkersRequired
	}
	maxParallel := computeMaxParallel(totalPoints, beadCount)
	baseWorkers := maxInt(minWorkersRequired, maxParallel)
	totalNeeded := baseWorkers + defaultSpecialists
	hiRes, err := m.selectAgents(ctx, baseWorkers, totalNeeded)
	if err != nil {
		return nil, err
	}
	entries := make([]workflow.WorkerEntry, len(hiRes))
	for i := range hiRes {
		entries[i] = hiRes[i].entry
	}
	if err := writeWorkersFile(ctx.Workflow, entries); err != nil {
		return nil, err
	}
	if err := updateWorkerList(ctx.Config.WorkerListPath(), entries); err != nil {
		return nil, err
	}
	if err := createHireBeads(projectDir, entries); err != nil {
		return nil, err
	}
	if err := m.generateAgentFiles(ctx, hiRes); err != nil {
		return nil, err
	}
	if ctx.Orchestrator != nil {
		if err := ctx.Orchestrator.RefreshOpenCodeConfig(); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func analyzeWorkload(projectDir string) (int, int, error) {
	cmd := exec.Command("bd", "ready", "--json")
	cmd.Dir = projectDir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return 0, 0, err
	}
	points, count, err := parseBeadStats(out.Bytes())
	return points, count, err
}

func parseBeadStats(data []byte) (int, int, error) {
	type beadRecord struct {
		ID       string          `json:"id"`
		Title    string          `json:"title"`
		Points   json.RawMessage `json:"points"`
		Estimate json.RawMessage `json:"estimate"`
		Size     json.RawMessage `json:"size"`
	}
	var arr []beadRecord
	if err := json.Unmarshal(data, &arr); err != nil {
		var wrapper struct {
			Items []beadRecord `json:"items"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return 0, 0, err
		}
		arr = wrapper.Items
	}
	total := 0
	count := 0
	for _, rec := range arr {
		if strings.TrimSpace(rec.ID) == "" {
			continue
		}
		count++
		points := parsePointValue(rec.Points)
		if points == 0 {
			points = parsePointValue(rec.Estimate)
		}
		if points == 0 {
			points = parsePointValue(rec.Size)
		}
		if points == 0 {
			points = 1
		}
		total += points
	}
	return total, count, nil
}

func parsePointValue(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err == nil {
		return int(math.Ceil(v))
	}
	return 0
}

func computeMaxParallel(totalPoints, beadCount int) int {
	maxParallel := 0
	if totalPoints > 0 {
		maxParallel = int(math.Ceil(float64(totalPoints) / float64(maxAgentStoryPoints)))
	}
	if beadCount > maxParallel {
		maxParallel = beadCount
	}
	if maxParallel < minWorkersRequired {
		maxParallel = minWorkersRequired
	}
	return maxParallel
}

func (m *Mode) selectAgents(ctx *modes.ModeContext, workerCount, totalNeeded int) ([]hiredAgent, error) {
	candidates, err := ctx.Orchestrator.LoadDenizenCVs()
	if err != nil {
		return nil, fmt.Errorf("failed to load denizen CVs: %w", err)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return strings.ToLower(candidates[i].Name) < strings.ToLower(candidates[j].Name)
	})
	used := make(map[string]struct{})
	selected := make([]hiredAgent, 0, totalNeeded)
	for _, agent := range candidates {
		key := strings.ToLower(strings.TrimSpace(agent.Name))
		if key == "" {
			continue
		}
		if _, ok := used[key]; ok {
			continue
		}
		selected = append(selected, hiredAgent{
			entry: workflow.WorkerEntry{
				Name:      agent.Name,
				Community: agent.Community,
			},
			source: filepath.Dir(agent.CVPath),
		})
		used[key] = struct{}{}
		if len(selected) >= totalNeeded {
			break
		}
	}
	sparkCounter := 1
	for len(selected) < totalNeeded {
		name := fmt.Sprintf(sparkNameFormat, sparkCounter)
		sparkCounter++
		selected = append(selected, hiredAgent{
			entry: workflow.WorkerEntry{
				Name:      name,
				Community: "spark",
				IsSpark:   true,
			},
		})
	}
	for i := range selected {
		if i < workerCount {
			selected[i].entry.Role = workerRole
			selected[i].entry.Capacity = maxAgentStoryPoints
		} else {
			selected[i].entry.Role = specialistRole
			selected[i].entry.Capacity = specialistStoryPoints
		}
	}
	return selected, nil
}

func writeWorkersFile(wf *workflow.Workflow, workers []workflow.WorkerEntry) error {
	if err := os.MkdirAll(wf.TeamDir(), 0755); err != nil {
		return err
	}
	return workflow.SaveWorkers(wf.WorkersPath(), workers)
}

func updateWorkerList(path string, workers []workflow.WorkerEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	var state struct {
		Orchestrator *orchestrator.WorkerRef  `json:"orchestrator,omitempty"`
		Workers      []orchestrator.WorkerRef `json:"workers,omitempty"`
		UpdatedAt    string                   `json:"updatedAt"`
	}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	state.Workers = make([]orchestrator.WorkerRef, 0, len(workers))
	for _, worker := range workers {
		state.Workers = append(state.Workers, orchestrator.WorkerRef{Name: worker.Name})
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func createHireBeads(projectDir string, workers []workflow.WorkerEntry) error {
	epicID, err := runBdCreate(projectDir, []string{"-t", "epic", "-p", "1"}, "HIRE")
	if err != nil {
		return err
	}
	for _, worker := range workers {
		title := fmt.Sprintf("Create agent file for %s (%s)", worker.Name, worker.Role)
		args := []string{"--parent", epicID}
		if _, err := runBdCreate(projectDir, args, title); err != nil {
			return err
		}
	}
	return nil
}

func runBdCreate(projectDir string, extra []string, title string) (string, error) {
	args := append([]string{"create", title}, extra...)
	cmd := exec.Command("bd", args...)
	cmd.Dir = projectDir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("bd %s failed: %s", strings.Join(args, " "), out.String())
	}
	id := extractBeadID(out.String())
	if id == "" {
		return "", fmt.Errorf("unable to parse bead id from output: %s", out.String())
	}
	return id, nil
}

func extractBeadID(output string) string {
	re := regexp.MustCompile(`\[(?P<id>[^\]]+)\]`)
	match := re.FindStringSubmatch(output)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func (m *Mode) generateAgentFiles(ctx *modes.ModeContext, hires []hiredAgent) error {
	skillPath, err := skills.Ensure(ctx.Config.SkillsDir(), skills.CreateAgentFile)
	if err != nil {
		return err
	}
	baseDir := ctx.Config.AgentsDir()
	for _, hire := range hires {
		roleDir := "workers"
		roleContext := "worker"
		if hire.entry.Role == specialistRole {
			roleDir = "specialists"
			roleContext = "specialist"
		}
		slug := slugifyName(hire.entry.Name)
		rolePath := filepath.Join(roleDir, slug)
		targetDir := filepath.Join(baseDir, rolePath)
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return err
		}
		targetFile := filepath.Join(targetDir, "AGENT.md")
		if hire.entry.IsSpark || hire.source == "" {
			if err := writeSparkAgent(targetFile, hire.entry.Name, hire.entry.Role); err != nil {
				return err
			}
			continue
		}
		stagedDir, err := stageHireSource(ctx, hire)
		if err != nil {
			return err
		}
		if err := runCreateAgentFileSkill(ctx, hire, rolePath, stagedDir, targetFile, skillPath, roleContext); err != nil {
			return err
		}
	}
	return nil
}

func runCreateAgentFileSkill(ctx *modes.ModeContext, hire hiredAgent, rolePath, sourceDir, targetFile, skillPath, roleContext string) error {
	window := fmt.Sprintf("agent-%s-%d", slugifyName(hire.entry.Name), time.Now().UnixNano())
	if err := createTmuxWindow(window, ctx.Config.ProjectDir); err != nil {
		return err
	}
	defer killTmuxWindow(window)
	prompt := fmt.Sprintf(
		"You are preparing %s (%s) for active duty. Load the create-agent-file skill at %s with identity_dir=%q, output_path=%q, role_context=%q. These identity materials live in %s—discover every Markdown file, honor their voice, and capture the sources you use. Write the AGENT.md file to %s and emit the skill's completion hook once it exists.",
		hire.entry.Name,
		hire.entry.Role,
		skillPath,
		sourceDir,
		targetFile,
		roleContext,
		sourceDir,
		targetFile,
	)
	if err := sendOpencodePrompt(window, prompt); err != nil {
		return err
	}
	return waitForFile(targetFile, opencodeSkillTimeout)
}

func stageHireSource(ctx *modes.ModeContext, hire hiredAgent) (string, error) {
	if hire.entry.Community == "" {
		return "", fmt.Errorf("hire %s is missing community metadata", hire.entry.Name)
	}
	if hire.source == "" {
		return "", fmt.Errorf("hire %s has no source directory", hire.entry.Name)
	}
	destDir := filepath.Join(ctx.Config.CVsDir(), hire.entry.Community, hire.entry.Name)
	if err := os.RemoveAll(destDir); err != nil {
		return "", fmt.Errorf("reset staged directory for %s: %w", hire.entry.Name, err)
	}
	if err := copyDir(hire.source, destDir); err != nil {
		return "", fmt.Errorf("stage source files for %s: %w", hire.entry.Name, err)
	}
	return destDir, nil
}

func writeSparkAgent(path, name, role string) error {
	content := fmt.Sprintf(`# %s

Role: %s

This is a SPARK placeholder agent created automatically during hiring. Flesh out this agent with a proper CV before assigning critical work.
`, name, role)
	return os.WriteFile(path, []byte(content), 0644)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
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
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func createTmuxWindow(name, dir string) error {
	cmd := exec.Command("tmux", "new-window", "-n", name, "-c", dir)
	return cmd.Run()
}

func killTmuxWindow(name string) {
	_ = exec.Command("tmux", "kill-window", "-t", name).Run()
}

func sendOpencodePrompt(window, prompt string) error {
	escaped := strings.ReplaceAll(prompt, "\"", `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", " ")
	cmd := exec.Command("tmux", "send-keys", "-t", window, fmt.Sprintf(`opencode --prompt "%s"`, escaped), "Enter")
	return cmd.Run()
}

func waitForFile(path string, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			return fmt.Errorf("timed out waiting for %s", path)
		case <-ticker.C:
			if info, err := os.Stat(path); err == nil && info.Size() > 0 {
				return nil
			}
		}
	}
}

func slugifyName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	replacer := strings.NewReplacer(" ", "-", "[", "", "]", "", "_", "-", "/", "-")
	result := replacer.Replace(lower)
	result = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(result, "")
	result = strings.Trim(result, "-")
	if result == "" {
		return "agent"
	}
	return result
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
