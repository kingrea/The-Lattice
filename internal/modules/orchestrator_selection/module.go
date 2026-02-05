package orchestrator_selection

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/modules/runtime"
	"github.com/kingrea/The-Lattice/internal/orchestrator"
	"github.com/kingrea/The-Lattice/internal/workflow"
)

const (
	moduleID      = "orchestrator-selection"
	moduleVersion = "1.0.0"
)

// Option configures the module during construction.
type Option func(*OrchestratorSelectionModule)

// WithClock overrides the timestamp source.
func WithClock(clock func() time.Time) Option {
	return func(m *OrchestratorSelectionModule) {
		if clock != nil {
			m.now = clock
		}
	}
}

// OrchestratorSelectionModule selects a denizen to lead execution and stamps the
// roster artifacts with metadata.
type OrchestratorSelectionModule struct {
	*module.Base
	now func() time.Time
}

// Register installs the module factory.
func Register(reg *module.Registry) {
	if reg == nil {
		return
	}
	reg.MustRegister(moduleID, func(module.Config) (module.Module, error) {
		return New(), nil
	})
}

// New configures IO contracts and defaults.
func New(opts ...Option) *OrchestratorSelectionModule {
	info := module.Info{
		ID:          moduleID,
		Name:        "Select Orchestrator",
		Description: "Chooses the orchestrator denizen and populates workflow/team artifacts with provenance metadata.",
		Version:     moduleVersion,
	}
	base := module.NewBase(info)
	base.SetInputs(
		artifact.ModulesDoc,
		artifact.ActionPlanDoc,
		artifact.ReviewsAppliedMarker,
		artifact.BeadsCreatedMarker,
	)
	base.SetOutputs(
		artifact.OrchestratorState,
		artifact.WorkersJSON,
	)
	mod := &OrchestratorSelectionModule{Base: &base, now: time.Now}
	for _, opt := range opts {
		if opt != nil {
			opt(mod)
		}
	}
	return mod
}

// Run evaluates CVs, selects the strongest candidate, and writes orchestrator
// metadata alongside the worker roster.
func (m *OrchestratorSelectionModule) Run(ctx *module.ModuleContext) (module.Result, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if missing, err := m.missingInput(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	} else if missing != "" {
		return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("waiting for %s", missing)}, nil
	}
	if complete, err := m.IsComplete(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	} else if complete {
		return module.Result{Status: module.StatusNoOp, Message: "orchestrator already selected"}, nil
	}

	agents, err := m.loadAgents(ctx)
	if err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	selected, summary, err := m.pickCandidate(ctx, agents)
	if err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.ensureSideEffects(ctx, selected); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.writeOrchestratorState(ctx, selected, summary); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.writeWorkersRoster(ctx, selected); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	m.logInfo(ctx, "Selected %s (%s) as orchestrator", selected.Name, selected.Community)
	return module.Result{Status: module.StatusCompleted, Message: fmt.Sprintf("selected %s", selected.Name)}, nil
}

// IsComplete reports true when orchestrator.json and workers.json both contain
// current metadata.
func (m *OrchestratorSelectionModule) IsComplete(ctx *module.ModuleContext) (bool, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return false, err
	}
	for _, ref := range []artifact.ArtifactRef{artifact.OrchestratorState, artifact.WorkersJSON} {
		result, err := ctx.Artifacts.Check(ref)
		if err != nil {
			return false, fmt.Errorf("%s: check %s: %w", moduleID, ref.ID, err)
		}
		if result.State != artifact.StateReady {
			return false, nil
		}
		if result.Metadata == nil || result.Metadata.ModuleID != moduleID || result.Metadata.Version != moduleVersion {
			return false, nil
		}
	}
	return true, nil
}

func (m *OrchestratorSelectionModule) missingInput(ctx *module.ModuleContext) (string, error) {
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

func (m *OrchestratorSelectionModule) loadAgents(ctx *module.ModuleContext) ([]orchestrator.Agent, error) {
	var loader *orchestrator.Orchestrator
	if ctx.Orchestrator != nil {
		loader = ctx.Orchestrator
	} else if ctx.Config != nil {
		loader = orchestrator.New(ctx.Config)
	}
	if loader == nil {
		return nil, fmt.Errorf("%s: orchestrator handle unavailable", moduleID)
	}
	agents, err := loader.LoadDenizenCVs()
	if err != nil {
		return nil, fmt.Errorf("%s: load denizen cvs: %w", moduleID, err)
	}
	filtered := make([]orchestrator.Agent, 0, len(agents))
	for _, agent := range agents {
		if strings.TrimSpace(agent.Name) == "" {
			continue
		}
		filtered = append(filtered, agent)
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("%s: no orchestrator candidates found", moduleID)
	}
	return filtered, nil
}

func (m *OrchestratorSelectionModule) pickCandidate(ctx *module.ModuleContext, agents []orchestrator.Agent) (orchestrator.Agent, selectionSummary, error) {
	lockedName := m.lockedAgentName(ctx)
	if lockedName != "" {
		for _, agent := range agents {
			if strings.EqualFold(strings.TrimSpace(agent.Name), lockedName) {
				return agent, selectionSummary{Score: scoreAgent(agent), Strategy: "reuse-existing"}, nil
			}
		}
	}
	type rankedAgent struct {
		agent orchestrator.Agent
		score int
	}
	ranked := make([]rankedAgent, len(agents))
	for i, agent := range agents {
		ranked[i] = rankedAgent{agent: agent, score: scoreAgent(agent)}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return strings.ToLower(ranked[i].agent.Name) < strings.ToLower(ranked[j].agent.Name)
		}
		return ranked[i].score > ranked[j].score
	})
	winner := ranked[0]
	return winner.agent, selectionSummary{Score: winner.score, Strategy: "weighted-capabilities"}, nil
}

