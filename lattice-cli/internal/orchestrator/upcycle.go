package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yourusername/lattice/internal/skills"
	"github.com/yourusername/lattice/internal/workflow"
)

// UpCycleConfig tunes the orchestration of active work sessions.
type UpCycleConfig struct {
	IdleTimeout          time.Duration
	QuestionPollInterval time.Duration
	EventPollInterval    time.Duration
	ResponseTimeout      time.Duration
	OrchestratorTimeout  time.Duration
}

var defaultUpCycleConfig = UpCycleConfig{
	IdleTimeout:          30 * time.Second,
	QuestionPollInterval: 5 * time.Second,
	EventPollInterval:    4 * time.Second,
	ResponseTimeout:      2 * time.Minute,
	OrchestratorTimeout:  5 * time.Minute,
}

// RunUpCycle launches the assigned agents and manages their sessions until completion.
func (o *Orchestrator) RunUpCycle(ctx context.Context, sessions []WorktreeSession) error {
	if len(sessions) == 0 {
		return fmt.Errorf("no worktree sessions to run")
	}
	cycleNumber, err := o.currentCycleNumber()
	if err != nil {
		return err
	}
	if err := o.updateCycleTrackerStatus("running"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	mgr := &upCycleManager{
		orchestrator: o,
		sessions:     make([]*cycleSession, 0, len(sessions)),
		config:       defaultUpCycleConfig,
		cycleNumber:  cycleNumber,
	}
	for _, session := range sessions {
		cs := &cycleSession{
			WorktreeSession: session,
			cycle:           1,
			questionSeen:    make(map[string]struct{}),
			eventSeen:       make(map[string]struct{}),
			allBeads:        make(map[string]Bead),
		}
		for _, bead := range session.Beads {
			cs.allBeads[canonicalBeadKey(bead.ID)] = bead
		}
		cs.rebuildBeadIndex()
		mgr.sessions = append(mgr.sessions, cs)
	}
	if err := mgr.run(ctx); err != nil {
		return err
	}
	return mgr.runDownCycle(ctx)
}

type upCycleManager struct {
	orchestrator *Orchestrator
	config       UpCycleConfig
	sessions     []*cycleSession
	cycleNumber  int
	cycleSummary string
}

type sessionReport struct {
	Agent      string
	Worktree   string
	FinalCycle int
	Cycles     []cycleReport
}

type dreamRequest struct {
	agent       ProjectAgent
	summaryPath string
}

type cycleReport struct {
	Number    int
	Message   string
	Completed []string
	Remaining []string
}

type cycleSession struct {
	WorktreeSession
	cycle        int
	questionSeen map[string]struct{}
	eventSeen    map[string]struct{}
	questionCtx  context.Context
	questionStop context.CancelFunc
	agentWindow  string
	beadsByID    map[string]Bead
	allBeads     map[string]Bead
}

func (cs *cycleSession) rebuildBeadIndex() {
	cs.beadsByID = make(map[string]Bead)
	for _, bead := range cs.Beads {
		cs.beadsByID[canonicalBeadKey(bead.ID)] = bead
	}
}

func (cs *cycleSession) describeBeadList(ids []string) []string {
	var result []string
	for _, id := range ids {
		label := cs.beadLabel(id)
		if label != "" {
			result = append(result, label)
		}
	}
	return result
}

func (cs *cycleSession) beadLabel(id string) string {
	key := canonicalBeadKey(id)
	if bead, ok := cs.allBeads[key]; ok {
		return fmt.Sprintf("%s · %s", bead.ID, bead.Title)
	}
	return strings.TrimSpace(id)
}

func (cs *cycleSession) stopQuestionWatcher() {
	if cs.questionStop != nil {
		cs.questionStop()
		cs.questionStop = nil
		cs.questionCtx = nil
	}
}

func (m *upCycleManager) run(ctx context.Context) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(m.sessions))
	for _, cs := range m.sessions {
		wg.Add(1)
		go func(session *cycleSession) {
			defer wg.Done()
			if err := m.runSession(ctx, session); err != nil {
				errCh <- err
			}
		}(cs)
	}
	wg.Wait()
	close(errCh)
	var errs []error
	for err := range errCh {
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (m *upCycleManager) runDownCycle(ctx context.Context) error {
	if err := m.runAgentSummaries(ctx); err != nil {
		return err
	}
	reports, err := m.collectSessionReports()
	if err != nil {
		return err
	}
	if err := m.runOrchestratorSummary(ctx); err != nil {
		return err
	}
	if err := m.runLocalDreaming(ctx); err != nil {
		return err
	}
	if err := m.landWorktrees(ctx); err != nil {
		return err
	}
	if err := m.writeDownCycleLog(reports); err != nil {
		return err
	}
	if err := m.destroyWorktrees(); err != nil {
		return err
	}
	return m.finalizeCycle()
}

func (m *upCycleManager) runAgentSummaries(ctx context.Context) error {
	skillPath, err := skills.Ensure(m.orchestrator.config.SkillsDir(), skills.DownCycleAgent)
	if err != nil {
		return err
	}
	for _, cs := range m.sessions {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		summaryPath := filepath.Join(cs.Path, "SUMMARY.md")
		window := fmt.Sprintf("summary-%d-%d", cs.Number, time.Now().UnixNano())
		if err := m.orchestrator.createTmuxWindowInDir(window, cs.Path); err != nil {
			return err
		}
		prompt := fmt.Sprintf(
			"Cycle %d has completed for %s. Load the skill at %s and execute it inside %s. Write the final session summary to %s. Do not exit until SUMMARY.md exists and captures all work outcomes, reflections, and repo memory",
			m.cycleNumber,
			cs.Agent.Name,
			skillPath,
			cs.Path,
			summaryPath,
		)
		if err := m.orchestrator.runOpenCode(prompt, window, cs.Agent.Name); err != nil {
			m.orchestrator.killTmuxWindow(window)
			return err
		}
		if err := m.orchestrator.waitForFile(summaryPath, 5*time.Minute); err != nil {
			m.orchestrator.killTmuxWindow(window)
			return err
		}
		_ = m.orchestrator.killTmuxWindow(window)
	}
	return nil
}

func (m *upCycleManager) collectSessionReports() ([]sessionReport, error) {
	reports := make([]sessionReport, 0, len(m.sessions))
	for _, cs := range m.sessions {
		report, err := m.buildSessionReport(cs)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
		status := WorktreeStatus{Phase: "down-cycle", State: "archived", Cycle: report.FinalCycle, Global: m.cycleNumber, Updated: time.Now().UTC()}
		_ = updateWorktreeStatusFile(cs.WorktreeSession, status)
	}
	return reports, nil
}

func (m *upCycleManager) runOrchestratorSummary(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	skillPath, err := skills.Ensure(m.orchestrator.config.SkillsDir(), skills.DownCycle)
	if err != nil {
		return err
	}
	window := fmt.Sprintf("down-cycle-%d", time.Now().UnixNano())
	if err := m.orchestrator.createTmuxWindow(window); err != nil {
		return err
	}
	defer m.orchestrator.killTmuxWindow(window)
	cycleDir := filepath.Join(m.orchestrator.config.LatticeProjectDir, "state", fmt.Sprintf("cycle-%d", m.cycleNumber))
	if err := os.MkdirAll(cycleDir, 0755); err != nil {
		return err
	}
	cycleSummary := filepath.Join(cycleDir, "SUMMARY.md")
	m.cycleSummary = cycleSummary
	summaryGlob := filepath.Join(m.orchestrator.config.LatticeProjectDir, "worktree", "*", "*", "SUMMARY.md")
	planPath := filepath.Join(m.orchestrator.config.LatticeProjectDir, "workflow", "action", "PLAN.md")
	repoMemory := filepath.Join(m.orchestrator.config.LatticeProjectDir, "state", "REPO_MEMORY.md")
	prompt := fmt.Sprintf(
		"All worktrees have produced SUMMARY.md files. Load the orchestrator skill at %s and execute it now. Read every summary matching %s, update %s to reflect actual bead status, update repo memory at %s, and write the cycle summary to %s for cycle %d. Assign special agents for stuck work, create beads for new bugs, and ensure repo learnings are captured. Do not finish until the cycle summary file exists and PLAN.md plus REPO_MEMORY.md are updated accordingly.",
		skillPath,
		summaryGlob,
		planPath,
		repoMemory,
		cycleSummary,
		m.cycleNumber,
	)
	if err := m.orchestrator.runOpenCode(prompt, window, ""); err != nil {
		return err
	}
	return m.orchestrator.waitForFile(cycleSummary, m.config.OrchestratorTimeout)
}

func (m *upCycleManager) runLocalDreaming(ctx context.Context) error {
	skillPath, err := skills.Ensure(m.orchestrator.config.SkillsDir(), skills.LocalDreaming)
	if err != nil {
		return err
	}
	requests, err := m.buildDreamRequests()
	if err != nil {
		return err
	}
	for _, req := range requests {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		agentDir := filepath.Dir(req.agent.Path)
		if err := os.MkdirAll(agentDir, 0755); err != nil {
			return err
		}
		memoryPath := filepath.Join(agentDir, "MEMORY.md")
		window := fmt.Sprintf("dream-%s-%d", slugifyToken(req.agent.Name), time.Now().UnixNano())
		if err := m.orchestrator.createTmuxWindow(window); err != nil {
			return err
		}
		prompt := fmt.Sprintf(
			"Cycle %d reflections are ready. Load the local-dreaming skill at %s. Use %s as the session summary for %s and append a memory entry to %s. Keep it personal—focus on how the work affected the agent. Do not finish until the memory file exists and includes a Cycle %d entry.",
			m.cycleNumber,
			skillPath,
			req.summaryPath,
			req.agent.Name,
			memoryPath,
			m.cycleNumber,
		)
		if err := m.orchestrator.runOpenCode(prompt, window, req.agent.Name); err != nil {
			m.orchestrator.killTmuxWindow(window)
			return err
		}
		if err := m.orchestrator.waitForFile(memoryPath, m.config.ResponseTimeout); err != nil {
			m.orchestrator.killTmuxWindow(window)
			return err
		}
		_ = m.orchestrator.killTmuxWindow(window)
	}
	return nil
}

func (m *upCycleManager) buildDreamRequests() ([]dreamRequest, error) {
	requests := make([]dreamRequest, 0, len(m.sessions)+1)
	seen := make(map[string]struct{})
	for _, cs := range m.sessions {
		key := strings.ToLower(strings.TrimSpace(cs.Agent.Name))
		if _, ok := seen[key]; ok {
			continue
		}
		requests = append(requests, dreamRequest{
			agent:       cs.Agent,
			summaryPath: filepath.Join(cs.Path, "SUMMARY.md"),
		})
		seen[key] = struct{}{}
	}
	orchAgent, err := m.findOrchestratorAgent()
	if err != nil {
		return nil, err
	}
	if orchAgent != nil && m.cycleSummary != "" {
		requests = append(requests, dreamRequest{
			agent:       *orchAgent,
			summaryPath: m.cycleSummary,
		})
	}
	return requests, nil
}

func (m *upCycleManager) findOrchestratorAgent() (*ProjectAgent, error) {
	workerList := m.orchestrator.loadWorkerList(m.orchestrator.config.WorkerListPath())
	if workerList.Orchestrator == nil {
		return nil, nil
	}
	name := strings.ToLower(strings.TrimSpace(workerList.Orchestrator.Name))
	agents, err := m.orchestrator.loadProjectAgents()
	if err != nil {
		return nil, err
	}
	for _, agent := range agents {
		if strings.ToLower(strings.TrimSpace(agent.Name)) == name {
			return &agent, nil
		}
	}
	return nil, fmt.Errorf("orchestrator agent %s not found", workerList.Orchestrator.Name)
}

func ensureGitClean(dir string) error {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git status failed in %s: %w", dir, err)
	}
	if strings.TrimSpace(string(output)) != "" {
		return fmt.Errorf("worktree %s still has pending changes after landing", dir)
	}
	return nil
}

