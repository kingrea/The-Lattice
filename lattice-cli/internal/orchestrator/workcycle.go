package orchestrator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	cycleMinStoryPoints     = 5
	maxAgentStoryPoints     = 8
	pluginAutoInstallEnv    = "LATTICE_PLUGIN_AUTO_INSTALL"
	pluginManualInstallHint = "Install it manually with opencode install opencode-worktree (requires npm) or run npm install -g opencode opencode-worktree and rerun lattice."
)

var ErrNoReadyBeads = errors.New("no ready beads available")

// ProjectAgent represents an agent that exists inside the project state directory.
type ProjectAgent struct {
	Name    string
	Role    string
	Summary string
	Path    string
	Memory  string
}

// Bead represents a single bead/task that can be assigned to an agent.
type Bead struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Points    int      `json:"points"`
	ParentID  string   `json:"parent"`
	Tags      []string `json:"tags"`
	Blocked   bool     `json:"blocked"`
	BlockedBy []string `json:"blockedBy"`
	DependsOn []string `json:"dependsOn"`
}

// WorktreeSession captures the state for a prepared worktree/agent session.
type WorktreeSession struct {
	Number    int
	Name      string
	Agent     ProjectAgent
	Beads     []Bead
	Path      string
	CreatedAt time.Time
}

// WorktreeStatus captures the status metadata rendered into WORKTREE.md.
type WorktreeStatus struct {
	Phase   string
	State   string
	Cycle   int
	Global  int
	Updated time.Time
}

// TotalPoints returns the sum of story points for all beads in the session.
func (s WorktreeSession) TotalPoints() int {
	total := 0
	for _, bead := range s.Beads {
		total += bead.Points
	}
	return total
}

// PrepareWorkCycle installs opencode-worktree, groups beads, and creates sessions.
func (o *Orchestrator) PrepareWorkCycle() ([]WorktreeSession, error) {
	if err := o.ensureWorktreeToolInstalled(); err != nil {
		return nil, err
	}
	cycleNumber, err := o.ensureCycleState()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cycle state: %w", err)
	}

	if sessions, err := o.loadTrackedSessions(); err == nil {
		return sessions, nil
	} else if err != nil && !errors.Is(err, errNoTrackedSessions) {
		return nil, err
	}

	scheduledAgents, err := o.selectScheduledAgents()
	if err != nil {
		return nil, err
	}
	if len(scheduledAgents) == 0 {
		return nil, fmt.Errorf("no agents available to schedule")
	}

	beads, err := o.loadReadyBeads()
	if err != nil {
		return nil, err
	}

	selected := selectBeadsForCycle(beads, scheduledAgents)
	if len(selected) == 0 {
		return nil, fmt.Errorf("no ready beads available for assignment")
	}

	assignments, err := assignBeadsToAgents(scheduledAgents, selected)
	if err != nil {
		return nil, err
	}

	sessions, err := o.createWorktreeSessions(assignments, cycleNumber)
	if err != nil {
		return nil, err
	}
	if err := o.persistCycleTracker(cycleNumber, sessions, "prepared"); err != nil {
		return nil, err
	}

	return sessions, nil
}

func (o *Orchestrator) ensureWorktreeToolInstalled() error {
	if o == nil || o.config == nil {
		return errors.New("orchestrator is not initialized")
	}
	if worktreePluginAvailable() {
		return nil
	}
	if !pluginAutoInstallEnabled() {
		return fmt.Errorf("opencode-worktree plugin is required but not installed. %s Enable automatic installation again by setting %s=1.", pluginManualInstallHint, pluginAutoInstallEnv)
	}
	if _, err := o.runProjectCommand("opencode", "install", "opencode-worktree"); err != nil {
		if worktreePluginAvailable() {
			return nil
		}
		errStr := strings.ToLower(err.Error())
		switch {
		case strings.Contains(errStr, `"opencode": executable file not found`):
			return fmt.Errorf("OpenCode CLI is not available on PATH, so Lattice cannot install opencode-worktree automatically. Install OpenCode with npm install -g opencode, then %s", pluginManualInstallHint)
		case pluginInstallPermissionError(errStr):
			return fmt.Errorf("Lattice does not have permission to install opencode-worktree automatically. %s Original error: %w", pluginManualInstallHint, err)
		case strings.Contains(errStr, "already installed") || strings.Contains(errStr, "exists"):
			return nil
		default:
			return fmt.Errorf("failed to install opencode-worktree automatically: %w. %s", err, pluginManualInstallHint)
		}
	}
	return nil
}