func (m *OrchestratorSelectionModule) ensureSideEffects(ctx *module.ModuleContext, agent orchestrator.Agent) error {
	if ctx.Orchestrator == nil {
		return nil
	}
	if err := ctx.Orchestrator.ApplyOrchestratorSelection(agent); err != nil {
		return fmt.Errorf("%s: apply orchestrator selection: %w", moduleID, err)
	}
	return nil
}

func (m *OrchestratorSelectionModule) writeOrchestratorState(ctx *module.ModuleContext, agent orchestrator.Agent, summary selectionSummary) error {
	payload := orchestratorStatePayload{
		Name:       agent.Name,
		Community:  agent.Community,
		CVPath:     agent.CVPath,
		SelectedAt: m.now().UTC().Format(time.RFC3339),
		Attributes: agentAttributes{Precision: agent.Precision, Autonomy: agent.Autonomy, Experience: agent.Experience},
		Selection:  summary,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%s: encode orchestrator state: %w", moduleID, err)
	}
	meta := artifact.Metadata{
		ArtifactID: artifact.OrchestratorState.ID,
		ModuleID:   moduleID,
		Version:    moduleVersion,
		Workflow:   ctx.Workflow.Dir(),
		Inputs:     inputIDs(m.Inputs()),
	}
	if err := ctx.Artifacts.Write(artifact.OrchestratorState, body, meta); err != nil {
		return fmt.Errorf("%s: write orchestrator.json: %w", moduleID, err)
	}
	return nil
}

func (m *OrchestratorSelectionModule) writeWorkersRoster(ctx *module.ModuleContext, agent orchestrator.Agent) error {
	existing := m.currentWorkers(ctx)
	payload := workerRosterPayload{
		Orchestrator: rosterAgent{
			Name:      agent.Name,
			Community: agent.Community,
			CVPath:    agent.CVPath,
		},
		Workers:   existing,
		UpdatedAt: m.now().UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%s: encode workers roster: %w", moduleID, err)
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

func (m *OrchestratorSelectionModule) currentWorkers(ctx *module.ModuleContext) []workflow.WorkerEntry {
	path := ctx.Workflow.WorkersPath()
	entries, err := workflow.LoadWorkers(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		m.logWarn(ctx, "Failed to read existing workers roster: %v", err)
		return nil
	}
	return entries
}

func (m *OrchestratorSelectionModule) lockedAgentName(ctx *module.ModuleContext) string {
	if ctx == nil || ctx.Workflow == nil {
		return ""
	}
	path := artifact.OrchestratorState.Path(ctx.Workflow)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Name)
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

func scoreAgent(agent orchestrator.Agent) int {
	return agent.Precision*3 + agent.Autonomy*2 + agent.Experience
}

func (m *OrchestratorSelectionModule) logInfo(ctx *module.ModuleContext, format string, args ...any) {
	if ctx == nil || ctx.Logbook == nil {
		return
	}
	ctx.Logbook.Info(format, args...)
}

func (m *OrchestratorSelectionModule) logWarn(ctx *module.ModuleContext, format string, args ...any) {
	if ctx == nil || ctx.Logbook == nil {
		return
	}
	ctx.Logbook.Warn(format, args...)
}

type agentAttributes struct {
	Precision  int `json:"precision"`
	Autonomy   int `json:"autonomy"`
	Experience int `json:"experience"`
}

type selectionSummary struct {
	Score    int    `json:"score"`
	Strategy string `json:"strategy"`
}

type orchestratorStatePayload struct {
	Name       string           `json:"name"`
	Community  string           `json:"community,omitempty"`
	CVPath     string           `json:"cvPath,omitempty"`
	SelectedAt string           `json:"selectedAt"`
	Attributes agentAttributes  `json:"attributes"`
	Selection  selectionSummary `json:"selection"`
}

type workerRosterPayload struct {
	Orchestrator rosterAgent            `json:"orchestrator"`
	Workers      []workflow.WorkerEntry `json:"workers"`
	UpdatedAt    string                 `json:"updatedAt"`
}

type rosterAgent struct {
	Name      string `json:"name"`
	Community string `json:"community,omitempty"`
	CVPath    string `json:"cvPath,omitempty"`
}