func (m *upCycleManager) landWorktrees(ctx context.Context) error {
	manualPath := filepath.Join(m.orchestrator.config.ProjectDir, "AGENTS.md")
	for _, cs := range m.sessions {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		window := fmt.Sprintf("land-%d-%d", cs.Number, time.Now().UnixNano())
		if err := m.orchestrator.createTmuxWindowInDir(window, cs.Path); err != nil {
			return err
		}
		prompt := fmt.Sprintf(
			"Cycle %d completed. Follow the landing instructions in %s for this worktree. Ensure all changes (including SUMMARY.md and MEMORY.md updates) are committed, tests run, git pull --rebase + bd sync executed, and git push succeeds. Do not finish until `git status --porcelain` is empty.",
			m.cycleNumber,
			manualPath,
		)
		if err := m.orchestrator.runOpenCode(prompt, window, ""); err != nil {
			m.orchestrator.killTmuxWindow(window)
			return err
		}
		_ = m.orchestrator.killTmuxWindow(window)
		if err := ensureGitClean(cs.Path); err != nil {
			return err
		}
	}
	return nil
}

func (m *upCycleManager) destroyWorktrees() error {
	parents := make(map[string]struct{})
	for _, cs := range m.sessions {
		if err := m.orchestrator.invokeWorktreeDelete(cs.Name, "cycle complete"); err != nil {
			return err
		}
		parent := filepath.Dir(cs.Path)
		if _, ok := parents[parent]; !ok {
			parents[parent] = struct{}{}
		}
		if err := os.RemoveAll(cs.Path); err != nil {
			return err
		}
	}
	for parent := range parents {
		_ = os.RemoveAll(parent)
	}
	return nil
}

