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

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/modules/runtime"
	"github.com/yourusername/lattice/internal/orchestrator"
	"github.com/yourusername/lattice/internal/skills"
	"github.com/yourusername/lattice/internal/workflow"
)

const (
	moduleID      = "hiring"
	moduleVersion = "1.0.0"

	minWorkersRequired    = 10
	defaultSpecialists    = 2
	maxAgentStoryPoints   = 8
	specialistStoryPoints = 4
	sparkNameFormat       = "[spark-%02d]"
	workerRole            = "worker"
	specialistRole        = "specialist"
	opencodeSkillTimeout  = 5 * time.Minute
)

// Option customizes the hiring module.
type Option func(*HiringModule)

// CommandRunner overrides the external command executor (bd, tmux, etc.).
type CommandRunner func(dir, name string, args ...string) ([]byte, error)

// AgentBriefWriter overrides how AGENT.md files are produced for non-SPARK hires.
type AgentBriefWriter func(ctx *module.ModuleContext, entry workflow.WorkerEntry, stagedDir, targetFile, roleContext string) error

// HiringModule converts a consolidated plan into a staffed roster and agent dossiers.
type HiringModule struct {
	*module.Base
	now        func() time.Time
	runCmd     CommandRunner
	briefMaker AgentBriefWriter
}

// Register adds the module factory to the registry.
func Register(reg *module.Registry) {
	if reg == nil {
		return
	}
	reg.MustRegister(moduleID, func(module.Config) (module.Module, error) {
		return New(), nil
	})
}

// New creates a hiring module with default configuration.
func New(opts ...Option) *HiringModule {
	info := module.Info{
		ID:          moduleID,
		Name:        "Hire Workers",
		Description: "Builds the worker roster, generates AGENT briefs, and opens bd tickets for dossier creation.",
		Version:     moduleVersion,
	}
	base := module.NewBase(info)
	base.SetInputs(
		artifact.ModulesDoc,
		artifact.ActionPlanDoc,
		artifact.BeadsCreatedMarker,
		artifact.OrchestratorState,
	)
	base.SetOutputs(artifact.WorkersJSON)
	mod := &HiringModule{
		Base:       &base,
		now:        time.Now,
		runCmd:     defaultCommandRunner,
		briefMaker: defaultBriefWriter,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(mod)
		}
	}
	return mod
}

// WithClock overrides the module clock (used in metadata + timestamps).
func WithClock(clock func() time.Time) Option {
	return func(m *HiringModule) {
		if clock != nil {
			m.now = clock
		}
	}
}

// WithCommandRunner swaps the external command executor.
func WithCommandRunner(runner CommandRunner) Option {
	return func(m *HiringModule) {
		if runner != nil {
			m.runCmd = runner
		}
	}
}

// WithAgentBriefWriter swaps the AGENT.md authoring strategy for non-SPARK hires.
func WithAgentBriefWriter(writer AgentBriefWriter) Option {
	return func(m *HiringModule) {
		if writer != nil {
			m.briefMaker = writer
		}
	}
}

