package resolver

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/workflow"
)

// NodeState represents the resolver's understanding of a module's readiness.
type NodeState string

const (
	NodeStateUnknown  NodeState = "unknown"
	NodeStatePending  NodeState = "pending"
	NodeStateReady    NodeState = "ready"
	NodeStateBlocked  NodeState = "blocked"
	NodeStateComplete NodeState = "complete"
	NodeStateError    NodeState = "error"
)

// Node captures a workflow module instance plus its dependency metadata.
type Node struct {
	ID           string
	Ref          workflow.ModuleRef
	Module       module.Module
	Dependencies []string
	Dependents   []string

	State     NodeState
	BlockedBy []string
	Err       error

	Artifacts    map[string]ArtifactReport
	fingerprints map[string]string
}

// ArtifactReport captures the resolver's understanding of an output artifact.
type ArtifactReport struct {
	Ref                 artifact.ArtifactRef
	Status              module.ArtifactStatus
	Metadata            *artifact.Metadata
	Err                 error
	StoredFingerprint   string
	ExpectedFingerprint string
}

// Resolver builds and evaluates the workflow dependency graph.
type Resolver struct {
	definition workflow.WorkflowDefinition
	nodes      map[string]*Node
	orderedIDs []string
}

// New constructs a resolver for the provided workflow definition. Modules are
// instantiated via the registry immediately so downstream code can run them.
func New(def workflow.WorkflowDefinition, registry *module.Registry) (*Resolver, error) {
	if registry == nil {
		return nil, fmt.Errorf("workflow: module registry is required")
	}
	normalized, err := def.Normalized()
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]*Node, len(normalized.Modules))
	ordered := make([]string, 0, len(normalized.Modules))
	for _, ref := range normalized.Modules {
		id := ref.InstanceID()
		mod, err := registry.Resolve(ref.ModuleID, convertConfig(ref.Config))
		if err != nil {
			return nil, fmt.Errorf("workflow %s module %s: %w", normalized.ID, id, err)
		}
		node := &Node{
			ID:           id,
			Ref:          ref,
			Module:       mod,
			Dependencies: normalized.Dependencies(id),
		}
		nodes[id] = node
		ordered = append(ordered, id)
	}
	for _, node := range nodes {
		for _, depID := range node.Dependencies {
			dep, ok := nodes[depID]
			if !ok {
				return nil, fmt.Errorf("workflow %s: dependency %s referenced by %s not declared", normalized.ID, depID, node.ID)
			}
			dep.Dependents = append(dep.Dependents, node.ID)
		}
	}
	for _, node := range nodes {
		if len(node.Dependents) > 1 {
			sort.Strings(node.Dependents)
		}
	}
	return &Resolver{
		definition: normalized,
		nodes:      nodes,
		orderedIDs: ordered,
	}, nil
}

// Definition returns a clone of the resolver's workflow definition.
func (r *Resolver) Definition() workflow.WorkflowDefinition {
	return r.definition.Clone()
}

// Nodes returns the nodes in workflow declaration order.
func (r *Resolver) Nodes() []*Node {
	out := make([]*Node, 0, len(r.orderedIDs))
	for _, id := range r.orderedIDs {
		if node, ok := r.nodes[id]; ok {
			out = append(out, node)
		}
	}
	return out
}

// Node retrieves a specific module node by workflow instance ID.
func (r *Resolver) Node(id string) (*Node, bool) {
	node, ok := r.nodes[id]
	return node, ok
}