func (m *upCycleManager) finalizeCycle() error {
	nextCycle, err := m.orchestrator.incrementCycleNumber()
	if err != nil {
		return err
	}
	worktreeBase := m.orchestrator.config.WorktreeDir()
	if err := os.RemoveAll(worktreeBase); err != nil {
		return err
	}
	if err := m.orchestrator.clearCycleTracker(); err != nil {
		return err
	}
	return m.orchestrator.restartInitialPromptWithCycle(nextCycle)
}

func (m *upCycleManager) buildSessionReport(cs *cycleSession) (sessionReport, error) {
	report := sessionReport{
		Agent:    cs.Agent.Name,
		Worktree: cs.Name,
	}
	dir := filepath.Join(cs.Path, "archive", "events")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return report, nil
		}
		return sessionReport{}, fmt.Errorf("session %s: read archive: %w", cs.Name, err)
	}
	type cycleFile struct {
		path  string
		cycle int
	}
	var files []cycleFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "agent-cycle-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		num := strings.TrimSuffix(strings.TrimPrefix(name, "agent-cycle-"), ".json")
		cycle, err := strconv.Atoi(num)
		if err != nil {
			continue
		}
		files = append(files, cycleFile{path: filepath.Join(dir, name), cycle: cycle})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].cycle < files[j].cycle })
	for _, file := range files {
		evt, err := readWorktreeEvent(file.path)
		if err != nil {
			_ = appendWorktreeLog(cs.WorktreeSession, fmt.Sprintf("Failed to read %s: %v", filepath.Base(file.path), err))
			continue
		}
		report.Cycles = append(report.Cycles, cycleReport{
			Number:    evt.Cycle,
			Message:   strings.TrimSpace(evt.Message),
			Completed: cs.describeBeadList(evt.CompletedBeads),
			Remaining: cs.describeBeadList(evt.RemainingBeads),
		})
	}
	if len(report.Cycles) > 0 {
		report.FinalCycle = report.Cycles[len(report.Cycles)-1].Number
	}
	return report, nil
}