// Run staffs the commission and emits roster + agent artifacts.
func (m *HiringModule) Run(ctx *module.ModuleContext) (module.Result, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if ctx.Orchestrator == nil && ctx.Config != nil {
		ctx.Orchestrator = orchestrator.New(ctx.Config)
	}
	if ctx.Orchestrator == nil {
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("%s: orchestrator handle unavailable", moduleID)
	}
	if missing, err := m.missingInput(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	} else if missing != "" {
		return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("waiting for %s", missing)}, nil
	}
	if complete, err := m.IsComplete(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	} else if complete {
		return module.Result{Status: module.StatusNoOp, Message: "roster already hired"}, nil
	}
	totalPoints, beadCount, err := m.analyzeWorkload(ctx)
	if err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	baseWorkers := maxInt(minWorkersRequired, computeMaxParallel(totalPoints, beadCount))
	totalNeeded := baseWorkers + defaultSpecialists
	hires, err := m.selectAgents(ctx, baseWorkers, totalNeeded)
	if err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	analysis := hiringAnalysis{
		TotalPoints:     totalPoints,
		BeadCount:       beadCount,
		BaseWorkers:     baseWorkers,
		Specialists:     defaultSpecialists,
		TotalRequested:  totalNeeded,
		TotalHires:      len(hires),
		SparkCount:      countSparks(hires),
		ComputationMode: "max(points/maxSP, beadCount, minWorkers)",
	}
	if err := m.writeWorkerRoster(ctx, hires, analysis); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.generateAgentFiles(ctx, hires); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.createHireBeads(ctx, hires); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := ctx.Orchestrator.RefreshOpenCodeConfig(); err != nil {
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("%s: refresh opencode config: %w", moduleID, err)
	}
	return module.Result{Status: module.StatusCompleted, Message: fmt.Sprintf("hired %d denizens", len(hires))}, nil
}

// IsComplete reports true when workers.json carries hiring metadata.
func (m *HiringModule) IsComplete(ctx *module.ModuleContext) (bool, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return false, err
	}
	result, err := ctx.Artifacts.Check(artifact.WorkersJSON)
	if err != nil {
		return false, fmt.Errorf("%s: check workers.json: %w", moduleID, err)
	}
	if result.State != artifact.StateReady {
		return false, nil
	}
	if result.Metadata == nil || result.Metadata.ModuleID != moduleID || result.Metadata.Version != moduleVersion {
		return false, nil
	}
	return true, nil
}

func (m *HiringModule) missingInput(ctx *module.ModuleContext) (string, error) {
	for _, ref := range m.Inputs() {
		result, err := ctx.Artifacts.Check(ref)
		if err != nil {
			return "", fmt.Errorf("%s: check %s: %w", moduleID, ref.ID, err)
		}
		if result.State != artifact.StateReady {
			return ref.Name, nil
		}
	}
	return "", nil
}

func (m *HiringModule) analyzeWorkload(ctx *module.ModuleContext) (int, int, error) {
	out, err := m.runCmd(ctx.Config.ProjectDir, "bd", "ready", "--json")
	if err != nil {
		return 0, 0, fmt.Errorf("%s: bd ready --json failed: %s: %w", moduleID, strings.TrimSpace(string(out)), err)
	}
	points, count, parseErr := parseBeadStats(out)
	if parseErr != nil {
		return 0, 0, fmt.Errorf("%s: parse bd ready output: %w", moduleID, parseErr)
	}
	return points, count, nil
}

func (m *HiringModule) selectAgents(ctx *module.ModuleContext, workerCount, totalNeeded int) ([]rosterAssignment, error) {
	agents, err := ctx.Orchestrator.LoadDenizenCVs()
	if err != nil {
		return nil, fmt.Errorf("%s: load denizen cvs: %w", moduleID, err)
	}
	sort.SliceStable(agents, func(i, j int) bool {
		return strings.ToLower(strings.TrimSpace(agents[i].Name)) < strings.ToLower(strings.TrimSpace(agents[j].Name))
	})
	used := make(map[string]struct{})
	selected := make([]rosterAssignment, 0, totalNeeded)
	for _, agent := range agents {
		key := strings.ToLower(strings.TrimSpace(agent.Name))
		if key == "" {
			continue
		}
		if _, ok := used[key]; ok {
			continue
		}
		entry := workflow.WorkerEntry{Name: agent.Name, Community: agent.Community}
		sourceDir := filepath.Dir(agent.CVPath)
		selected = append(selected, rosterAssignment{Entry: entry, Source: sourceDir})
		used[key] = struct{}{}
		if len(selected) >= totalNeeded {
			break
		}
	}
	sparkCounter := 1
	for len(selected) < totalNeeded {
		name := fmt.Sprintf(sparkNameFormat, sparkCounter)
		sparkCounter++
		entry := workflow.WorkerEntry{Name: name, Community: "spark", IsSpark: true}
		selected = append(selected, rosterAssignment{Entry: entry})
	}
	for i := range selected {
		if i < workerCount {
			selected[i].Entry.Role = workerRole
			selected[i].Entry.Capacity = maxAgentStoryPoints
		} else {
			selected[i].Entry.Role = specialistRole
			selected[i].Entry.Capacity = specialistStoryPoints
		}
	}
	return selected, nil
}

