package engine

import (
	"fmt"
	"strings"
	"time"

	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/workflow"
	"github.com/kingrea/The-Lattice/internal/workflow/resolver"
	"github.com/kingrea/The-Lattice/internal/workflow/scheduler"
)

// Engine coordinates the resolver and scheduler while persisting workflow state.
type Engine struct {
	registry *module.Registry
	repo     StateStore
	clock    func() time.Time
}

// Option customizes the engine instance.
type Option func(*Engine)

// WithClock injects a deterministic clock (primarily for tests).
func WithClock(clock func() time.Time) Option {
	return func(e *Engine) {
		if clock != nil {
			e.clock = clock
		}
	}
}

// New wires a workflow engine to the module registry and persistence store.
func New(registry *module.Registry, repo StateStore, opts ...Option) (*Engine, error) {
	if registry == nil {
		return nil, fmt.Errorf("workflow engine: module registry is required")
	}
	if repo == nil {
		return nil, fmt.Errorf("workflow engine: state store is required")
	}
	engine := &Engine{
		registry: registry,
		repo:     repo,
		clock:    time.Now,
	}
	for _, opt := range opts {
		opt(engine)
	}
	return engine, nil
}

// StartRequest bootstraps a workflow definition with the engine runtime.
type StartRequest struct {
	Definition workflow.WorkflowDefinition
	Runtime    *RuntimeOverrides
}

// ResumeRequest refreshes persistent state after process restarts.
type ResumeRequest struct {
	Runtime *RuntimeOverrides
}

// ModuleStatusUpdate informs the engine that a module finished running.
type ModuleStatusUpdate struct {
	ID         string
	Result     module.Result
	Err        error
	FinishedAt time.Time
}

// UpdateRequest applies runtime overrides and module result updates.
type UpdateRequest struct {
	Runtime *RuntimeOverrides
	Results []ModuleStatusUpdate
}

// Start evaluates a workflow definition from scratch.
func (e *Engine) Start(ctx *module.ModuleContext, req StartRequest) (State, error) {
	if ctx == nil {
		return State{}, fmt.Errorf("workflow engine: module context is required")
	}
	normalized, err := req.Definition.Normalized()
	if err != nil {
		return State{}, err
	}
	runtime := applyRuntimeOverrides(EngineRuntime{}, req.Runtime)
	state, err := e.buildState(ctx, normalized, runtime, nil)
	if err != nil {
		return State{}, err
	}
	now := e.now()
	state.RunID = generateRunID(normalized.ID, now)
	state.WorkflowID = normalized.ID
	state.UpdatedAt = now
	if err := e.repo.Save(state); err != nil {
		return State{}, err
	}
	return state, nil
}

// Resume reloads persisted state and refreshes resolver/scheduler snapshots.
func (e *Engine) Resume(ctx *module.ModuleContext, req ResumeRequest) (State, error) {
	if ctx == nil {
		return State{}, fmt.Errorf("workflow engine: module context is required")
	}
	current, err := e.repo.Load()
	if err != nil {
		return State{}, err
	}
	runtime := applyRuntimeOverrides(current.Runtime, req.Runtime)
	state, err := e.buildState(ctx, current.Definition, runtime, current.Runs)
	if err != nil {
		return State{}, err
	}
	state.RunID = current.RunID
	state.WorkflowID = current.WorkflowID
	state.UpdatedAt = e.now()
	if err := e.repo.Save(state); err != nil {
		return State{}, err
	}
	return state, nil
}

// Update merges module results, reapplies runtime overrides, and refreshes state.
func (e *Engine) Update(ctx *module.ModuleContext, req UpdateRequest) (State, error) {
	if ctx == nil {
		return State{}, fmt.Errorf("workflow engine: module context is required")
	}
	current, err := e.repo.Load()
	if err != nil {
		return State{}, err
	}
	updatedRuns := mergeRuns(current.Runs, req.Results, e.now)
	runtime := applyRuntimeOverrides(current.Runtime, req.Runtime)
	runtime.Running = releaseRunning(runtime.Running, req.Results)
	state, err := e.buildState(ctx, current.Definition, runtime, updatedRuns)
	if err != nil {
		return State{}, err
	}
	state.RunID = current.RunID
	state.WorkflowID = current.WorkflowID
	state.UpdatedAt = e.now()
	if err := e.repo.Save(state); err != nil {
		return State{}, err
	}
	return state, nil
}

// View returns the last persisted snapshot without recomputing resolver state.
func (e *Engine) View() (State, error) {
	return e.repo.Load()
}

func (e *Engine) buildState(ctx *module.ModuleContext, def workflow.WorkflowDefinition, runtime EngineRuntime, runs map[string]ModuleRun) (State, error) {
	runtime = applyWorkflowRuntime(def, runtime)
	res, err := resolver.New(def, e.registry)
	if err != nil {
		return State{}, err
	}
	if err := res.Refresh(ctx); err != nil {
		return State{}, err
	}
	sched, err := scheduler.New(res)
	if err != nil {
		return State{}, err
	}
	batch, err := sched.Runnable(runtime.schedulerRequest())
	if err != nil {
		return State{}, err
	}
	nodes := summarizeNodes(res, runs)
	runtime.Running = dropCompletedRunning(runtime.Running, nodes)
	status, reason := deriveEngineStatus(nodes, runtime, runs)
	state := State{
		WorkflowID:   def.ID,
		Definition:   def.Clone(),
		Runtime:      runtime.clone(),
		Nodes:        nodes,
		Runnable:     runnableIDs(batch.Nodes),
		Skipped:      cloneSkipped(batch.Skipped),
		Runs:         cloneRuns(runs),
		Status:       status,
		StatusReason: reason,
	}
	return state, nil
}

