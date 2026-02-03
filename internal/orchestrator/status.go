package orchestrator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionSnapshot captures the real-time view of a worktree session for the TUI.
type SessionSnapshot struct {
	Worktree         WorktreeSession
	Status           WorktreeStatus
	QuestionsTotal   int
	QuestionsWaiting int
	LastUpdated      time.Time
}

// CycleStatus summarizes the currently tracked work cycle.
type CycleStatus struct {
	Cycle        int
	Status       string
	UpdatedAt    time.Time
	SessionCount int
}

// SessionSnapshots returns the current worktree sessions along with their status metadata.
func (o *Orchestrator) SessionSnapshots() ([]SessionSnapshot, error) {
	if o == nil || o.config == nil {
		return nil, fmt.Errorf("orchestrator is not initialized")
	}
	sessions, err := o.loadTrackedSessions()
	if err != nil {
		if errors.Is(err, errNoTrackedSessions) {
			return nil, nil
		}
		return nil, err
	}
	snapshots := make([]SessionSnapshot, 0, len(sessions))
	for _, session := range sessions {
		statePath := filepath.Join(session.Path, "WORKTREE.md")
		status, serr := readWorktreeStatus(statePath)
		if serr != nil {
			status = WorktreeStatus{Phase: "unknown", State: "unknown"}
		}
		total, waiting := countPendingQuestions(session.Path)
		snapshots = append(snapshots, SessionSnapshot{
			Worktree:         session,
			Status:           status,
			QuestionsTotal:   total,
			QuestionsWaiting: waiting,
			LastUpdated:      status.Updated,
		})
	}
	sort.SliceStable(snapshots, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(snapshots[i].Worktree.Agent.Name))
		right := strings.ToLower(strings.TrimSpace(snapshots[j].Worktree.Agent.Name))
		if left == right {
			return snapshots[i].Worktree.Name < snapshots[j].Worktree.Name
		}
		return left < right
	})
	return snapshots, nil
}

// CurrentCycleStatus reports metadata about the actively tracked cycle, if any.
func (o *Orchestrator) CurrentCycleStatus() (CycleStatus, error) {
	if o == nil || o.config == nil {
		return CycleStatus{}, fmt.Errorf("orchestrator is not initialized")
	}
	tracker, err := o.readCycleTracker()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CycleStatus{}, errNoTrackedSessions
		}
		return CycleStatus{}, err
	}
	updated, _ := time.Parse(time.RFC3339, tracker.UpdatedAt)
	return CycleStatus{
		Cycle:        tracker.Cycle,
		Status:       tracker.Status,
		UpdatedAt:    updated,
		SessionCount: len(tracker.Sessions),
	}, nil
}