func (m *HiringModule) writeWorkerRoster(ctx *module.ModuleContext, hires []rosterAssignment, analysis hiringAnalysis) error {
	entries := make([]workflow.WorkerEntry, 0, len(hires))
	specialists := make([]workflow.WorkerEntry, 0, len(hires))
	for _, hire := range hires {
		entries = append(entries, hire.Entry)
		if hire.Entry.Role == specialistRole {
			specialists = append(specialists, hire.Entry)
		}
	}
	orch, err := m.loadOrchestratorRef(ctx)
	if err != nil {
		return err
	}
	payload := workerRosterPayload{
		Orchestrator: orch,
		Workers:      entries,
		Specialists:  specialists,
		UpdatedAt:    m.now().UTC().Format(time.RFC3339),
		Analysis:     analysis,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%s: encode workers.json: %w", moduleID, err)
	}
	meta := artifact.Metadata{
		ArtifactID: artifact.WorkersJSON.ID,
		ModuleID:   moduleID,
		Version:    moduleVersion,
		Workflow:   ctx.Workflow.Dir(),
		Inputs:     inputIDs(m.Inputs()),
	}
	if err := ctx.Artifacts.Write(artifact.WorkersJSON, body, meta); err != nil {
		return fmt.Errorf("%s: write workers.json: %w", moduleID, err)
	}
	return nil
}

func (m *HiringModule) generateAgentFiles(ctx *module.ModuleContext, hires []rosterAssignment) error {
	baseDir := ctx.Config.AgentsDir()
	for _, hire := range hires {
		roleDir := "workers"
		roleContext := workerRole
		if hire.Entry.Role == specialistRole {
			roleDir = "specialists"
			roleContext = specialistRole
		}
		slug := slugifyName(hire.Entry.Name)
		targetDir := filepath.Join(baseDir, roleDir, slug)
		if err := os.RemoveAll(targetDir); err != nil {
			return fmt.Errorf("%s: reset %s: %w", moduleID, targetDir, err)
		}
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return fmt.Errorf("%s: mkdir %s: %w", moduleID, targetDir, err)
		}
		agentPath := filepath.Join(targetDir, "AGENT.md")
		supportPath := filepath.Join(targetDir, "AGENT_SUP.md")
		if hire.Entry.IsSpark {
			if err := writeSparkAgent(agentPath, hire.Entry.Name, hire.Entry.Role); err != nil {
				return err
			}
			if err := writeSupportPacket(ctx, hire.Entry, agentPath, supportPath); err != nil {
				return err
			}
			continue
		}
		stagedDir, err := stageHireSource(ctx, hire)
		if err != nil {
			return err
		}
		if err := m.briefMaker(ctx, hire.Entry, stagedDir, agentPath, roleContext); err != nil {
			return fmt.Errorf("%s: generate agent file for %s: %w", moduleID, hire.Entry.Name, err)
		}
		if err := writeSupportPacket(ctx, hire.Entry, agentPath, supportPath); err != nil {
			return err
		}
	}
	return nil
}

func (m *HiringModule) loadOrchestratorRef(ctx *module.ModuleContext) (rosterAgent, error) {
	path := artifact.OrchestratorState.Path(ctx.Workflow)
	data, err := os.ReadFile(path)
	if err != nil {
		return rosterAgent{}, fmt.Errorf("%s: read orchestrator state: %w", moduleID, err)
	}
	var payload orchestratorStatePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return rosterAgent{}, fmt.Errorf("%s: parse orchestrator state: %w", moduleID, err)
	}
	return rosterAgent{Name: payload.Name, Community: payload.Community, CVPath: payload.CVPath}, nil
}

