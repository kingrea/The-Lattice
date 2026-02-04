package refinement

import (
	"context"
	"fmt"

	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/orchestrator"
)

type orchestratorClient interface {
	LoadProjectAgents() ([]orchestrator.ProjectAgent, error)
	CurrentWorkerList() orchestrator.WorkerList
	RunStakeholderAudit(role string, agent orchestrator.ProjectAgent, path string, desc string) error
	RunAuditSynthesis(auditDir string, projectDesc string) (string, error)
	PrepareWorkCycle() ([]orchestrator.WorktreeSession, error)
	RunUpCycle(context.Context, []orchestrator.WorktreeSession) error
}

type orchestratorFactory func(*module.ModuleContext) (orchestratorClient, error)

type defaultOrchestratorClient struct {
	orch *orchestrator.Orchestrator
}

func defaultClientFactory(ctx *module.ModuleContext) (orchestratorClient, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%s: module context missing", moduleID)
	}
	if ctx.Orchestrator == nil {
		if ctx.Config == nil {
			return nil, fmt.Errorf("%s: config required to initialize orchestrator", moduleID)
		}
		ctx.Orchestrator = orchestrator.New(ctx.Config)
	}
	return defaultOrchestratorClient{orch: ctx.Orchestrator}, nil
}

func (c defaultOrchestratorClient) LoadProjectAgents() ([]orchestrator.ProjectAgent, error) {
	if c.orch == nil {
		return nil, fmt.Errorf("%s: orchestrator unavailable", moduleID)
	}
	return c.orch.LoadProjectAgents()
}

func (c defaultOrchestratorClient) CurrentWorkerList() orchestrator.WorkerList {
	if c.orch == nil {
		return orchestrator.WorkerList{}
	}
	return c.orch.CurrentWorkerList()
}

func (c defaultOrchestratorClient) RunStakeholderAudit(role string, agent orchestrator.ProjectAgent, path string, desc string) error {
	if c.orch == nil {
		return fmt.Errorf("%s: orchestrator unavailable", moduleID)
	}
	return c.orch.RunStakeholderAudit(role, agent, path, desc)
}

func (c defaultOrchestratorClient) RunAuditSynthesis(auditDir string, projectDesc string) (string, error) {
	if c.orch == nil {
		return "", fmt.Errorf("%s: orchestrator unavailable", moduleID)
	}
	return c.orch.RunAuditSynthesis(auditDir, projectDesc)
}

func (c defaultOrchestratorClient) PrepareWorkCycle() ([]orchestrator.WorktreeSession, error) {
	if c.orch == nil {
		return nil, fmt.Errorf("%s: orchestrator unavailable", moduleID)
	}
	return c.orch.PrepareWorkCycle()
}

func (c defaultOrchestratorClient) RunUpCycle(ctx context.Context, sessions []orchestrator.WorktreeSession) error {
	if c.orch == nil {
		return fmt.Errorf("%s: orchestrator unavailable", moduleID)
	}
	return c.orch.RunUpCycle(ctx, sessions)
}
