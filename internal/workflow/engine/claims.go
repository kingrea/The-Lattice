package engine

import (
	"fmt"

	"github.com/yourusername/lattice/internal/module"
)

// ClaimRequest asks the engine to reserve runnable modules for execution.
type ClaimRequest struct {
	Runtime *RuntimeOverrides
	// Limit caps how many runnable modules may be claimed at once. Zero means "all".
	Limit int
	// Modules restricts claims to a subset of runnable module IDs. When empty,
	// every runnable module is eligible.
	Modules []string
}

// WorkClaim describes a runnable module that has been reserved for execution.
type WorkClaim struct {
	ID          string                    `json:"id"`
	ModuleID    string                    `json:"module_id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description,omitempty"`
	Optional    bool                      `json:"optional,omitempty"`
	Concurrency module.ConcurrencyProfile `json:"concurrency"`
}

// ClaimResult returns the new engine state plus the reserved modules.
type ClaimResult struct {
	Claims []WorkClaim
	State  State
}

// Claim reserves runnable modules, marks them as running, and persists the new
// engine snapshot so other workers observe the updated runtime state.
func (e *Engine) Claim(ctx *module.ModuleContext, req ClaimRequest) (ClaimResult, error) {
	if ctx == nil {
		return ClaimResult{}, fmt.Errorf("workflow engine: module context is required")
	}
	current, err := e.repo.Load()
	if err != nil {
		return ClaimResult{}, err
	}
	runtime := applyRuntimeOverrides(current.Runtime, req.Runtime)
	state, err := e.buildState(ctx, current.Definition, runtime, current.Runs)
	if err != nil {
		return ClaimResult{}, err
	}
	state.RunID = current.RunID
	state.WorkflowID = current.WorkflowID
	runnable := filterClaimable(state.Runnable, req.Modules)
	limit := len(runnable)
	if req.Limit > 0 && req.Limit < limit {
		limit = req.Limit
	}
	claimIDs := make([]string, limit)
	copy(claimIDs, runnable[:limit])
	claims := make([]WorkClaim, 0, len(claimIDs))
	for _, id := range claimIDs {
		status, ok := findModuleStatus(state.Nodes, id)
		if !ok {
			continue
		}
		claims = append(claims, WorkClaim{
			ID:          status.ID,
			ModuleID:    status.ModuleID,
			Name:        status.Name,
			Description: status.Description,
			Optional:    status.Optional,
			Concurrency: status.Concurrency,
		})
	}
	state.Runtime.Running = appendRunning(state.Runtime.Running, claimIDs)
	state.Runnable = stripIDs(state.Runnable, claimIDs)
	state.Status, state.StatusReason = deriveEngineStatus(state.Nodes, state.Runtime, state.Runs)
	state.UpdatedAt = e.now()
	if err := e.repo.Save(state); err != nil {
		return ClaimResult{}, err
	}
	return ClaimResult{Claims: claims, State: state}, nil
}

func findModuleStatus(nodes []ModuleStatus, id string) (ModuleStatus, bool) {
	for _, node := range nodes {
		if node.ID == id {
			return node, true
		}
	}
	return ModuleStatus{}, false
}
