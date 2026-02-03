package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yourusername/lattice/internal/workflow"
)

var errNoTrackedSessions = errors.New("no tracked sessions")

// ErrNoTrackedSessions is returned when no worktree sessions are being tracked.
var ErrNoTrackedSessions = errNoTrackedSessions

type cycleTracker struct {
	Cycle     int              `json:"cycle"`
	Status    string           `json:"status"`
	UpdatedAt string           `json:"updatedAt"`
	Sessions  []trackedSession `json:"sessions"`
}

type trackedSession struct {
	Number    int    `json:"number"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	AgentName string `json:"agentName"`
	AgentPath string `json:"agentPath,omitempty"`
	CreatedAt string `json:"createdAt"`
	Beads     []Bead `json:"beads"`
}

func (o *Orchestrator) cycleTrackerPath() string {
	return filepath.Join(o.config.WorkflowDir(), workflow.WorkDir, "current-cycle.json")
}

func (o *Orchestrator) persistCycleTracker(cycle int, sessions []WorktreeSession, status string) error {
	tracker := cycleTracker{Cycle: cycle, Status: status, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	tracker.Sessions = make([]trackedSession, 0, len(sessions))
	for _, session := range sessions {
		created := session.CreatedAt
		if created.IsZero() {
			created = time.Now().UTC()
		}
		tracker.Sessions = append(tracker.Sessions, trackedSession{
			Number:    session.Number,
			Name:      session.Name,
			Path:      session.Path,
			AgentName: session.Agent.Name,
			AgentPath: session.Agent.Path,
			CreatedAt: created.Format(time.RFC3339),
			Beads:     append([]Bead(nil), session.Beads...),
		})
	}
	return o.writeCycleTracker(tracker)
}

func (o *Orchestrator) updateCycleTrackerStatus(status string) error {
	tracker, err := o.readCycleTracker()
	if err != nil {
		return err
	}
	tracker.Status = status
	tracker.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return o.writeCycleTracker(tracker)
}

func (o *Orchestrator) clearCycleTracker() error {
	if err := os.Remove(o.cycleTrackerPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (o *Orchestrator) loadTrackedSessions() ([]WorktreeSession, error) {
	tracker, err := o.readCycleTracker()
	if err != nil {
		if os.IsNotExist(err) {
			return o.reconstructTrackedSessions()
		}
		return nil, err
	}
	sessions, err := o.sessionsFromTracker(tracker)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, errNoTrackedSessions
	}
	return sessions, nil
}

func (o *Orchestrator) readCycleTracker() (cycleTracker, error) {
	path := o.cycleTrackerPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return cycleTracker{}, err
	}
	var tracker cycleTracker
	if err := json.Unmarshal(data, &tracker); err != nil {
		return cycleTracker{}, fmt.Errorf("failed to parse cycle tracker: %w", err)
	}
	return tracker, nil
}

func (o *Orchestrator) writeCycleTracker(tracker cycleTracker) error {
	path := o.cycleTrackerPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tracker, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (o *Orchestrator) sessionsFromTracker(tracker cycleTracker) ([]WorktreeSession, error) {
	if len(tracker.Sessions) == 0 {
		return nil, nil
	}
	lookup, err := o.agentLookup()
	if err != nil {
		return nil, err
	}
	sessions := make([]WorktreeSession, 0, len(tracker.Sessions))
	for _, ts := range tracker.Sessions {
		agent, err := o.resolveTrackedAgent(ts, lookup)
		if err != nil {
			return nil, err
		}
		created, _ := time.Parse(time.RFC3339, ts.CreatedAt)
		session := WorktreeSession{
			Number:    ts.Number,
			Name:      ts.Name,
			Agent:     agent,
			Beads:     append([]Bead(nil), ts.Beads...),
			Path:      ts.Path,
			CreatedAt: created,
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (o *Orchestrator) resolveTrackedAgent(ts trackedSession, lookup map[string]ProjectAgent) (ProjectAgent, error) {
	if ts.AgentPath != "" {
		if agent, err := parseProjectAgentFile(ts.AgentPath); err == nil {
			return agent, nil
		}
	}
	key := strings.ToLower(strings.TrimSpace(ts.AgentName))
	if agent, ok := lookup[key]; ok {
		return agent, nil
	}
	return ProjectAgent{}, fmt.Errorf("agent %s not found in project", ts.AgentName)
}

func (o *Orchestrator) agentLookup() (map[string]ProjectAgent, error) {
	agents, err := o.loadProjectAgents()
	if err != nil {
		return nil, err
	}
	lookup := make(map[string]ProjectAgent, len(agents))
	for _, agent := range agents {
		lookup[strings.ToLower(strings.TrimSpace(agent.Name))] = agent
	}
	return lookup, nil
}

func (o *Orchestrator) reconstructTrackedSessions() ([]WorktreeSession, error) {
	sessionDirs, err := o.scanWorktreeSessions()
	if err != nil {
		return nil, err
	}
	if len(sessionDirs) == 0 {
		return nil, errNoTrackedSessions
	}
	lookup, err := o.agentLookup()
	if err != nil {
		return nil, err
	}
	sessions := make([]WorktreeSession, 0, len(sessionDirs))
	for _, dir := range sessionDirs {
		session, err := o.parseWorktreeSession(dir, lookup)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	cycle, err := o.currentCycleNumber()
	if err == nil {
		_ = o.persistCycleTracker(cycle, sessions, "prepared")
	}
	return sessions, nil
}

func (o *Orchestrator) scanWorktreeSessions() ([]string, error) {
	base := o.config.WorktreeDir()
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		root := filepath.Join(base, entry.Name())
		subEntries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, sub := range subEntries {
			if !sub.IsDir() {
				continue
			}
			dirs = append(dirs, filepath.Join(root, sub.Name()))
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

func (o *Orchestrator) parseWorktreeSession(dir string, lookup map[string]ProjectAgent) (WorktreeSession, error) {
	statePath := filepath.Join(dir, "WORKTREE.md")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return WorktreeSession{}, err
	}
	agentName, createdAt, beads := parseWorktreeState(string(data))
	if agentName == "" {
		return WorktreeSession{}, fmt.Errorf("%s missing agent metadata", statePath)
	}
	key := strings.ToLower(strings.TrimSpace(agentName))
	agent, ok := lookup[key]
	if !ok {
		return WorktreeSession{}, fmt.Errorf("agent %s not found for %s", agentName, dir)
	}
	num := parseSessionNumber(dir)
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return WorktreeSession{
		Number:    num,
		Name:      filepath.Base(dir),
		Agent:     agent,
		Beads:     beads,
		Path:      dir,
		CreatedAt: createdAt,
	}, nil
}

func parseSessionNumber(dir string) int {
	parent := filepath.Base(filepath.Dir(dir))
	num, err := strconv.Atoi(parent)
	if err != nil {
		return 0
	}
	return num
}

func parseWorktreeState(content string) (string, time.Time, []Bead) {
	lines := strings.Split(content, "\n")
	var agent string
	var created time.Time
	var beads []Bead
	inBeads := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "- Agent:"):
			agent = strings.TrimSpace(strings.TrimPrefix(line, "- Agent:"))
		case strings.HasPrefix(line, "- Created:"):
			ts := strings.TrimSpace(strings.TrimPrefix(line, "- Created:"))
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				created = t
			}
		case strings.HasPrefix(line, "## Assigned Beads"):
			inBeads = true
		case strings.HasPrefix(line, "## "):
			if inBeads {
				inBeads = false
			}
		default:
			if inBeads && strings.HasPrefix(line, "- ") {
				if bead, ok := parseBeadLine(line); ok {
					beads = append(beads, bead)
				}
			}
		}
	}
	return agent, created, beads
}

func parseBeadLine(line string) (Bead, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(line, "-"))
	if trimmed == "" {
		return Bead{}, false
	}
	points := 0
	titlePart := trimmed
	if idx := strings.LastIndex(trimmed, "("); idx != -1 {
		titlePart = strings.TrimSpace(trimmed[:idx])
		meta := trimmed[idx+1:]
		if end := strings.Index(meta, "pt"); end != -1 {
			value := strings.TrimSpace(meta[:end])
			fields := strings.Fields(value)
			if len(fields) > 0 {
				if n, err := strconv.Atoi(fields[0]); err == nil {
					points = n
				}
			}
		}
	}
	parts := strings.Split(titlePart, "·")
	if len(parts) < 2 {
		return Bead{}, false
	}
	id := strings.TrimSpace(parts[0])
	title := strings.TrimSpace(strings.Join(parts[1:], "·"))
	if id == "" {
		return Bead{}, false
	}
	if points == 0 {
		points = 1
	}
	return Bead{ID: id, Title: title, Points: points}, true
}