func summarizeNodes(res *resolver.Resolver, runs map[string]ModuleRun) []ModuleStatus {
	nodes := res.Nodes()
	result := make([]ModuleStatus, 0, len(nodes))
	for _, node := range nodes {
		info := node.Module.Info()
		ref := node.Ref
		status := ModuleStatus{
			ID:           node.ID,
			ModuleID:     ref.ModuleID,
			Name:         pickName(ref, info),
			Description:  ref.Description,
			Optional:     ref.Optional,
			Concurrency:  info.Concurrency,
			State:        node.State,
			Dependencies: cloneStrings(node.Dependencies),
			Dependents:   cloneStrings(node.Dependents),
			BlockedBy:    cloneStrings(node.BlockedBy),
		}
		if node.Err != nil {
			status.Error = node.Err.Error()
		}
		if len(node.Artifacts) > 0 {
			status.Artifacts = make(map[string]ArtifactStatus, len(node.Artifacts))
			for id, report := range node.Artifacts {
				status.Artifacts[id] = ArtifactStatus{
					ID:                  id,
					Status:              report.Status,
					ExpectedFingerprint: report.ExpectedFingerprint,
					StoredFingerprint:   report.StoredFingerprint,
					Error:               errorString(report.Err),
				}
			}
		}
		if run, ok := runs[node.ID]; ok {
			copyRun := run
			status.LastRun = &copyRun
		}
		result = append(result, status)
	}
	return result
}

func pickName(ref workflow.ModuleRef, info module.Info) string {
	if ref.Name != "" {
		return ref.Name
	}
	if info.Name != "" {
		return info.Name
	}
	if ref.ModuleID != "" {
		return ref.ModuleID
	}
	return ref.InstanceID()
}

func deriveEngineStatus(nodes []ModuleStatus, runtime EngineRuntime, runs map[string]ModuleRun) (EngineStatus, string) {
	for _, status := range nodes {
		if status.State == resolver.NodeStateError {
			return EngineStatusError, fmt.Sprintf("%s encountered an error", status.ID)
		}
	}
	for id, run := range runs {
		if run.Status == module.StatusFailed {
			return EngineStatusError, fmt.Sprintf("%s failed", id)
		}
	}
	hasReady := false
	hasPending := false
	for _, status := range nodes {
		switch status.State {
		case resolver.NodeStateReady:
			hasReady = true
		case resolver.NodeStatePending, resolver.NodeStateBlocked, resolver.NodeStateUnknown:
			hasPending = true
		}
	}
	if !hasReady && !hasPending {
		return EngineStatusComplete, ""
	}
	if hasReady || len(runtime.Running) > 0 {
		return EngineStatusRunning, ""
	}
	return EngineStatusBlocked, ""
}

func runnableIDs(nodes []*resolver.Node) []string {
	if len(nodes) == 0 {
		return nil
	}
	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = node.ID
	}
	return ids
}

func cloneSkipped(values map[string]scheduler.SkipReason) map[string]scheduler.SkipReason {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]scheduler.SkipReason, len(values))
	for id, reason := range values {
		out[id] = reason
	}
	return out
}

func cloneRuns(values map[string]ModuleRun) map[string]ModuleRun {
	if len(values) == 0 {
		return map[string]ModuleRun{}
	}
	out := make(map[string]ModuleRun, len(values))
	for id, run := range values {
		out[id] = run
	}
	return out
}

func mergeRuns(existing map[string]ModuleRun, updates []ModuleStatusUpdate, clock func() time.Time) map[string]ModuleRun {
	result := cloneRuns(existing)
	if len(updates) == 0 {
		return result
	}
	for _, update := range updates {
		if update.ID == "" {
			continue
		}
		finished := update.FinishedAt
		if finished.IsZero() {
			finished = clock()
		}
		record := ModuleRun{
			Status:     update.Result.Status,
			Message:    update.Result.Message,
			Error:      errorString(update.Err),
			FinishedAt: finished,
		}
		result[update.ID] = record
	}
	return result
}

func applyRuntimeOverrides(base EngineRuntime, overrides *RuntimeOverrides) EngineRuntime {
	if overrides == nil {
		return base
	}
	if overrides.Targets != nil {
		base.Targets = cloneStrings(*overrides.Targets)
	}
	if overrides.BatchSize != nil {
		base.BatchSize = *overrides.BatchSize
	}
	if overrides.MaxParallel != nil {
		base.MaxParallel = *overrides.MaxParallel
	}
	if overrides.Running != nil {
		base.Running = cloneStrings(*overrides.Running)
	}
	if overrides.ManualGates != nil {
		base.ManualGates = cloneManualGates(*overrides.ManualGates)
	}
	return base
}

func generateRunID(workflowID string, now time.Time) string {
	base := strings.TrimSpace(workflowID)
	if base == "" {
		base = "workflow"
	}
	base = strings.ToLower(strings.ReplaceAll(base, " ", "-"))
	return fmt.Sprintf("%s-%d", base, now.UnixNano())
}

func (e *Engine) now() time.Time {
	if e.clock == nil {
		return time.Now()
	}
	return e.clock()
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
