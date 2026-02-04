package refinement

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/modules/runtime"
	"github.com/yourusername/lattice/internal/orchestrator"
)

const (
	moduleID      = "refinement"
	moduleVersion = "1.0.0"
)

// Option customizes the refinement module.
type Option func(*Module)

// Module orchestrates stakeholder audits, synthesis, and follow-up work.
type Module struct {
	*module.Base
	now       func() time.Time
	newClient orchestratorFactory
}

// Register adds the module to the registry.
func Register(reg *module.Registry) {
	if reg == nil {
		return
	}
	reg.MustRegister(moduleID, func(module.Config) (module.Module, error) {
		return New(), nil
	})
}

// New constructs the refinement module with default behavior.
func New(opts ...Option) *Module {
	info := module.Info{
		ID:          moduleID,
		Name:        "Run Refinement Audits",
		Description: "Collects stakeholder audits, synthesizes beads, and drives follow-up work cycles.",
		Version:     moduleVersion,
		Concurrency: module.ConcurrencyProfile{Exclusive: true},
	}
	base := module.NewBase(info)
	base.SetInputs(
		artifact.WorkersJSON,
		artifact.OrchestratorState,
	)
	base.SetOutputs(
		artifact.StakeholdersJSON,
		artifact.AuditDirectory,
		artifact.AuditSynthesisDoc,
	)
	mod := &Module{
		Base:      &base,
		now:       time.Now,
		newClient: defaultClientFactory,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(mod)
		}
	}
	return mod
}

// WithClock overrides the module clock (used for metadata timestamps).
func WithClock(clock func() time.Time) Option {
	return func(m *Module) {
		if clock != nil {
			m.now = clock
		}
	}
}

// WithOrchestratorFactory swaps the orchestrator client constructor (tests).
func WithOrchestratorFactory(factory orchestratorFactory) Option {
	return func(m *Module) {
		if factory != nil {
			m.newClient = factory
		}
	}
}

// Run executes the refinement lifecycle.
func (m *Module) Run(ctx *module.ModuleContext) (module.Result, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	requested, err := m.refinementRequested(ctx)
	if err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if !requested {
		return module.Result{Status: module.StatusNoOp, Message: "refinement marker absent"}, nil
	}
	if missing, err := m.missingInput(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	} else if missing != "" {
		return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("waiting for %s", missing)}, nil
	}
	client, err := m.newClient(ctx)
	if err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	profile := detectProjectProfile(ctx.Config.ProjectDir)
	assignments, auditDir, err := m.prepareStakeholders(ctx, client, profile)
	if err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.runStakeholderAudits(ctx, client, auditDir, assignments, profile); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	followUpMsg, err := m.runFollowUpCycle(ctx, client)
	if err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.clearRefinementMarker(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	message := fmt.Sprintf("audits:%d follow-up:%s", len(assignments), followUpMsg)
	return module.Result{Status: module.StatusCompleted, Message: message}, nil
}

// IsComplete returns true when the refinement marker is absent.
func (m *Module) IsComplete(ctx *module.ModuleContext) (bool, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return false, err
	}
	requested, err := m.refinementRequested(ctx)
	if err != nil {
		return false, err
	}
	return !requested, nil
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

func (m *Module) refinementRequested(ctx *module.ModuleContext) (bool, error) {
	result, err := ctx.Artifacts.Check(artifact.RefinementNeededMarker)
	if err != nil {
		return false, fmt.Errorf("%s: check refinement marker: %w", moduleID, err)
	}
	return result.State == artifact.StateReady, nil
}

func (m *Module) prepareStakeholders(ctx *module.ModuleContext, client orchestratorClient, profile projectProfile) ([]stakeholderAssignment, string, error) {
	assignments, err := planStakeholderAssignments(ctx, client, profile)
	if err != nil {
		return nil, "", err
	}
	if err := ctx.Artifacts.Write(artifact.AuditDirectory, nil, artifact.Metadata{}); err != nil {
		return nil, "", fmt.Errorf("%s: ensure audit directory: %w", moduleID, err)
	}
	if err := m.writeStakeholdersManifest(ctx, assignments, profile); err != nil {
		return nil, "", err
	}
	return assignments, artifact.AuditDirectory.Path(ctx.Workflow), nil
}