func worktreePluginAvailable() bool {
	if _, err := exec.LookPath("opencode-worktree"); err == nil {
		return true
	}
	return false
}

func pluginAutoInstallEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(pluginAutoInstallEnv)))
	switch value {
	case "", "1", "true", "yes", "on", "auto":
		return true
	}
	return false
}

func pluginInstallPermissionError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return strings.Contains(lower, "permission denied") || strings.Contains(lower, "eacces") || strings.Contains(lower, "operation not permitted")
}

func (o *Orchestrator) loadProjectAgents() ([]ProjectAgent, error) {
	agentsDir := o.config.AgentsDir()
	if _, err := os.Stat(agentsDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("expected agents under %s", agentsDir)
		}
		return nil, fmt.Errorf("failed to read agents directory: %w", err)
	}

	var agents []ProjectAgent
	walkErr := filepath.WalkDir(agentsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(d.Name(), "AGENT.md") {
			return nil
		}
		agent, err := parseProjectAgentFile(path)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", path, err)
		}
		agents = append(agents, agent)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	if len(agents) == 0 {
		return nil, fmt.Errorf("no agent files found in %s", agentsDir)
	}

	sort.SliceStable(agents, func(i, j int) bool {
		return strings.ToLower(agents[i].Name) < strings.ToLower(agents[j].Name)
	})

	return agents, nil
}

func parseProjectAgentFile(path string) (ProjectAgent, error) {
	filename := filepath.Base(path)
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	name := parseAgentNameFromFilename(base)
	data, err := os.ReadFile(path)
	if err != nil {
		return ProjectAgent{}, err
	}
	content := string(data)
	if docName := parseFrontMatterValue(content, "name"); docName != "" {
		name = docName
	}
	role := parseFrontMatterValue(content, "role")
	if role == "" {
		role = parseFrontMatterValue(content, "title")
	}
	summary := parseFirstParagraph(content)
	memoryPath := filepath.Join(filepath.Dir(path), "MEMORY.md")
	if info, err := os.Stat(memoryPath); err == nil && !info.IsDir() {
		return ProjectAgent{
			Name:    name,
			Role:    role,
			Summary: summary,
			Path:    path,
			Memory:  memoryPath,
		}, nil
	}
	return ProjectAgent{
		Name:    name,
		Role:    role,
		Summary: summary,
		Path:    path,
	}, nil
}

func parseAgentNameFromFilename(base string) string {
	trimmed := strings.TrimSpace(base)
	trimmed = strings.TrimPrefix(trimmed, "AGENT-")
	trimmed = strings.TrimPrefix(trimmed, "AGENTS-")
	trimmed = strings.TrimPrefix(trimmed, "agent-")
	trimmed = strings.TrimPrefix(trimmed, "agents-")
	trimmed = strings.ReplaceAll(trimmed, "_", " ")
	if trimmed == "" {
		return "Agent"
	}
	return trimmed
}

func parseFrontMatterValue(content, key string) string {
	needle := strings.ToLower(key) + ":"
	lines := strings.Split(content, "\n")
	inFrontMatter := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "---" {
			if !inFrontMatter {
				inFrontMatter = true
				continue
			}
			break
		}
		if !inFrontMatter {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, needle) {
			return strings.TrimSpace(line[len(needle):])
		}
	}
	return ""
}

func parseFirstParagraph(content string) string {
	lines := strings.Split(content, "\n")
	var buf []string
	inBody := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "---" && !inBody {
			inBody = true
			continue
		}
		if strings.HasPrefix(line, "#") {
			if len(buf) > 0 {
				break
			}
			continue
		}
		if line == "" {
			if len(buf) > 0 {
				break
			}
			continue
		}
		buf = append(buf, line)
	}
	return strings.Join(buf, " ")
}