func (m *upCycleManager) writeDownCycleLog(reports []sessionReport) error {
	if len(reports) == 0 {
		return nil
	}
	workDir := filepath.Join(m.orchestrator.config.LatticeProjectDir, workflow.WorkflowDir, workflow.WorkDir)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return err
	}
	logPath := filepath.Join(workDir, workflow.FileWorkLog)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	timestamp := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(f, "\n## Down cycle summary (%s)\n\n", timestamp)
	for _, report := range reports {
		fmt.Fprintf(f, "### %s — %s\n", report.Worktree, report.Agent)
		fmt.Fprintf(f, "- cycles run: %d\n", len(report.Cycles))
		if len(report.Cycles) == 0 {
			fmt.Fprintln(f, "- no agent cycle data recorded")
			continue
		}
		for _, cycle := range report.Cycles {
			fmt.Fprintf(f, "  - cycle %d\n", cycle.Number)
			if len(cycle.Completed) > 0 {
				fmt.Fprintf(f, "    - completed: %s\n", strings.Join(cycle.Completed, "; "))
			}
			if len(cycle.Remaining) > 0 {
				fmt.Fprintf(f, "    - remaining: %s\n", strings.Join(cycle.Remaining, "; "))
			}
			if cycle.Message != "" {
				fmt.Fprintf(f, "    - notes: %s\n", cycle.Message)
			}
		}
		fmt.Fprintln(f)
	}
	return nil
}