func (m *Module) runStakeholderAudits(ctx *module.ModuleContext, client orchestratorClient, auditDir string, assignments []stakeholderAssignment, profile projectProfile) error {
	for _, assignment := range assignments {
		auditPath := filepath.Join(auditDir, fmt.Sprintf("%s-audit.md", slugify(assignment.Role)))
		if err := client.RunStakeholderAudit(assignment.Role, assignment.Agent, auditPath, profile.Summary()); err != nil {
			return fmt.Errorf("%s: run %s audit: %w", moduleID, assignment.Role, err)
		}
	}
	summaryPath, err := client.RunAuditSynthesis(auditDir, profile.Summary())
	if err != nil {
		return fmt.Errorf("%s: audit synthesis: %w", moduleID, err)
	}
	body, err := os.ReadFile(summaryPath)
	if err != nil {
		return fmt.Errorf("%s: read audit synthesis: %w", moduleID, err)
	}
	meta := m.metadataFor(ctx, artifact.AuditSynthesisDoc, artifact.StakeholdersJSON, artifact.RefinementNeededMarker)
	if err := ctx.Artifacts.Write(artifact.AuditSynthesisDoc, body, meta); err != nil {
		return fmt.Errorf("%s: write audit synthesis: %w", moduleID, err)
	}
	return nil
}

func (m *Module) runFollowUpCycle(ctx *module.ModuleContext, client orchestratorClient) (string, error) {
	sessions, err := client.PrepareWorkCycle()
	switch {
	case err == nil:
		// continue
	case errors.Is(err, orchestrator.ErrNoReadyBeads), errors.Is(err, orchestrator.ErrNoTrackedSessions):
		if err := m.writeCompletionMarker(ctx); err != nil {
			return "", err
		}
		return "no ready beads", nil
	default:
		return "", fmt.Errorf("%s: prepare follow-up cycle: %w", moduleID, err)
	}
	if len(sessions) == 0 {
		if err := m.writeCompletionMarker(ctx); err != nil {
			return "", err
		}
		return "no sessions staged", nil
	}
	if err := ctx.Artifacts.Write(artifact.WorkInProgressMarker, nil, artifact.Metadata{}); err != nil {
		return "", fmt.Errorf("%s: mark work in progress: %w", moduleID, err)
	}
	if err := client.RunUpCycle(context.Background(), sessions); err != nil {
		_ = removeIfExists(artifact.WorkInProgressMarker.Path(ctx.Workflow))
		return "", fmt.Errorf("%s: run follow-up cycle: %w", moduleID, err)
	}
	if err := m.writeCompletionMarker(ctx); err != nil {
		return "", err
	}
	points, beads := summarizeSessions(sessions)
	return fmt.Sprintf("processed %d session(s), %d bead(s), %d pt", len(sessions), beads, points), nil
}

func (m *Module) writeCompletionMarker(ctx *module.ModuleContext) error {
	if err := removeIfExists(artifact.WorkInProgressMarker.Path(ctx.Workflow)); err != nil {
		return err
	}
	if err := ctx.Artifacts.Write(artifact.WorkCompleteMarker, nil, artifact.Metadata{}); err != nil {
		return fmt.Errorf("%s: mark work complete: %w", moduleID, err)
	}
	return nil
}

func (m *Module) clearRefinementMarker(ctx *module.ModuleContext) error {
	if err := removeIfExists(artifact.RefinementNeededMarker.Path(ctx.Workflow)); err != nil {
		return fmt.Errorf("%s: clear refinement marker: %w", moduleID, err)
	}
	return nil
}

func (m *Module) metadataFor(ctx *module.ModuleContext, ref artifact.ArtifactRef, extra ...artifact.ArtifactRef) artifact.Metadata {
	meta := artifact.Metadata{
		ArtifactID: ref.ID,
		ModuleID:   moduleID,
		Version:    moduleVersion,
		Workflow:   ctx.Workflow.Dir(),
	}
	inputs := append([]artifact.ArtifactRef{}, m.Inputs()...)
	inputs = append(inputs, extra...)
	runtime.WithInputs(inputs...)(&meta)
	return meta
}

func summarizeSessions(sessions []orchestrator.WorktreeSession) (points int, beads int) {
	for _, session := range sessions {
		points += session.TotalPoints()
		beads += len(session.Beads)
	}
	return points, beads
}

func removeIfExists(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