func (o *Orchestrator) loadReadyBeads() ([]Bead, error) {
	output, err := o.runProjectCommand("bd", "ready", "--json")
	if err != nil {
		return nil, err
	}
	records, err := parseBeadRecords([]byte(output))
	if err != nil {
		return nil, err
	}
	beads := convertBeadRecords(records)
	if len(beads) == 0 {
		return nil, ErrNoReadyBeads
	}
	sort.SliceStable(beads, func(i, j int) bool {
		if beads[i].Points == beads[j].Points {
			return beads[i].ID < beads[j].ID
		}
		return beads[i].Points > beads[j].Points
	})
	return beads, nil
}

type beadRecord struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Status       string      `json:"status"`
	Points       json.Number `json:"points"`
	Estimate     json.Number `json:"estimate"`
	Size         json.Number `json:"size"`
	ParentID     string      `json:"parent"`
	Tags         []string    `json:"tags"`
	Blocked      bool        `json:"blocked"`
	BlockedBy    []string    `json:"blockedBy"`
	BlockedByAlt []string    `json:"blocked_by"`
	DependsOn    []string    `json:"dependsOn"`
	DependsOnAlt []string    `json:"depends_on"`
}

func parseBeadRecords(data []byte) ([]beadRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var arr []beadRecord
	if err := decoder.Decode(&arr); err == nil && len(arr) > 0 {
		return arr, nil
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var wrapper struct {
		Items []beadRecord `json:"items"`
	}
	if err := decoder.Decode(&wrapper); err == nil && len(wrapper.Items) > 0 {
		return wrapper.Items, nil
	}
	return nil, fmt.Errorf("unexpected bd ready output")
}

func convertBeadRecords(records []beadRecord) []Bead {
	beads := make([]Bead, 0, len(records))
	for _, rec := range records {
		id := strings.TrimSpace(rec.ID)
		if id == "" {
			continue
		}
		points := firstNonZeroNumber(rec.Points, rec.Estimate, rec.Size)
		if points <= 0 {
			points = 1
		}
		blockedBy := dedupeStrings(append(append([]string{}, rec.BlockedBy...), rec.BlockedByAlt...))
		dependsOn := dedupeStrings(append(append([]string{}, rec.DependsOn...), rec.DependsOnAlt...))
		blocked := rec.Blocked || len(blockedBy) > 0 || len(dependsOn) > 0 || containsBlockedTag(rec.Tags) || statusIndicatesBlock(rec.Status)
		beads = append(beads, Bead{
			ID:        id,
			Title:     strings.TrimSpace(rec.Title),
			Status:    rec.Status,
			Points:    points,
			ParentID:  strings.TrimSpace(rec.ParentID),
			Tags:      rec.Tags,
			Blocked:   blocked,
			BlockedBy: blockedBy,
			DependsOn: dependsOn,
		})
	}
	unblocked := make([]Bead, 0, len(beads))
	for _, bead := range beads {
		if bead.Blocked {
			continue
		}
		unblocked = append(unblocked, bead)
	}
	return unblocked
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func containsBlockedTag(tags []string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), "block") {
			return true
		}
	}
	return false
}

func statusIndicatesBlock(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return strings.Contains(s, "block")
}

func firstNonZeroNumber(numbers ...json.Number) int {
	for _, num := range numbers {
		if num == "" {
			continue
		}
		if val, err := num.Int64(); err == nil && val > 0 {
			return int(val)
		}
		if val, err := num.Float64(); err == nil && val > 0 {
			return int(val + 0.5)
		}
	}
	return 0
}

func selectBeadsForCycle(beads []Bead, agents []scheduledAgent) []Bead {
	if len(beads) == 0 || len(agents) == 0 {
		return nil
	}
	target := 0
	for _, agent := range agents {
		cap := agent.Capacity
		if cap <= 0 {
			cap = maxAgentStoryPoints
		}
		target += cap
	}
	if target < cycleMinStoryPoints {
		target = cycleMinStoryPoints
	}
	var selection []Bead
	for _, bead := range beads {
		selection = append(selection, bead)
		target -= bead.Points
		if target <= 0 {
			break
		}
	}
	return selection
}

type agentAssignment struct {
	Agent    ProjectAgent
	Beads    []Bead
	Points   int
	Capacity int
}