func (m *upCycleManager) runSession(ctx context.Context, cs *cycleSession) error {
	defer cs.stopQuestionWatcher()
	for {
		if err := m.startAgentCycle(ctx, cs); err != nil {
			return err
		}
		agentEvent, err := m.waitForAgentEvent(ctx, cs)
		if err != nil {
			return err
		}
		if err := m.runPostCycleOrchestrator(ctx, cs, agentEvent); err != nil {
			return err
		}
		remaining := m.filterRemainingBeads(cs, agentEvent.RemainingBeads)
		cs.Beads = remaining
		cs.WorktreeSession.Beads = remaining
		cs.rebuildBeadIndex()
		if len(remaining) == 0 {
			status := WorktreeStatus{Phase: "up-cycle", State: "complete", Cycle: cs.cycle, Global: m.cycleNumber, Updated: time.Now().UTC()}
			_ = updateWorktreeStatusFile(cs.WorktreeSession, status)
			_ = appendWorktreeLog(cs.WorktreeSession, fmt.Sprintf("Cycle %d complete for %s", cs.cycle, cs.Agent.Name))
			return nil
		}
		cs.cycle++
	}
}

func (m *upCycleManager) startAgentCycle(ctx context.Context, cs *cycleSession) error {
	status := WorktreeStatus{Phase: "up-cycle", State: "running", Cycle: cs.cycle, Global: m.cycleNumber, Updated: time.Now().UTC()}
	if err := updateWorktreeStatusFile(cs.WorktreeSession, status); err != nil {
		return fmt.Errorf("session %s cycle %d: update status: %w", cs.Name, cs.cycle, err)
	}
	if cs.questionCtx == nil {
		qCtx, cancel := context.WithCancel(ctx)
		cs.questionCtx = qCtx
		cs.questionStop = cancel
		go m.watchQuestions(qCtx, cs)
	}
	finalSkillPath, err := skills.Ensure(m.orchestrator.config.SkillsDir(), skills.FinalSession)
	if err != nil {
		return err
	}
	window := fmt.Sprintf("worktree-agent-%d-%d", cs.Number, cs.cycle)
	if err := m.orchestrator.createTmuxWindowInDir(window, cs.Path); err != nil {
		return fmt.Errorf("session %s: tmux window: %w", cs.Name, err)
	}
	cs.agentWindow = window
	prompt := m.buildAgentPrompt(cs, finalSkillPath)
	if err := m.orchestrator.runOpenCode(prompt, window, cs.Agent.Name); err != nil {
		_ = m.orchestrator.killTmuxWindow(window)
		return fmt.Errorf("session %s: failed to launch agent: %w", cs.Name, err)
	}
	_ = appendWorktreeLog(cs.WorktreeSession, fmt.Sprintf("Cycle %d dispatched to %s", cs.cycle, cs.Agent.Name))
	return nil
}