func stageHireSource(ctx *module.ModuleContext, hire rosterAssignment) (string, error) {
	if hire.Entry.Community == "" {
		return "", fmt.Errorf("%s: hire %s missing community", moduleID, hire.Entry.Name)
	}
	if strings.TrimSpace(hire.Source) == "" {
		return "", fmt.Errorf("%s: hire %s missing source directory", moduleID, hire.Entry.Name)
	}
	destDir := filepath.Join(ctx.Config.CVsDir(), hire.Entry.Community, hire.Entry.Name)
	if err := os.RemoveAll(destDir); err != nil {
		return "", fmt.Errorf("%s: reset staged dir for %s: %w", moduleID, hire.Entry.Name, err)
	}
	if err := copyDir(hire.Source, destDir); err != nil {
		return "", fmt.Errorf("%s: stage source files for %s: %w", moduleID, hire.Entry.Name, err)
	}
	return destDir, nil
}

func writeSparkAgent(path, name, role string) error {
	content := fmt.Sprintf(`# %s

Role: %s

This is a SPARK placeholder agent created automatically during hiring. Flesh out this agent with a proper CV before assigning critical work.
`, name, role)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("%s: write spark agent %s: %w", moduleID, name, err)
	}
	return nil
}

func writeSupportPacket(ctx *module.ModuleContext, entry workflow.WorkerEntry, agentPath, supportPath string) error {
	relAgent := relativeToProject(ctx, agentPath)
	relPlan := relativeToProject(ctx, ctx.Workflow.ActionPlanPath())
	var b strings.Builder
	fmt.Fprintf(&b, "# Support Packet: %s\n\n", entry.Name)
	fmt.Fprintf(&b, "- Role: %s\n", entry.Role)
	if entry.Community != "" {
		fmt.Fprintf(&b, "- Community: %s\n", entry.Community)
	}
	if entry.Capacity > 0 {
		fmt.Fprintf(&b, "- Capacity: %d story points\n", entry.Capacity)
	}
	if relAgent != "" {
		fmt.Fprintf(&b, "- Agent Brief: %s\n", relAgent)
	}
	if relPlan != "" {
		fmt.Fprintf(&b, "- Action Plan: %s\n", relPlan)
	}
	b.WriteString("\n## Notes\n")
	if entry.IsSpark {
		b.WriteString("This SPARK placeholder needs a full identity. Create a CV, regenerate AGENT.md, and refresh AGENT_SUP.md once ready.\n")
	} else {
		b.WriteString("Review the AGENT.md brief before assigning beads. Update this packet with task-specific guardrails so supervisors can track progress.\n")
	}
	return os.WriteFile(supportPath, []byte(b.String()), 0o644)
}

func relativeToProject(ctx *module.ModuleContext, target string) string {
	rel, err := filepath.Rel(ctx.Config.ProjectDir, target)
	if err != nil {
		return target
	}
	return fmt.Sprintf("{file:%s}", filepath.ToSlash(rel))
}

func (m *HiringModule) createHireBeads(ctx *module.ModuleContext, hires []rosterAssignment) error {
	if len(hires) == 0 {
		return nil
	}
	epArgs := []string{"-t", "epic", "-p", "1"}
	epID, err := m.runBdCreate(ctx, epArgs, "HIRE")
	if err != nil {
		return err
	}
	for _, hire := range hires {
		title := fmt.Sprintf("Create agent file for %s (%s)", hire.Entry.Name, hire.Entry.Role)
		args := []string{"--parent", epID, "-t", "task", "-p", "2"}
		if _, err := m.runBdCreate(ctx, args, title); err != nil {
			return err
		}
	}
	return nil
}