// Refresh re-evaluates module completion status and dependency readiness using
// the provided module context. Callers should invoke Refresh before querying for
// runnable modules to ensure the snapshot reflects on-disk artifacts.
func (r *Resolver) Refresh(ctx *module.ModuleContext) error {
	if ctx == nil {
		return fmt.Errorf("workflow: module context is required")
	}
	for _, node := range r.nodes {
		node.Err = nil
		node.BlockedBy = nil
		node.Artifacts = nil
		node.fingerprints = nil
		node.State = NodeStateUnknown
		if fpProvider, ok := node.Module.(module.Fingerprinter); ok {
			fingerprints, err := fpProvider.ArtifactFingerprints(ctx)
			if err != nil {
				node.State = NodeStateError
				node.Err = fmt.Errorf("workflow: fingerprints for %s: %w", node.ID, err)
				continue
			}
			if len(fingerprints) > 0 {
				node.fingerprints = fingerprints
			}
		}
		complete, err := node.Module.IsComplete(ctx)
		if err != nil {
			node.State = NodeStateError
			node.Err = err
			continue
		}
		if complete {
			node.State = NodeStateComplete
		} else {
			node.State = NodeStatePending
		}
	}
	for _, node := range r.nodes {
		if node.State == NodeStateError {
			continue
		}
		r.refreshArtifacts(ctx, node)
		if node.State == NodeStateComplete && node.hasArtifactIssues() {
			node.State = NodeStatePending
		}
	}
	for _, node := range r.nodes {
		if node.State == NodeStateComplete || node.State == NodeStateError {
			continue
		}
		blockers := r.blockers(node)
		if len(blockers) == 0 {
			node.State = NodeStateReady
		} else {
			node.State = NodeStateBlocked
			node.BlockedBy = blockers
		}
	}
	return nil
}

// Ready returns nodes that are runnable because all dependencies are complete.
func (r *Resolver) Ready() []*Node {
	var ready []*Node
	for _, id := range r.orderedIDs {
		node := r.nodes[id]
		if node.State == NodeStateReady {
			ready = append(ready, node)
		}
	}
	return ready
}