func (m *upCycleManager) waitForAgentEvent(ctx context.Context, cs *cycleSession) (worktreeEvent, error) {
	dir := filepath.Join(cs.Path, "outbox", "events")
	ticker := time.NewTicker(m.config.EventPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return worktreeEvent{}, ctx.Err()
		case <-ticker.C:
			entries, err := os.ReadDir(dir)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return worktreeEvent{}, fmt.Errorf("session %s: read events: %w", cs.Name, err)
			}
			if len(entries) == 0 {
				continue
			}
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].Name() < entries[j].Name()
			})
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				path := filepath.Join(dir, entry.Name())
				if _, ok := cs.eventSeen[path]; ok {
					continue
				}
				cs.eventSeen[path] = struct{}{}
				evt, err := readWorktreeEvent(path)
				if err != nil {
					_ = appendWorktreeLog(cs.WorktreeSession, fmt.Sprintf("Failed to parse %s: %v", entry.Name(), err))
					continue
				}
				if evt.Type != "agent_complete" {
					continue
				}
				if evt.Cycle != 0 && evt.Cycle != cs.cycle {
					continue
				}
				_ = m.archiveEventFile(cs, path)
				if cs.agentWindow != "" {
					_ = m.orchestrator.killTmuxWindow(cs.agentWindow)
					cs.agentWindow = ""
				}
				return evt, nil
			}
		}
	}
}

func (m *upCycleManager) runPostCycleOrchestrator(ctx context.Context, cs *cycleSession, evt worktreeEvent) error {
	status := WorktreeStatus{Phase: "up-cycle", State: "review", Cycle: cs.cycle, Global: m.cycleNumber, Updated: time.Now().UTC()}
	_ = updateWorktreeStatusFile(cs.WorktreeSession, status)
	window := fmt.Sprintf("worktree-orchestrator-%d-%d", cs.Number, cs.cycle)
	if err := m.orchestrator.createTmuxWindowInDir(window, cs.Path); err != nil {
		return fmt.Errorf("session %s: orchestrator window: %w", cs.Name, err)
	}
	defer m.orchestrator.killTmuxWindow(window)
	marker := filepath.Join(cs.Path, "outbox", "events", fmt.Sprintf("orchestrator-cycle-%d.json", cs.cycle))
	prompt := m.buildOrchestratorPrompt(cs, evt, marker)
	if err := m.orchestrator.runOpenCode(prompt, window, ""); err != nil {
		return fmt.Errorf("session %s: orchestrator launch: %w", cs.Name, err)
	}
	if err := m.orchestrator.waitForFile(marker, m.config.OrchestratorTimeout); err != nil {
		return fmt.Errorf("session %s: orchestrator timeout: %w", cs.Name, err)
	}
	_ = m.archiveEventFile(cs, marker)
	_ = appendWorktreeLog(cs.WorktreeSession, fmt.Sprintf("Orchestrator finished cycle %d", cs.cycle))
	if err := m.archiveWorktree(cs, len(evt.RemainingBeads) > 0); err != nil {
		return fmt.Errorf("session %s: archive worktree: %w", cs.Name, err)
	}
	return nil
}

func (m *upCycleManager) watchQuestions(ctx context.Context, cs *cycleSession) {
	dir := filepath.Join(cs.Path, "outbox", "questions")
	ticker := time.NewTicker(m.config.QuestionPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			entries, err := os.ReadDir(dir)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				_ = appendWorktreeLog(cs.WorktreeSession, fmt.Sprintf("Question watcher error: %v", err))
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
					continue
				}
				path := filepath.Join(dir, entry.Name())
				if _, ok := cs.questionSeen[path]; ok {
					continue
				}
				cs.questionSeen[path] = struct{}{}
				go m.handleQuestion(ctx, cs, path)
			}
		}
	}
}

func (m *upCycleManager) handleQuestion(ctx context.Context, cs *cycleSession, questionPath string) {
	responsePath := responsePathForQuestion(cs.Path, questionPath)
	timer := time.NewTimer(m.config.IdleTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		if fileExists(responsePath) {
			return
		}
		_ = appendWorktreeLog(cs.WorktreeSession, fmt.Sprintf("Auto-orchestrator responding to %s", filepath.Base(questionPath)))
		if err := m.spawnAutoResponse(cs, questionPath, responsePath); err != nil {
			_ = appendWorktreeLog(cs.WorktreeSession, fmt.Sprintf("Auto-response failed: %v", err))
		}
	}
}

