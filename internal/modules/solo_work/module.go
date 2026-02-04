package solo_work

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/modules/runtime"
)

const (
	moduleID         = "solo-work"
	moduleVersion    = "1.0.0"
	soloWorkerName   = "Solo Operator"
	soloMode         = "solo"
	snippetRuneLimit = 640
)

// Option customizes the solo work module lifecycle.
type Option func(*Module)

// WithClock overrides the timestamp generator (tests).
func WithClock(clock func() time.Time) Option {
	return func(m *Module) {
		if clock != nil {
			m.now = clock
		}
	}
}

// Module orchestrates the single-operator execution path.
type Module struct {
	*module.Base
	now func() time.Time
}

// Register installs the module factory into the registry.
func Register(reg *module.Registry) {
	if reg == nil {
		return
	}
	reg.MustRegister(moduleID, func(module.Config) (module.Module, error) {
		return New(), nil
	})
}

// New wires the module metadata and IO contracts.
func New(opts ...Option) *Module {
	info := module.Info{
		ID:          moduleID,
		Name:        "Execute Solo Tasks",
		Description: "Guides a single operator through action plan execution and records completion artifacts.",
		Version:     moduleVersion,
	}
	base := module.NewBase(info)
	base.SetInputs(
		artifact.ModulesDoc,
		artifact.ActionPlanDoc,
	)
	base.SetOutputs(
		artifact.WorkLogDoc,
		artifact.WorkCompleteMarker,
		artifact.WorkersJSON,
		artifact.OrchestratorState,
	)
	mod := &Module{
		Base: &base,
		now:  time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(mod)
		}
	}
	return mod
}

// Run synthesizes the solo execution scaffolding.
func (m *Module) Run(ctx *module.ModuleContext) (module.Result, error) {
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
		return module.Result{Status: module.StatusNoOp, Message: "solo tasks already completed"}, nil
	}
	if err := m.writeWorkLog(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.ensureSoloRoster(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.ensureSoloOrchestrator(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.writeCompletionMarker(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	return module.Result{Status: module.StatusCompleted, Message: "solo work documented"}, nil
}

// IsComplete reports when all solo artifacts and markers exist.
func (m *Module) IsComplete(ctx *module.ModuleContext) (bool, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return false, err
	}
	for _, ref := range []artifact.ArtifactRef{artifact.WorkLogDoc, artifact.WorkersJSON, artifact.OrchestratorState} {
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
	marker, err := ctx.Artifacts.Check(artifact.WorkCompleteMarker)
	if err != nil {
		return false, fmt.Errorf("%s: check work-complete marker: %w", moduleID, err)
	}
	if marker.State != artifact.StateReady {
		return false, nil
	}
	return true, nil
}

func (m *Module) missingInput(ctx *module.ModuleContext) (string, error) {
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

func (m *Module) writeWorkLog(ctx *module.ModuleContext) error {
	modulesSnippet, err := readDocumentSnippet(artifact.ModulesDoc.Path(ctx.Workflow))
	if err != nil {
		return fmt.Errorf("%s: read modules snippet: %w", moduleID, err)
	}
	planSnippet, err := readDocumentSnippet(artifact.ActionPlanDoc.Path(ctx.Workflow))
	if err != nil {
		return fmt.Errorf("%s: read action plan snippet: %w", moduleID, err)
	}
	var b strings.Builder
	timestamp := m.now().UTC().Format(time.RFC3339)
	b.WriteString("# Solo Execution Log\n\n")
	fmt.Fprintf(&b, "Generated %s. This log guides a self-serve pass through MODULES.md and PLAN.md. Update the sections below as you ship tasks.\n\n", timestamp)
	b.WriteString("## Getting started\n")
	b.WriteString("1. Review COMMISSION/ARCHITECTURE/CONVENTIONS for scope.\n")
	b.WriteString("2. Follow the sequence captured in PLAN.md.\n")
	b.WriteString("3. Record major decisions, blockers, and handoffs directly in this log.\n\n")
	if strings.TrimSpace(modulesSnippet) != "" {
		b.WriteString("## MODULES.md snapshot\n")
		b.WriteString(modulesSnippet)
		if !strings.HasSuffix(modulesSnippet, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if strings.TrimSpace(planSnippet) != "" {
		b.WriteString("## PLAN.md snapshot\n")
		b.WriteString(planSnippet)
		if !strings.HasSuffix(planSnippet, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Live notes\n- Pending tasks:\n- Blockers:\n- Done today:\n")
	meta := artifact.Metadata{
		ArtifactID: artifact.WorkLogDoc.ID,
		ModuleID:   moduleID,
		Version:    moduleVersion,
		Workflow:   ctx.Workflow.Dir(),
		Inputs:     inputIDs(m.Inputs()),
	}
	return ctx.Artifacts.Write(artifact.WorkLogDoc, []byte(b.String()), meta)
}

func (m *Module) ensureSoloRoster(ctx *module.ModuleContext) error {
	payload := map[string]any{
		"mode":  soloMode,
		"notes": fmt.Sprintf("Auto-generated %s for single-operator workflow", m.now().UTC().Format(time.RFC3339)),
		"workers": []map[string]any{
			{
				"name":      soloWorkerName,
				"role":      "operator",
				"community": "local",
				"capacity":  32,
			},
		},
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

func (m *Module) ensureSoloOrchestrator(ctx *module.ModuleContext) error {
	payload := map[string]any{
		"name":        soloWorkerName,
		"mode":        soloMode,
		"selected_at": m.now().UTC().Format(time.RFC3339),
		"notes":       "single-operator workflow path",
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

func (m *Module) writeCompletionMarker(ctx *module.ModuleContext) error {
	meta := artifact.Metadata{
		ArtifactID: artifact.WorkCompleteMarker.ID,
		ModuleID:   moduleID,
		Version:    moduleVersion,
		Workflow:   ctx.Workflow.Dir(),
	}
	if err := ctx.Artifacts.Write(artifact.WorkCompleteMarker, nil, meta); err != nil {
		return fmt.Errorf("%s: write work-complete marker: %w", moduleID, err)
	}
	return nil
}

func readDocumentSnippet(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	body := data
	if _, b, err := artifact.ParseFrontMatter(data); err == nil {
		body = b
	} else if err != nil && !errors.Is(err, artifact.ErrMissingFrontMatter) && !errors.Is(err, artifact.ErrMalformedFrontMatter) {
		return "", err
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "", nil
	}
	runes := []rune(trimmed)
	if len(runes) <= snippetRuneLimit {
		return trimmed, nil
	}
	return strings.TrimSpace(string(runes[:snippetRuneLimit])) + "…", nil
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