// Queue returns modules that must run to satisfy the requested targets. If no
// targets are provided, every incomplete module is considered. Dependencies are
// returned before the modules that require them, and already-complete modules
// are skipped.
func (r *Resolver) Queue(targets ...string) ([]*Node, error) {
	if len(targets) == 0 {
		targets = append([]string{}, r.orderedIDs...)
	}
	visited := make(map[string]bool, len(targets))
	ordered := make([]*Node, 0, len(r.nodes))
	var visit func(string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		node, ok := r.nodes[id]
		if !ok {
			return fmt.Errorf("workflow: unknown module %s", id)
		}
		visited[id] = true
		for _, dep := range node.Dependencies {
			if err := visit(dep); err != nil {
				return err
			}
		}
		if node.State != NodeStateComplete {
			ordered = append(ordered, node)
		}
		return nil
	}
	for _, id := range targets {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func (r *Resolver) blockers(node *Node) []string {
	if len(node.Dependencies) == 0 {
		return nil
	}
	blockers := make([]string, 0, len(node.Dependencies))
	for _, depID := range node.Dependencies {
		dep, ok := r.nodes[depID]
		if !ok || dep.State != NodeStateComplete {
			blockers = append(blockers, depID)
		}
	}
	if len(blockers) == 0 {
		return nil
	}
	return blockers
}

func (r *Resolver) refreshArtifacts(ctx *module.ModuleContext, node *Node) {
	outputs := node.Module.Outputs()
	if len(outputs) == 0 {
		node.Artifacts = nil
		return
	}
	if node.Artifacts == nil {
		node.Artifacts = make(map[string]ArtifactReport, len(outputs))
	} else {
		for key := range node.Artifacts {
			delete(node.Artifacts, key)
		}
	}
	for _, ref := range outputs {
		report := r.CheckArtifact(ctx, node, ref)
		node.Artifacts[ref.ID] = report
	}
}

func (n *Node) hasArtifactIssues() bool {
	if len(n.Artifacts) == 0 {
		return false
	}
	for _, report := range n.Artifacts {
		switch report.Status {
		case module.ArtifactStatusFresh, module.ArtifactStatusReady:
			continue
		default:
			return true
		}
	}
	return false
}

// CheckArtifact evaluates a single artifact and returns its resolver status.
func (r *Resolver) CheckArtifact(ctx *module.ModuleContext, node *Node, ref artifact.ArtifactRef) ArtifactReport {
	report := ArtifactReport{Ref: ref, Status: module.ArtifactStatusUnknown}
	if ctx == nil || ctx.Artifacts == nil {
		report.Status = module.ArtifactStatusError
		report.Err = fmt.Errorf("workflow: artifact store unavailable")
		r.emitInvalidation(ctx, node, report, module.InvalidationReasonCheckError)
		return report
	}
	result, err := ctx.Artifacts.Check(ref)
	report.Metadata = result.Metadata
	if err != nil {
		report.Err = err
	}
	switch result.State {
	case artifact.StateMissing:
		report.Status = module.ArtifactStatusMissing
		r.emitInvalidation(ctx, node, report, module.InvalidationReasonMissing)
	case artifact.StateInvalid:
		if report.Err == nil {
			report.Err = result.Err
		}
		report.Status = module.ArtifactStatusInvalid
		r.emitInvalidation(ctx, node, report, module.InvalidationReasonInvalidMetadata)
	case artifact.StateError:
		if report.Err == nil {
			report.Err = result.Err
		}
		if report.Err == nil {
			report.Err = fmt.Errorf("workflow: %s encountered an unknown error", ref.ID)
		}
		report.Status = module.ArtifactStatusError
		r.emitInvalidation(ctx, node, report, module.InvalidationReasonCheckError)
	case artifact.StateReady:
		info := node.Module.Info()
		meta := result.Metadata
		if meta == nil {
			report.Status = module.ArtifactStatusInvalid
			report.Err = fmt.Errorf("workflow: %s missing metadata", ref.ID)
			r.emitInvalidation(ctx, node, report, module.InvalidationReasonInvalidMetadata)
			break
		}
		if meta.ModuleID != info.ID {
			report.Status = module.ArtifactStatusInvalid
			report.Err = fmt.Errorf("workflow: %s created by %s expected %s", ref.ID, meta.ModuleID, info.ID)
			r.emitInvalidation(ctx, node, report, module.InvalidationReasonInvalidMetadata)
			break
		}
		if meta.Version != info.Version {
			report.Status = module.ArtifactStatusOutdated
			r.emitInvalidation(ctx, node, report, module.InvalidationReasonVersionMismatch)
			break
		}
		expected, hasExpected, fpErr := r.expectedFingerprint(ctx, node, ref)
		if fpErr != nil {
			report.Status = module.ArtifactStatusError
			report.Err = fpErr
			r.emitInvalidation(ctx, node, report, module.InvalidationReasonCheckError)
			break
		}
		if !hasExpected {
			report.Status = module.ArtifactStatusReady
			break
		}
		stored := fingerprintFromMetadata(meta, ref.ID)
		report.ExpectedFingerprint = expected
		report.StoredFingerprint = stored
		if strings.TrimSpace(stored) == "" {
			report.Status = module.ArtifactStatusReady
			break
		}
		if stored != expected {
			report.Status = module.ArtifactStatusOutdated
			r.emitInvalidation(ctx, node, report, module.InvalidationReasonFingerprint)
			break
		}
		report.Status = module.ArtifactStatusFresh
	default:
		report.Status = module.ArtifactStatusUnknown
	}
	return report
}

func (r *Resolver) expectedFingerprint(ctx *module.ModuleContext, node *Node, ref artifact.ArtifactRef) (string, bool, error) {
	if node == nil {
		return "", false, nil
	}
	if node.fingerprints == nil {
		provider, ok := node.Module.(module.Fingerprinter)
		if !ok {
			return "", false, nil
		}
		fingerprints, err := provider.ArtifactFingerprints(ctx)
		if err != nil {
			return "", false, err
		}
		node.fingerprints = fingerprints
	}
	value, ok := node.fingerprints[ref.ID]
	if !ok || strings.TrimSpace(value) == "" {
		return "", false, nil
	}
	return value, true, nil
}

func fingerprintFromMetadata(meta *artifact.Metadata, artifactID string) string {
	if meta == nil || len(meta.Notes) == 0 {
		return ""
	}
	return meta.Notes[module.FingerprintNoteKey(artifactID)]
}

func (r *Resolver) emitInvalidation(ctx *module.ModuleContext, node *Node, report ArtifactReport, reason module.ArtifactInvalidationReason) {
	handler, ok := node.Module.(module.ArtifactInvalidationHandler)
	if !ok {
		return
	}
	event := module.ArtifactInvalidation{
		Artifact:            report.Ref,
		Status:              report.Status,
		Reason:              reason,
		StoredFingerprint:   report.StoredFingerprint,
		ExpectedFingerprint: report.ExpectedFingerprint,
		Metadata:            report.Metadata,
		Err:                 report.Err,
	}
	if err := handler.OnArtifactInvalidation(ctx, event); err != nil {
		node.State = NodeStateError
		node.Err = err
	}
}

func convertConfig(cfg workflow.ModuleConfig) module.Config {
	if len(cfg) == 0 {
		return nil
	}
	out := make(module.Config, len(cfg))
	for key, value := range cfg {
		out[key] = value
	}
	return out
}