func (m *upCycleManager) spawnAutoResponse(cs *cycleSession, questionPath, responsePath string) error {
	window := fmt.Sprintf("worktree-help-%d-%d", cs.Number, time.Now().Unix())
	if err := m.orchestrator.createTmuxWindowInDir(window, cs.Path); err != nil {
		return err
	}
	defer m.orchestrator.killTmuxWindow(window)
	worktreePath := filepath.Join(cs.Path, "WORKTREE.md")
	prompt := fmt.Sprintf(
		"You are the orchestrator answering a blocking question. Read %s for the question and %s for current context. "+
			"Write a concise response to %s. Provide a direct answer or advise them to continue with best judgement.",
		questionPath, worktreePath, responsePath,
	)
	if err := m.orchestrator.runOpenCode(prompt, window, ""); err != nil {
		return err
	}
	return m.orchestrator.waitForFile(responsePath, m.config.ResponseTimeout)
}

func (m *upCycleManager) archiveWorktree(cs *cycleSession, hasRemaining bool) error {
	current := filepath.Join(cs.Path, "WORKTREE.md")
	archiveName := fmt.Sprintf("CYCLE-%d-WORKTREE.md", cs.cycle)
	archivePath := filepath.Join(cs.Path, "archive", archiveName)
	if err := os.Rename(current, archivePath); err != nil {
		return err
	}
	nextCycle := cs.cycle + 1
	if !hasRemaining {
		nextCycle = cs.cycle
	}
	nextStatus := WorktreeStatus{Phase: "up-cycle", Cycle: nextCycle, Global: m.cycleNumber, Updated: time.Now().UTC()}
	if hasRemaining {
		nextStatus.State = "pending"
	} else {
		nextStatus.State = "complete"
	}
	sessionCopy := cs.WorktreeSession
	sessionCopy.CreatedAt = time.Now().UTC()
	return writeWorktreeState(sessionCopy, nextStatus)
}

func (m *upCycleManager) buildAgentPrompt(cs *cycleSession, finalSkillPath string) string {
	worktreePath := filepath.Join(cs.Path, "WORKTREE.md")
	questionDir := filepath.Join(cs.Path, "outbox", "questions")
	responseDir := filepath.Join(cs.Path, "inbox", "responses")
	eventPath := filepath.Join(cs.Path, "outbox", "events", fmt.Sprintf("agent-cycle-%d.json", cs.cycle))
	agentManual := filepath.Join(m.orchestrator.config.ProjectDir, "AGENTS.md")
	memoryPath := cs.Agent.Memory
	memoryLine := ""
	if memoryPath != "" {
		memoryLine = fmt.Sprintf("Personal memory: %s (load fully before working)\n", memoryPath)
	}
	var beadLines []string
	for _, bead := range cs.Beads {
		beadLines = append(beadLines, fmt.Sprintf("- %s · %s (%d pt)", bead.ID, bead.Title, bead.Points))
	}
	beadSection := "(no beads assigned)"
	if len(beadLines) > 0 {
		beadSection = strings.Join(beadLines, "\n")
	}
	return fmt.Sprintf(
		"Get started on the work that is assigned to you. Use bd for issue tracking. Only do the work that is assigned to you in beads (bd).\n\n"+
			"Session: %s (cycle %d)\n"+
			"Worktree root: %s\n"+
			"Context file (load entirely): %s\n"+
			"Agent instructions: %s\n"+
			"%s"+
			"\n"+
			"Assigned beads:\n%s\n\n"+
			"Guidance:\n"+
			"1. Work bead-by-bead. Keep WORKTREE.md updated with your current bead, status, and timestamps.\n"+
			"2. If you discover an unrelated bug, log a single-sentence entry under '# unrelated bugs' in WORKTREE.md with the file path.\n"+
			"3. If you attempt the same bead three times without success, add an entry under '# need help' with the bead ID, describe the problem, and unassign it via bd before moving on.\n"+
			"4. Follow AGENTS.md when committing: clean working tree, run tests, git pull --rebase, bd sync, git push.\n"+
			"5. For orchestrator questions, drop a markdown file into %s. Use the filename cycle-%d-<slug>.md and wait for an answer in %s (same slug + .response.md).\n"+
			"6. If you wait too long, default to best judgement—but still log the question thread in WORKTREE.md.\n"+
			"7. When you finish or hit context compaction, run the final-session-prompt skill at %s and paste the output into WORKTREE.md.\n"+
			"8. After all work you can do this cycle is complete, write a JSON event to %s with:\n"+
			"   {\n     \"type\": \"agent_complete\",\n     \"cycle\": %d,\n     \"completedBeads\": [..],\n     \"remainingBeads\": [..],\n     \"message\": \"notes for orchestrator\"\n   }\n"+
			"   Then exit.\n",
		cs.Name,
		cs.cycle,
		cs.Path,
		worktreePath,
		agentManual,
		memoryLine,
		beadSection,
		questionDir,
		cs.cycle,
		responseDir,
		finalSkillPath,
		eventPath,
		cs.cycle,
	)
}

