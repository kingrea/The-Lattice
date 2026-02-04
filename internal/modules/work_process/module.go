package work_process

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/modules/runtime"
	"github.com/yourusername/lattice/internal/orchestrator"
)

const (
	moduleID      = "work-process"
	moduleVersion = "1.0.0"
)

// Option customizes the work process module.
type Option func(*WorkProcessModule)

// WithClock overrides the timestamp source.
func WithClock(clock func() time.Time) Option {
	return func(m *WorkProcessModule) {
		if clock != nil {
			m.now = clock
		}
	}
}

// WithRunner swaps the cycle runner implementation (used in tests).
func WithRunner(r cycleRunner) Option {
	return func(m *WorkProcessModule) {
		if r != nil {
			m.runner = r
		}
	}
}

type cycleRunner interface {
	Prepare(*module.ModuleContext) ([]orchestrator.WorktreeSession, error)
	Execute(context.Context, *module.ModuleContext, []orchestrator.WorktreeSession) error
}

// WorkProcessModule orchestrates agent work cycles and records provenance.
type WorkProcessModule struct {
	*module.Base
	now    func() time.Time
	runner cycleRunner
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

// New configures the module metadata and IO contracts.
func New(opts ...Option) *WorkProcessModule {
	info := module.Info{
		ID:          moduleID,
		Name:        "Run Work Cycle",
		Description: "Stages worktree sessions, runs the orchestrator cycle, and updates work artifacts.",
		Version:     moduleVersion,
		Concurrency: module.ConcurrencyProfile{Exclusive: true},
	}
	base := module.NewBase(info)
	base.SetInputs(
		artifact.WorkersJSON,
		artifact.OrchestratorState,
		artifact.BeadsCreatedMarker,
	)
	base.SetOutputs(
		artifact.WorkTasksDoc,
		artifact.WorkLogDoc,
		artifact.WorkCompleteMarker,
	)
	mod := &WorkProcessModule{
		Base:   &base,
		now:    time.Now,
		runner: defaultCycleRunner{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(mod)
		}
	}
	return mod
}

// Run prepares the next work cycle, executes sessions, and updates artifacts.
func (m *WorkProcessModule) Run(ctx *module.ModuleContext) (module.Result, error) {
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
		return module.Result{Status: module.StatusNoOp, Message: "work already completed"}, nil
	}
	if _, err := ensureOrchestrator(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.ensureWorkDir(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	_ = m.clearRefinementMarker(ctx)
	if err := m.markInProgress(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	sessions, err := m.runner.Prepare(ctx)
	if err != nil {
		if errors.Is(err, orchestrator.ErrNoReadyBeads) || errors.Is(err, orchestrator.ErrNoTrackedSessions) {
			_ = m.markRefinementNeeded(ctx)
			return module.Result{Status: module.StatusNeedsInput, Message: "no ready beads available"}, nil
		}
		_ = m.clearInProgress(ctx)
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("%s: prepare cycle: %w", moduleID, err)
	}
	if len(sessions) == 0 {
		_ = m.markRefinementNeeded(ctx)
		return module.Result{Status: module.StatusNeedsInput, Message: "no sessions staged"}, nil
	}
	if err := m.writeTasksDoc(ctx, sessions); err != nil {
		_ = m.clearInProgress(ctx)
		return module.Result{Status: module.StatusFailed}, err
	}
	started := m.now()
	if err := m.runner.Execute(context.Background(), ctx, sessions); err != nil {
		_ = m.clearInProgress(ctx)
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("%s: run cycle: %w", moduleID, err)
	}
	if err := m.markComplete(ctx); err != nil {
		_ = m.clearInProgress(ctx)
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.appendWorkLog(ctx, sessions, started); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	return module.Result{Status: module.StatusCompleted, Message: fmt.Sprintf("ran %d session(s)", len(sessions))}, nil
}

// IsComplete returns true when the work complete marker exists.
func (m *WorkProcessModule) IsComplete(ctx *module.ModuleContext) (bool, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return false, err
	}
	ready, err := runtime.EnsureMarker(ctx, moduleID, moduleVersion, artifact.WorkCompleteMarker)
	if err != nil {
		return false, err
	}
	return ready, nil
}

func (m *WorkProcessModule) missingInput(ctx *module.ModuleContext) (string, error) {
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

func (m *WorkProcessModule) ensureWorkDir(ctx *module.ModuleContext) error {
	if ctx == nil || ctx.Workflow == nil {
		return fmt.Errorf("%s: workflow unavailable", moduleID)
	}
	return os.MkdirAll(ctx.Workflow.WorkDir(), 0o755)
}

func (m *WorkProcessModule) markInProgress(ctx *module.ModuleContext) error {
	return ctx.Artifacts.Write(artifact.WorkInProgressMarker, nil, artifact.Metadata{})
}

func (m *WorkProcessModule) clearInProgress(ctx *module.ModuleContext) error {
	return removeIfExists(artifact.WorkInProgressMarker.Path(ctx.Workflow))
}

func (m *WorkProcessModule) markComplete(ctx *module.ModuleContext) error {
	if err := m.clearInProgress(ctx); err != nil {
		return err
	}
	if err := m.clearRefinementMarker(ctx); err != nil {
		return err
	}
	return ctx.Artifacts.Write(artifact.WorkCompleteMarker, nil, artifact.Metadata{})
}

func (m *WorkProcessModule) markRefinementNeeded(ctx *module.ModuleContext) error {
	if err := m.clearInProgress(ctx); err != nil {
		return err
	}
	if err := removeIfExists(artifact.WorkCompleteMarker.Path(ctx.Workflow)); err != nil {
		return err
	}
	return ctx.Artifacts.Write(artifact.RefinementNeededMarker, nil, artifact.Metadata{})
}

func (m *WorkProcessModule) clearRefinementMarker(ctx *module.ModuleContext) error {
	return removeIfExists(artifact.RefinementNeededMarker.Path(ctx.Workflow))
}

func (m *WorkProcessModule) writeTasksDoc(ctx *module.ModuleContext, sessions []orchestrator.WorktreeSession) error {
	sorted := append([]orchestrator.WorktreeSession(nil), sessions...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
	})
	var b strings.Builder
	timestamp := m.now().UTC().Format(time.RFC3339)
	fmt.Fprintf(&b, "# Prepared Work Sessions\n\nGenerated at %s.\n\n", timestamp)
	for _, session := range sorted {
		fmt.Fprintf(&b, "## %s — %s\n", session.Name, session.Agent.Name)
		fmt.Fprintf(&b, "- total points: %d\n", session.TotalPoints())
		fmt.Fprintf(&b, "- beads (%d):\n", len(session.Beads))
		for _, bead := range session.Beads {
			fmt.Fprintf(&b, "  - %s · %s (%d pt)\n", bead.ID, bead.Title, bead.Points)
		}
		b.WriteString("\n")
	}
	if len(sorted) == 0 {
		b.WriteString("_No sessions staged._\n")
	}
	meta := m.metadataFor(ctx, artifact.WorkTasksDoc)
	if meta.Notes == nil {
		meta.Notes = map[string]string{}
	}
	meta.Notes[module.FingerprintNoteKey(artifact.WorkTasksDoc.ID)] = planFingerprint(sorted)
	return ctx.Artifacts.Write(artifact.WorkTasksDoc, []byte(b.String()), meta)
}

func (m *WorkProcessModule) appendWorkLog(ctx *module.ModuleContext, sessions []orchestrator.WorktreeSession, started time.Time) error {
	body, err := existingBody(ctx, artifact.WorkLogDoc)
	if err != nil {
		return err
	}
	var b strings.Builder
	if len(body) > 0 {
		b.Write(body)
		if !strings.HasSuffix(strings.TrimSpace(string(body)), "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	timestamp := started.UTC().Format(time.RFC3339)
	fmt.Fprintf(&b, "## Cycle dispatch (%s)\n\n", timestamp)
	for _, session := range sessions {
		fmt.Fprintf(&b, "- %s → %s (%d pt, %d bead)\n", session.Agent.Name, session.Name, session.TotalPoints(), len(session.Beads))
		for _, bead := range session.Beads {
			fmt.Fprintf(&b, "  - %s · %s (%d pt)\n", bead.ID, bead.Title, bead.Points)
		}
	}
	if len(sessions) == 0 {
		b.WriteString("- no sessions executed\n")
	}
	return ctx.Artifacts.Write(artifact.WorkLogDoc, []byte(b.String()), m.metadataFor(ctx, artifact.WorkLogDoc))
}

func (m *WorkProcessModule) metadataFor(ctx *module.ModuleContext, ref artifact.ArtifactRef) artifact.Metadata {
	return artifact.Metadata{
		ArtifactID: ref.ID,
		ModuleID:   moduleID,
		Version:    moduleVersion,
		Workflow:   ctx.Workflow.Dir(),
		Inputs:     inputIDs(m.Inputs()),
	}
}

func planFingerprint(sessions []orchestrator.WorktreeSession) string {
	if len(sessions) == 0 {
		return "none"
	}
	var parts []string
	for _, session := range sessions {
		beadIDs := make([]string, len(session.Beads))
		for i, bead := range session.Beads {
			beadIDs[i] = strings.ToLower(strings.TrimSpace(bead.ID))
		}
		sort.Strings(beadIDs)
		parts = append(parts, fmt.Sprintf("%s|%s|%s", strings.ToLower(strings.TrimSpace(session.Name)), strings.ToLower(strings.TrimSpace(session.Agent.Name)), strings.Join(beadIDs, ",")))
	}
	joined := strings.Join(parts, ";")
	sum := sha256.Sum256([]byte(joined))
	return fmt.Sprintf("%x", sum[:])
}

func existingBody(ctx *module.ModuleContext, ref artifact.ArtifactRef) ([]byte, error) {
	if ctx == nil || ctx.Workflow == nil {
		return nil, fmt.Errorf("%s: workflow unavailable", moduleID)
	}
	path := ref.Path(ctx.Workflow)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	meta, body, err := artifact.ParseFrontMatter(data)
	if err == nil {
		_ = meta
		return body, nil
	}
	if errors.Is(err, artifact.ErrMissingFrontMatter) || errors.Is(err, artifact.ErrMalformedFrontMatter) {
		return data, nil
	}
	return nil, err
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

func ensureOrchestrator(ctx *module.ModuleContext) (*orchestrator.Orchestrator, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%s: module context missing", moduleID)
	}
	if ctx.Orchestrator != nil {
		return ctx.Orchestrator, nil
	}
	if ctx.Config == nil {
		return nil, fmt.Errorf("%s: config unavailable", moduleID)
	}
	ctx.Orchestrator = orchestrator.New(ctx.Config)
	return ctx.Orchestrator, nil
}

type defaultCycleRunner struct{}

func (defaultCycleRunner) Prepare(ctx *module.ModuleContext) ([]orchestrator.WorktreeSession, error) {
	orch, err := ensureOrchestrator(ctx)
	if err != nil {
		return nil, err
	}
	return orch.PrepareWorkCycle()
}

func (defaultCycleRunner) Execute(goCtx context.Context, ctx *module.ModuleContext, sessions []orchestrator.WorktreeSession) error {
	orch, err := ensureOrchestrator(ctx)
	if err != nil {
		return err
	}
	return orch.RunUpCycle(goCtx, sessions)
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
