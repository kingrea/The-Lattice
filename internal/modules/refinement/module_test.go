package refinement

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/config"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/orchestrator"
	"github.com/kingrea/The-Lattice/internal/workflow"
)

func TestModuleRunGeneratesOutputs(t *testing.T) {
	ctx := newRefinementTestContext(t)
	seedRefinementInputs(t, ctx)
	if err := ctx.Artifacts.Write(artifact.RefinementNeededMarker, nil, artifact.Metadata{}); err != nil {
		t.Fatalf("write refinement marker: %v", err)
	}
	sessions := []orchestrator.WorktreeSession{{
		Number: 1,
		Name:   "tree-1-alpha",
		Agent:  orchestrator.ProjectAgent{Name: "Aster"},
		Beads:  []orchestrator.Bead{{ID: "task-1", Title: "Fix bug", Points: 3}},
	}}
	stub := &stubOrchestratorClient{
		t:               t,
		agents:          []orchestrator.ProjectAgent{{Name: "Aster"}, {Name: "Beryl"}},
		workerList:      orchestrator.WorkerList{Workers: []orchestrator.WorkerRef{{Name: "Kai"}}},
		prepareSessions: sessions,
		summaryBody:     "# Summary\n\n- Created beads\n",
	}
	mod := New(
		WithClock(func() time.Time { return time.Date(2026, 2, 4, 10, 0, 0, 0, time.UTC) }),
		WithOrchestratorFactory(func(*module.ModuleContext) (orchestratorClient, error) { return stub, nil }),
	)
	result, err := mod.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != module.StatusCompleted {
		t.Fatalf("unexpected status: %+v", result)
	}
	if stub.runUpCycleCalls != 1 {
		t.Fatalf("expected RunUpCycle to execute once, got %d", stub.runUpCycleCalls)
	}
	ensureExists(t, artifact.WorkCompleteMarker.Path(ctx.Workflow))
	ensureMissing(t, artifact.WorkInProgressMarker.Path(ctx.Workflow))
	ensureMissing(t, artifact.RefinementNeededMarker.Path(ctx.Workflow))
	manifest := readStakeholdersManifest(t, artifact.StakeholdersJSON.Path(ctx.Workflow))
	if len(manifest.Roles) != 10 {
		t.Fatalf("expected 10 roles, got %d", len(manifest.Roles))
	}
	if manifest.ProjectType == "" {
		t.Fatalf("project type missing")
	}
	summaryData, err := os.ReadFile(artifact.AuditSynthesisDoc.Path(ctx.Workflow))
	if err != nil {
		t.Fatalf("read synthesis: %v", err)
	}
	if _, _, err := artifact.ParseFrontMatter(summaryData); err != nil {
		t.Fatalf("synthesis missing frontmatter: %v", err)
	}
}

func TestModuleRunNoMarkerNoOp(t *testing.T) {
	ctx := newRefinementTestContext(t)
	seedRefinementInputs(t, ctx)
	stub := &stubOrchestratorClient{t: t, agents: []orchestrator.ProjectAgent{{Name: "Aster"}}}
	mod := New(WithOrchestratorFactory(func(*module.ModuleContext) (orchestratorClient, error) { return stub, nil }))
	result, err := mod.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != module.StatusNoOp {
		t.Fatalf("expected no-op status, got %+v", result)
	}
	ensureMissing(t, artifact.StakeholdersJSON.Path(ctx.Workflow))
}

func TestModuleRunAuditFailure(t *testing.T) {
	ctx := newRefinementTestContext(t)
	seedRefinementInputs(t, ctx)
	if err := ctx.Artifacts.Write(artifact.RefinementNeededMarker, nil, artifact.Metadata{}); err != nil {
		t.Fatalf("write refinement marker: %v", err)
	}
	stub := &stubOrchestratorClient{
		t:        t,
		agents:   []orchestrator.ProjectAgent{{Name: "Aster"}},
		auditErr: fmt.Errorf("boom"),
	}
	mod := New(WithOrchestratorFactory(func(*module.ModuleContext) (orchestratorClient, error) { return stub, nil }))
	result, err := mod.Run(ctx)
	if err == nil {
		t.Fatalf("expected error")
	}
	if result.Status != module.StatusFailed {
		t.Fatalf("expected failed status, got %+v", result)
	}
	ensureExists(t, artifact.RefinementNeededMarker.Path(ctx.Workflow))
	ensureMissing(t, artifact.WorkCompleteMarker.Path(ctx.Workflow))
}