func assignBeadsToAgents(agents []scheduledAgent, beads []Bead) ([]agentAssignment, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("no agents available to assign work")
	}
	if len(beads) == 0 {
		return nil, fmt.Errorf("no beads to assign")
	}
	limit := len(agents)
	if limit > len(beads) {
		limit = len(beads)
	}
	if limit == 0 {
		limit = len(agents)
	}
	assignments := make([]*agentAssignment, 0, limit)
	for i := 0; i < limit; i++ {
		cap := agents[i].Capacity
		if cap <= 0 {
			cap = maxAgentStoryPoints
		}
		assignments = append(assignments, &agentAssignment{Agent: agents[i].Agent, Capacity: cap})
	}
	for _, bead := range beads {
		slot := pickAssignment(assignments)
		slot.Beads = append(slot.Beads, bead)
		slot.Points += bead.Points
	}
	var result []agentAssignment
	for _, slot := range assignments {
		if len(slot.Beads) == 0 {
			continue
		}
		result = append(result, *slot)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no assignments were created")
	}
	return result, nil
}

func pickAssignment(assignments []*agentAssignment) *agentAssignment {
	if len(assignments) == 0 {
		return nil
	}
	best := assignments[0]
	for _, slot := range assignments[1:] {
		if compareAssignmentLoad(slot, best) < 0 {
			best = slot
		}
	}
	return best
}

func compareAssignmentLoad(a, b *agentAssignment) int {
	loadA := loadRatio(a)
	loadB := loadRatio(b)
	switch {
	case loadA < loadB:
		return -1
	case loadA > loadB:
		return 1
	default:
		if len(a.Beads) < len(b.Beads) {
			return -1
		}
		if len(a.Beads) > len(b.Beads) {
			return 1
		}
		if a.Agent.Name < b.Agent.Name {
			return -1
		}
		if a.Agent.Name > b.Agent.Name {
			return 1
		}
	}
	return 0
}

func loadRatio(a *agentAssignment) float64 {
	cap := a.Capacity
	if cap <= 0 {
		cap = maxAgentStoryPoints
	}
	if cap == 0 {
		return 0
	}
	return float64(a.Points) / float64(cap)
}

func (o *Orchestrator) createWorktreeSessions(assignments []agentAssignment, cycleNumber int) ([]WorktreeSession, error) {
	if len(assignments) == 0 {
		return nil, fmt.Errorf("no assignments to materialize")
	}
	base := o.config.WorktreeDir()
	if err := os.MkdirAll(base, 0755); err != nil {
		return nil, fmt.Errorf("failed to prepare worktree directory: %w", err)
	}
	nextNumber, err := o.nextWorktreeNumber(base)
	if err != nil {
		return nil, err
	}
	var sessions []WorktreeSession
	for _, assignment := range assignments {
		number := nextNumber
		nextNumber++
		sessionRoot := filepath.Join(base, strconv.Itoa(number))
		if err := os.MkdirAll(sessionRoot, 0755); err != nil {
			return nil, fmt.Errorf("failed to create session directory: %w", err)
		}
		name := buildWorktreeName(number, assignment.Agent, assignment.Beads)
		sessionDir := filepath.Join(sessionRoot, name)
		if err := os.MkdirAll(sessionDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create branch session directory: %w", err)
		}
		folders := []string{
			filepath.Join(sessionDir, "archive"),
			filepath.Join(sessionDir, "archive", "events"),
			filepath.Join(sessionDir, "outbox"),
			filepath.Join(sessionDir, "outbox", "questions"),
			filepath.Join(sessionDir, "outbox", "events"),
			filepath.Join(sessionDir, "inbox"),
			filepath.Join(sessionDir, "inbox", "responses"),
		}
		for _, folder := range folders {
			if err := os.MkdirAll(folder, 0755); err != nil {
				return nil, fmt.Errorf("failed to create %s: %w", folder, err)
			}
		}
		if err := o.invokeWorktreeCreate(name); err != nil {
			return nil, err
		}
		session := WorktreeSession{
			Number:    number,
			Name:      name,
			Agent:     assignment.Agent,
			Beads:     assignment.Beads,
			Path:      sessionDir,
			CreatedAt: time.Now().UTC(),
		}
		status := WorktreeStatus{Phase: "pre-cycle", State: "pending", Cycle: 0, Global: cycleNumber, Updated: session.CreatedAt}
		if err := writeWorktreeState(session, status); err != nil {
			return nil, err
		}
		if err := writeWorktreeLog(session); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (o *Orchestrator) nextWorktreeNumber(base string) (int, error) {
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return 1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to scan worktree directory: %w", err)
	}
	maxNumber := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		num, err := strconv.Atoi(entry.Name())
		if err == nil && num > maxNumber {
			maxNumber = num
		}
	}
	return maxNumber + 1, nil
}