func (m *upCycleManager) buildOrchestratorPrompt(cs *cycleSession, evt worktreeEvent, marker string) string {
	worktreePath := filepath.Join(cs.Path, "WORKTREE.md")
	planPath := filepath.Join(m.orchestrator.config.LatticeProjectDir, "action", "PLAN.md")
	return fmt.Sprintf(
		"You are the orchestrator for %s (cycle %d).\n"+
			"1. Read %s for the full session log.\n"+
			"2. For each entry under '# unrelated bugs', create a new bead via bd with clear title and reference.\n"+
			"3. For each remaining bead called out in the event summary, update its bead description/status with any relevant notes.\n"+
			"4. For every '# need help' entry, append it to %s under '# cycle %d' -> '## help' with '- <worktree>/<bead>: <summary>'.\n"+
			"5. Answer any outstanding agent questions if necessary.\n"+
			"6. When done, write a JSON file to %s with {\"type\":\"orchestrator_complete\", \"cycle\":%d, \"notes\":\"...\"}.\n"+
			"Leave WORKTREE archiving to the system.",
		cs.Name,
		cs.cycle,
		worktreePath,
		planPath,
		cs.cycle,
		marker,
		cs.cycle,
	)
}

func (m *upCycleManager) filterRemainingBeads(cs *cycleSession, remaining []string) []Bead {
	if len(remaining) == 0 {
		return nil
	}
	set := make(map[string]struct{})
	for _, id := range remaining {
		set[canonicalBeadKey(id)] = struct{}{}
	}
	var filtered []Bead
	for key, bead := range cs.beadsByID {
		if _, ok := set[key]; ok {
			filtered = append(filtered, bead)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].ID < filtered[j].ID
	})
	return filtered
}

func (m *upCycleManager) archiveEventFile(cs *cycleSession, path string) error {
	archiveDir := filepath.Join(cs.Path, "archive", "events")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return err
	}
	return os.Rename(path, filepath.Join(archiveDir, filepath.Base(path)))
}

func readWorktreeEvent(path string) (worktreeEvent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return worktreeEvent{}, err
	}
	var evt worktreeEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return worktreeEvent{}, err
	}
	if evt.Type == "" {
		evt.Type = "agent_complete"
	}
	return evt, nil
}

func responsePathForQuestion(sessionPath, questionPath string) string {
	base := strings.TrimSuffix(filepath.Base(questionPath), filepath.Ext(questionPath))
	return filepath.Join(sessionPath, "inbox", "responses", base+".response.md")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func canonicalBeadKey(id string) string {
	return strings.ToUpper(strings.TrimSpace(id))
}

type worktreeEvent struct {
	Type           string   `json:"type"`
	Cycle          int      `json:"cycle"`
	Message        string   `json:"message"`
	RemainingBeads []string `json:"remainingBeads"`
	CompletedBeads []string `json:"completedBeads"`
}