func TestModuleRunNoReadyBeadsStillCompletes(t *testing.T) {
	ctx := newRefinementTestContext(t)
	seedRefinementInputs(t, ctx)
	if err := ctx.Artifacts.Write(artifact.RefinementNeededMarker, nil, artifact.Metadata{}); err != nil {
		t.Fatalf("write refinement marker: %v", err)
	}
	stub := &stubOrchestratorClient{
		t:          t,
		agents:     []orchestrator.ProjectAgent{{Name: "Aster"}},
		prepareErr: orchestrator.ErrNoReadyBeads,
	}
	mod := New(WithOrchestratorFactory(func(*module.ModuleContext) (orchestratorClient, error) { return stub, nil }))
	result, err := mod.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != module.StatusCompleted {
		t.Fatalf("unexpected status: %+v", result)
	}
	if stub.runUpCycleCalls != 0 {
		t.Fatalf("expected no follow-up execution")
	}
	ensureExists(t, artifact.WorkCompleteMarker.Path(ctx.Workflow))
	ensureMissing(t, artifact.RefinementNeededMarker.Path(ctx.Workflow))
}

type stubOrchestratorClient struct {
	t               *testing.T
	agents          []orchestrator.ProjectAgent
	workerList      orchestrator.WorkerList
	prepareSessions []orchestrator.WorktreeSession
	prepareErr      error
	RunErr          error
	runUpCycleCalls int
	auditErr        error
	summaryBody     string
	synthesisErr    error
}

func (s *stubOrchestratorClient) LoadProjectAgents() ([]orchestrator.ProjectAgent, error) {
	clone := make([]orchestrator.ProjectAgent, len(s.agents))
	copy(clone, s.agents)
	return clone, nil
}

func (s *stubOrchestratorClient) CurrentWorkerList() orchestrator.WorkerList {
	return s.workerList
}

func (s *stubOrchestratorClient) RunStakeholderAudit(role string, agent orchestrator.ProjectAgent, path string, _ string) error {
	s.t.Helper()
	if s.auditErr != nil {
		return s.auditErr
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("# %s\n\nAgent: %s\n", role, agent.Name)
	return os.WriteFile(path, []byte(content), 0o644)
}

func (s *stubOrchestratorClient) RunAuditSynthesis(auditDir string, _ string) (string, error) {
	if s.synthesisErr != nil {
		return "", s.synthesisErr
	}
	path := filepath.Join(auditDir, "SYNTHESIS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	body := s.summaryBody
	if strings.TrimSpace(body) == "" {
		body = "# Summary\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (s *stubOrchestratorClient) PrepareWorkCycle() ([]orchestrator.WorktreeSession, error) {
	if s.prepareErr != nil {
		return nil, s.prepareErr
	}
	clone := make([]orchestrator.WorktreeSession, len(s.prepareSessions))
	copy(clone, s.prepareSessions)
	return clone, nil
}

func (s *stubOrchestratorClient) RunUpCycle(_ context.Context, _ []orchestrator.WorktreeSession) error {
	s.runUpCycleCalls++
	return s.RunErr
}

func newRefinementTestContext(t *testing.T) *module.ModuleContext {
	projectDir := t.TempDir()
	if err := config.InitLatticeDir(projectDir); err != nil {
		t.Fatalf("init lattice dir: %v", err)
	}
	cfg := &config.Config{
		ProjectDir:        projectDir,
		LatticeRoot:       projectDir,
		LatticeProjectDir: filepath.Join(projectDir, config.LatticeDir),
	}
	wf := workflow.New(cfg.LatticeProjectDir)
	if err := wf.Initialize(); err != nil {
		t.Fatalf("initialize workflow: %v", err)
	}
	return &module.ModuleContext{
		Config:    cfg,
		Workflow:  wf,
		Artifacts: artifact.NewStore(wf),
	}
}

func seedRefinementInputs(t *testing.T, ctx *module.ModuleContext) {
	writeJSONArtifact(t, ctx, artifact.WorkersJSON, []byte(`{"workers":[{"name":"Kai"}]}`))
	writeJSONArtifact(t, ctx, artifact.OrchestratorState, []byte(`{"name":"Aster"}`))
}

func writeJSONArtifact(t *testing.T, ctx *module.ModuleContext, ref artifact.ArtifactRef, body []byte) {
	meta := artifact.Metadata{
		ArtifactID: ref.ID,
		ModuleID:   "test",
		Version:    "0.0.1",
		Workflow:   ctx.Workflow.Dir(),
	}
	if err := ctx.Artifacts.Write(ref, body, meta); err != nil {
		t.Fatalf("write %s: %v", ref.ID, err)
	}
}

type stakeholdersManifestPayload struct {
	ProjectType string                    `json:"projectType"`
	Roles       map[string]map[string]any `json:"roles"`
}

func readStakeholdersManifest(t *testing.T, path string) stakeholdersManifestPayload {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stakeholders: %v", err)
	}
	var payload stakeholdersManifestPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode stakeholders: %v", err)
	}
	return payload
}

func ensureExists(t *testing.T, path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func ensureMissing(t *testing.T, path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected %s to be absent", path)
	}
}
