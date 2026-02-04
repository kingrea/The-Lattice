package engine

import (
	"strings"

	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/workflow"
)

func applyWorkflowRuntime(def workflow.WorkflowDefinition, runtime EngineRuntime) EngineRuntime {
	if def.Runtime.MaxParallel > 0 && runtime.MaxParallel <= 0 {
		runtime.MaxParallel = def.Runtime.MaxParallel
	}
	return runtime
}

func releaseRunning(running []string, updates []ModuleStatusUpdate) []string {
	if len(running) == 0 || len(updates) == 0 {
		return running
	}
	released := map[string]struct{}{}
	for _, update := range updates {
		id := strings.TrimSpace(update.ID)
		if id == "" {
			continue
		}
		status := update.Result.Status
		if status == "" {
			if update.Err != nil {
				status = module.StatusFailed
			} else {
				status = module.StatusCompleted
			}
		}
		if status == module.StatusNeedsInput {
			continue
		}
		released[id] = struct{}{}
	}
	if len(released) == 0 {
		return running
	}
	filtered := make([]string, 0, len(running))
	for _, id := range running {
		if _, drop := released[id]; drop {
			continue
		}
		filtered = append(filtered, id)
	}
	return filtered
}

func appendRunning(running []string, ids []string) []string {
	if len(ids) == 0 {
		return running
	}
	set := make(map[string]struct{}, len(running))
	for _, id := range running {
		if id == "" {
			continue
		}
		set[id] = struct{}{}
	}
	for _, id := range ids {
		clean := strings.TrimSpace(id)
		if clean == "" {
			continue
		}
		if _, exists := set[clean]; exists {
			continue
		}
		running = append(running, clean)
		set[clean] = struct{}{}
	}
	return running
}

func stripIDs(values []string, ids []string) []string {
	if len(values) == 0 || len(ids) == 0 {
		return values
	}
	drop := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		drop[id] = struct{}{}
	}
	if len(drop) == 0 {
		return values
	}
	filtered := make([]string, 0, len(values))
	for _, id := range values {
		if _, remove := drop[id]; remove {
			continue
		}
		filtered = append(filtered, id)
	}
	return filtered
}

func filterClaimable(runnable []string, requested []string) []string {
	if len(runnable) == 0 {
		return nil
	}
	if len(requested) == 0 {
		out := make([]string, len(runnable))
		copy(out, runnable)
		return out
	}
	allowed := make(map[string]struct{}, len(requested))
	for _, id := range requested {
		clean := strings.TrimSpace(id)
		if clean == "" {
			continue
		}
		allowed[clean] = struct{}{}
	}
	var filtered []string
	for _, id := range runnable {
		if _, ok := allowed[id]; ok {
			filtered = append(filtered, id)
		}
	}
	return filtered
}