func (o *Orchestrator) invokeWorktreeCreate(name string) error {
	if _, err := o.runProjectCommand("opencode", "worktree_create", name); err == nil {
		return nil
	}
	if _, err := o.runProjectCommand("opencode-worktree", "worktree_create", name); err == nil {
		return nil
	}
	if _, err := o.runProjectCommand("worktree_create", name); err == nil {
		return nil
	}
	return fmt.Errorf("failed to create worktree %s", name)
}

func (o *Orchestrator) invokeWorktreeDelete(name, reason string) error {
	if reason == "" {
		reason = "cycle complete"
	}
	if _, err := o.runProjectCommand("opencode", "worktree_delete", name, reason); err == nil {
		return nil
	}
	if _, err := o.runProjectCommand("opencode-worktree", "worktree_delete", name, reason); err == nil {
		return nil
	}
	if _, err := o.runProjectCommand("worktree_delete", name, reason); err == nil {
		return nil
	}
	return fmt.Errorf("failed to delete worktree %s", name)
}

func writeWorktreeState(session WorktreeSession, status WorktreeStatus) error {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Worktree Session %d\n\n", session.Number))
	b.WriteString(fmt.Sprintf("- Agent: %s\n", session.Agent.Name))
	b.WriteString(fmt.Sprintf("- Worktree: %s\n", session.Name))
	updated := status.Updated
	if updated.IsZero() {
		updated = time.Now().UTC()
	}
	b.WriteString(fmt.Sprintf("- Created: %s\n", updated.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Total Points: %d\n", session.TotalPoints()))
	b.WriteString(fmt.Sprintf("- Beads: %d\n\n", len(session.Beads)))
	b.WriteString("## Assigned Beads\n")
	for _, bead := range session.Beads {
		b.WriteString(fmt.Sprintf("- %s · %s (%d pt)\n", bead.ID, bead.Title, bead.Points))
	}
	b.WriteString("\n## Status\n")
	phase := status.Phase
	if phase == "" {
		phase = "pre-cycle"
	}
	state := status.State
	if state == "" {
		state = "pending"
	}
	b.WriteString(fmt.Sprintf("- phase: %s\n", phase))
	b.WriteString(fmt.Sprintf("- state: %s\n", state))
	b.WriteString(fmt.Sprintf("- cycle: %d\n", status.Cycle))
	b.WriteString(fmt.Sprintf("- globalCycle: %d\n", status.Global))
	b.WriteString(fmt.Sprintf("- updated: %s\n", updated.Format(time.RFC3339)))

	b.WriteString("\n## Session Checklist\n")
	b.WriteString("- Keep WORKTREE.md as the source of truth for status\n")
	b.WriteString("- Track progress bead-by-bead and update status frequently\n")
	b.WriteString("- Record any handoffs or context changes\n")

	b.WriteString("\n# unrelated bugs\n")
	b.WriteString("- none recorded yet\n")

	b.WriteString("\n# need help\n")
	b.WriteString("- none recorded yet\n")
	statePath := filepath.Join(session.Path, "WORKTREE.md")
	return os.WriteFile(statePath, []byte(b.String()), 0644)
}

func writeWorktreeLog(session WorktreeSession) error {
	var b strings.Builder
	b.WriteString("# LOG\n\n")
	ids := strings.Join(beadIDs(session.Beads), ", ")
	b.WriteString(fmt.Sprintf("- %s · Session created for %s (%s) with beads: %s\n",
		session.CreatedAt.Format(time.RFC3339), session.Agent.Name, session.Name, ids))
	logPath := filepath.Join(session.Path, "LOG.md")
	return os.WriteFile(logPath, []byte(b.String()), 0644)
}

func appendWorktreeLog(session WorktreeSession, message string) error {
	logPath := filepath.Join(session.Path, "LOG.md")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	timestamp := time.Now().UTC().Format(time.RFC3339)
	if _, err := fmt.Fprintf(f, "- %s · %s\n", timestamp, message); err != nil {
		return err
	}
	return nil
}

func beadIDs(beads []Bead) []string {
	ids := make([]string, 0, len(beads))
	for _, bead := range beads {
		ids = append(ids, bead.ID)
	}
	return ids
}

func updateWorktreeStatusFile(session WorktreeSession, status WorktreeStatus) error {
	statePath := filepath.Join(session.Path, "WORKTREE.md")
	return updateStatusLines(statePath, status)
}

func updateStatusLines(path string, status WorktreeStatus) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	replacements := map[string]string{}
	phase := status.Phase
	if phase == "" {
		phase = "up-cycle"
	}
	state := status.State
	if state == "" {
		state = "running"
	}
	replacements["- phase:"] = fmt.Sprintf("- phase: %s", phase)
	replacements["- state:"] = fmt.Sprintf("- state: %s", state)
	replacements["- cycle:"] = fmt.Sprintf("- cycle: %d", status.Cycle)
	if status.Global > 0 {
		replacements["- globalCycle:"] = fmt.Sprintf("- globalCycle: %d", status.Global)
	}
	updated := status.Updated
	if updated.IsZero() {
		updated = time.Now().UTC()
	}
	replacements["- updated:"] = fmt.Sprintf("- updated: %s", updated.Format(time.RFC3339))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for key, repl := range replacements {
			if strings.HasPrefix(trimmed, key) {
				leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				lines[i] = leading + repl
				delete(replacements, key)
				break
			}
		}
		if len(replacements) == 0 {
			break
		}
	}
	result := strings.Join(lines, "\n")
	return os.WriteFile(path, []byte(result), 0644)
}