func (m *HiringModule) runBdCreate(ctx *module.ModuleContext, extra []string, title string) (string, error) {
	args := append([]string{"create", title}, extra...)
	args = append(args, "--json")
	out, err := m.runCmd(ctx.Config.ProjectDir, "bd", args...)
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		return "", fmt.Errorf("%s: bd create %q failed: %s: %w", moduleID, title, trimmed, err)
	}
	var resp struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(out, &resp) == nil && strings.TrimSpace(resp.ID) != "" {
		return strings.TrimSpace(resp.ID), nil
	}
	if id := extractBeadID(trimmed); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("%s: unable to parse bead id from %s", moduleID, trimmed)
}

func parseBeadStats(data []byte) (int, int, error) {
	type beadRecord struct {
		ID       string          `json:"id"`
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

func countSparks(hires []rosterAssignment) int {
	count := 0
	for _, hire := range hires {
		if hire.Entry.IsSpark {
			count++
		}
	}
	return count
}

type rosterAssignment struct {
	Entry  workflow.WorkerEntry
	Source string
}

type hiringAnalysis struct {
	TotalPoints     int    `json:"totalPoints"`
	BeadCount       int    `json:"beadCount"`
	BaseWorkers     int    `json:"baseWorkers"`
	Specialists     int    `json:"specialists"`
	TotalRequested  int    `json:"totalRequested"`
	TotalHires      int    `json:"totalHires"`
	SparkCount      int    `json:"sparkCount"`
	ComputationMode string `json:"computationMode"`
}

type workerRosterPayload struct {
	Orchestrator rosterAgent            `json:"orchestrator"`
	Workers      []workflow.WorkerEntry `json:"workers"`
	Specialists  []workflow.WorkerEntry `json:"specialists"`
	UpdatedAt    string                 `json:"updatedAt"`
	Analysis     hiringAnalysis         `json:"analysis"`
}

type rosterAgent struct {
	Name      string `json:"name"`
	Community string `json:"community,omitempty"`
	CVPath    string `json:"cvPath,omitempty"`
}

type orchestratorStatePayload struct {
	Name      string `json:"name"`
	Community string `json:"community"`
	CVPath    string `json:"cvPath"`
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

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
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

func defaultCommandRunner(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	return buf.Bytes(), cmd.Run()
}

func defaultBriefWriter(ctx *module.ModuleContext, entry workflow.WorkerEntry, stagedDir, targetFile, roleContext string) error {
	skillPath, err := skills.Ensure(ctx.Config.SkillsDir(), skills.CreateAgentFile)
	if err != nil {
		return err
	}
	return runCreateAgentFileSkill(ctx.Config.ProjectDir, entry, stagedDir, targetFile, skillPath, roleContext)
}

func runCreateAgentFileSkill(projectDir string, entry workflow.WorkerEntry, sourceDir, targetFile, skillPath, roleContext string) error {
	window := fmt.Sprintf("agent-%s-%d", slugifyName(entry.Name), time.Now().UnixNano())
	if err := createTmuxWindow(window, projectDir); err != nil {
		return err
	}
	defer killTmuxWindow(window)
	prompt := fmt.Sprintf(
		"You are preparing %s (%s) for active duty. Load the create-agent-file skill at %s with identity_dir=%q, output_path=%q, role_context=%q. These identity materials live in %s—discover every Markdown file, honor their voice, and capture the sources you use. Write the AGENT.md file to %s and emit the skill's completion hook once it exists.",
		entry.Name,
		roleContext,
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

func createTmuxWindow(name, dir string) error {
	args := []string{"new-window", "-n", name}
	if strings.TrimSpace(dir) != "" {
		args = append(args, "-c", dir)
	}
	cmd := exec.Command("tmux", args...)
	return cmd.Run()
}

func killTmuxWindow(name string) {
	if name == "" {
		return
	}
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

func inputIDs(refs []artifact.ArtifactRef) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
		}
	}
	return ids
}

func extractBeadID(output string) string {
	re := regexp.MustCompile(`\[(?P<id>[^\]]+)\]`)
	match := re.FindStringSubmatch(output)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
