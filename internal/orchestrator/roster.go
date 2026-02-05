package orchestrator

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kingrea/The-Lattice/internal/workflow"
)

const (
	defaultSpecialistCapacity = 4
	fallbackMaxAgents         = 4
)

type scheduledAgent struct {
	Agent    ProjectAgent
	Role     string
	Capacity int
}

func (o *Orchestrator) selectScheduledAgents() ([]scheduledAgent, error) {
	roster, err := workflow.LoadWorkers(o.config.WorkerListPath())
	if err != nil {
		if os.IsNotExist(err) {
			return o.fallbackScheduledAgents()
		}
		return nil, err
	}
	filtered := o.filterRoster(roster)
	if len(filtered) == 0 {
		return o.fallbackScheduledAgents()
	}
	return o.bindRosterToAgents(filtered)
}

func (o *Orchestrator) filterRoster(entries []workflow.WorkerEntry) []workflow.WorkerEntry {
	allowSpark := allowSparkAssignments()
	filtered := make([]workflow.WorkerEntry, 0, len(entries))
	for _, entry := range entries {
		normalized, err := entry.Normalize()
		if err != nil {
			continue
		}
		if normalized.IsSpark && !allowSpark {
			continue
		}
		filtered = append(filtered, normalized)
	}
	return filtered
}

func allowSparkAssignments() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("LATTICE_ASSIGN_SPARK")))
	return value == "1" || value == "true" || value == "yes"
}

func (o *Orchestrator) bindRosterToAgents(roster []workflow.WorkerEntry) ([]scheduledAgent, error) {
	projectAgents, err := o.loadProjectAgents()
	if err != nil {
		return nil, err
	}
	index := make(map[string]ProjectAgent, len(projectAgents))
	for _, agent := range projectAgents {
		index[strings.ToLower(strings.TrimSpace(agent.Name))] = agent
	}
	scheduled := make([]scheduledAgent, 0, len(roster))
	for _, entry := range roster {
		key := strings.ToLower(strings.TrimSpace(entry.Name))
		agent, ok := index[key]
		if !ok {
			return nil, fmt.Errorf("agent %s is missing from .lattice/agents", entry.Name)
		}
		scheduled = append(scheduled, scheduledAgent{
			Agent:    agent,
			Role:     entry.Role,
			Capacity: capacityForEntry(entry),
		})
	}
	return scheduled, nil
}

func capacityForEntry(entry workflow.WorkerEntry) int {
	if entry.Capacity > 0 {
		return entry.Capacity
	}
	if strings.EqualFold(entry.Role, "specialist") {
		return defaultSpecialistCapacity
	}
	return maxAgentStoryPoints
}

func (o *Orchestrator) fallbackScheduledAgents() ([]scheduledAgent, error) {
	projectAgents, err := o.loadProjectAgents()
	if err != nil {
		return nil, err
	}
	if len(projectAgents) == 0 {
		return nil, fmt.Errorf("no agent files available; run hiring first")
	}
	sort.SliceStable(projectAgents, func(i, j int) bool {
		return strings.ToLower(projectAgents[i].Name) < strings.ToLower(projectAgents[j].Name)
	})
	limit := len(projectAgents)
	if limit > fallbackMaxAgents {
		limit = fallbackMaxAgents
	}
	scheduled := make([]scheduledAgent, 0, limit)
	for i := 0; i < limit; i++ {
		scheduled = append(scheduled, scheduledAgent{
			Agent:    projectAgents[i],
			Role:     projectAgents[i].Role,
			Capacity: maxAgentStoryPoints,
		})
	}
	return scheduled, nil
}