func readWorktreeStatus(path string) (WorktreeStatus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorktreeStatus{}, err
	}
	status := WorktreeStatus{}
	lines := strings.Split(string(data), "\n")
	inStatus := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		switch {
		case strings.EqualFold(line, "## Status"):
			inStatus = true
			continue
		case strings.HasPrefix(line, "## ") && inStatus:
			inStatus = false
			continue
		}
		if !inStatus || !strings.HasPrefix(line, "-") {
			continue
		}
		parse := func(prefix string) (string, bool) {
			if strings.HasPrefix(line, prefix) {
				return strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
			}
			return "", false
		}
		if value, ok := parse("- phase:"); ok {
			status.Phase = value
			continue
		}
		if value, ok := parse("- state:"); ok {
			status.State = value
			continue
		}
		if value, ok := parse("- cycle:"); ok {
			if n, err := strconv.Atoi(value); err == nil {
				status.Cycle = n
			}
			continue
		}
		if value, ok := parse("- globalCycle:"); ok {
			if n, err := strconv.Atoi(value); err == nil {
				status.Global = n
			}
			continue
		}
		if value, ok := parse("- updated:"); ok {
			if t, err := time.Parse(time.RFC3339, value); err == nil {
				status.Updated = t
			}
		}
	}
	if status.Phase == "" {
		status.Phase = "unknown"
	}
	if status.State == "" {
		status.State = "unknown"
	}
	return status, nil
}

func countPendingQuestions(sessionPath string) (int, int) {
	dir := filepath.Join(sessionPath, "outbox", "questions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	total := 0
	waiting := 0
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		total++
		questionPath := filepath.Join(dir, entry.Name())
		responsePath := responsePathForQuestion(sessionPath, questionPath)
		if _, err := os.Stat(responsePath); err != nil {
			if os.IsNotExist(err) {
				waiting++
			}
		}
	}
	return total, waiting
}

func buildWorktreeName(number int, agent ProjectAgent, beads []Bead) string {
	agentSlug := slugifyToken(agent.Name)
	var beadSlugs []string
	for _, bead := range beads {
		beadSlugs = append(beadSlugs, slugifyToken(bead.ID))
	}
	return fmt.Sprintf("tree-%d-%s-%s", number, agentSlug, strings.Join(beadSlugs, "-"))
}

func slugifyToken(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return "session"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		switch r {
		case ' ', '-', '_':
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "session"
	}
	return result
}

func (o *Orchestrator) runProjectCommand(name string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Dir = o.config.ProjectDir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s %s failed: %s", name, strings.Join(args, " "), errMsg)
	}
	return stdout.String(), nil
}
