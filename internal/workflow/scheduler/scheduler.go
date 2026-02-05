package scheduler

import (
	"fmt"

	"github.com/kingrea/The-Lattice/internal/workflow/resolver"
)

// Selector exposes the minimal contract the workflow engine needs to request
// runnable module batches.
type Selector interface {
	Runnable(RunnableRequest) (RunnableBatch, error)
}

// Scheduler implements Selector on top of a dependency resolver. It examines
// the resolved queue, filters nodes that are truly runnable, and enforces any
// configured constraints.
type Scheduler struct {
	resolver *resolver.Resolver
}

// New wires a Scheduler to a resolver snapshot.
func New(res *resolver.Resolver) (*Scheduler, error) {
	if res == nil {
		return nil, fmt.Errorf("workflow: scheduler requires a resolver")
	}
	return &Scheduler{resolver: res}, nil
}

// RunnableRequest captures the current runtime state plus any scheduling
// constraints. The Scheduler produces batches that satisfy these constraints.
type RunnableRequest struct {
	// Targets optionally narrows scheduling to a subset of workflow nodes. When
	// empty, every incomplete module is considered.
	Targets []string
	// BatchSize limits how many runnable nodes are returned at once. Values <= 0
	// are treated as "no limit" (subject to MaxParallel enforcement).
	BatchSize int
	// MaxParallel caps how many modules may be active at once, including the
	// modules listed in Running. Values <= 0 disable the limit.
	MaxParallel int
	// Running should list module instance IDs that are currently executing so the
	// scheduler won't dispatch them twice.
	Running []string
	// ManualGates describes whether a module requires manual approval and the
	// approval status.
	ManualGates map[string]ManualGateState
}

// ManualGateState records whether a manual approval is required before a module
// may run.
type ManualGateState struct {
	Required bool
	Approved bool
	Note     string
}

// RunnableBatch describes the scheduler's decision.
type RunnableBatch struct {
	Nodes   []*resolver.Node
	Skipped map[string]SkipReason
}

// SkipReason explains why a node was excluded from the runnable set.
type SkipReason struct {
	Reason SkipReasonCode
	Detail string
}

// SkipReasonCode enumerates scheduler skip reasons.
type SkipReasonCode string

const (
	SkipReasonNotReady    SkipReasonCode = "not-ready"
	SkipReasonManualGate  SkipReasonCode = "manual-gate"
	SkipReasonConcurrency SkipReasonCode = "concurrency"
	SkipReasonActive      SkipReasonCode = "already-running"
)

// Runnable returns a batch of runnable nodes constrained by the request.
func (s *Scheduler) Runnable(req RunnableRequest) (RunnableBatch, error) {
	queue, err := s.resolver.Queue(req.Targets...)
	if err != nil {
		return RunnableBatch{}, err
	}
	rq := newRunnableQueue(queue)
	running := req.runningSet()
	manual := req.manualGateSet()
	inventory := s.concurrencyInventory(running)
	result := RunnableBatch{}
	if req.MaxParallel > 0 && inventory.slots >= req.MaxParallel {
		s.recordConcurrencySkip(&result, fmt.Sprintf("max parallel %d reached", req.MaxParallel))
		return result, nil
	}
	if inventory.exclusiveID != "" {
		s.recordConcurrencySkip(&result, fmt.Sprintf("%s requires exclusive execution", inventory.exclusiveID))
		return result, nil
	}
	maxBatch := req.batchNodeLimit(rq.Len())
	batchSlots := 0
	for rq.Len() > 0 {
		node := rq.Pop()
		if node == nil {
			break
		}
		if _, runningAlready := running[node.ID]; runningAlready {
			result.addSkip(node.ID, SkipReason{Reason: SkipReasonActive, Detail: "module already running"})
			continue
		}
		if node.State != resolver.NodeStateReady {
			result.addSkip(node.ID, SkipReason{Reason: SkipReasonNotReady, Detail: string(node.State)})
			continue
		}
		if gate, ok := manual[node.ID]; ok && gate.Required && !gate.Approved {
			note := gate.Note
			if note == "" {
				note = "awaiting manual approval"
			}
			result.addSkip(node.ID, SkipReason{Reason: SkipReasonManualGate, Detail: note})
			continue
		}
		nodeSlots := nodeSlotCost(node)
		nodeExclusive := nodeRequiresExclusive(node)
		if nodeExclusive && (inventory.slots > 0 || batchSlots > 0) {
			result.addSkip(node.ID, SkipReason{Reason: SkipReasonConcurrency, Detail: "requires exclusive execution"})
			continue
		}
		if req.MaxParallel > 0 && inventory.slots+batchSlots+nodeSlots > req.MaxParallel {
			result.addSkip(node.ID, SkipReason{Reason: SkipReasonConcurrency, Detail: fmt.Sprintf("max parallel %d reached", req.MaxParallel)})
			continue
		}
		result.Nodes = append(result.Nodes, node)
		batchSlots += nodeSlots
		if nodeExclusive {
			break
		}
		if maxBatch > 0 && len(result.Nodes) >= maxBatch {
			break
		}
	}
	return result, nil
}

func (req RunnableRequest) runningSet() map[string]struct{} {
	if len(req.Running) == 0 {
		return map[string]struct{}{}
	}
	set := make(map[string]struct{}, len(req.Running))
	for _, id := range req.Running {
		if id == "" {
			continue
		}
		set[id] = struct{}{}
	}
	return set
}

func (req RunnableRequest) manualGateSet() map[string]ManualGateState {
	if len(req.ManualGates) == 0 {
		return map[string]ManualGateState{}
	}
	set := make(map[string]ManualGateState, len(req.ManualGates))
	for id, state := range req.ManualGates {
		if id == "" {
			continue
		}
		set[id] = state
	}
	return set
}

func (req RunnableRequest) batchNodeLimit(queueLen int) int {
	if queueLen <= 0 {
		return 0
	}
	if req.BatchSize <= 0 || req.BatchSize > queueLen {
		return queueLen
	}
	return req.BatchSize
}

func (b *RunnableBatch) addSkip(id string, reason SkipReason) {
	if id == "" {
		return
	}
	if b.Skipped == nil {
		b.Skipped = make(map[string]SkipReason)
	}
	b.Skipped[id] = reason
}

type runnableQueue struct {
	nodes []*resolver.Node
}

func newRunnableQueue(nodes []*resolver.Node) *runnableQueue {
	if len(nodes) == 0 {
		return &runnableQueue{}
	}
	copyNodes := make([]*resolver.Node, len(nodes))
	copy(copyNodes, nodes)
	return &runnableQueue{nodes: copyNodes}
}

func (q *runnableQueue) Len() int {
	return len(q.nodes)
}

func (q *runnableQueue) Pop() *resolver.Node {
	if len(q.nodes) == 0 {
		return nil
	}
	node := q.nodes[0]
	q.nodes = q.nodes[1:]
	return node
}

type runningInventory struct {
	slots       int
	exclusiveID string
}

func (s *Scheduler) concurrencyInventory(running map[string]struct{}) runningInventory {
	inv := runningInventory{}
	if len(running) == 0 {
		return inv
	}
	for id := range running {
		node, _ := s.resolver.Node(id)
		slots := nodeSlotCost(node)
		inv.slots += slots
		if inv.exclusiveID == "" && nodeRequiresExclusive(node) {
			inv.exclusiveID = id
		}
	}
	return inv
}

func nodeSlotCost(node *resolver.Node) int {
	if node == nil {
		return 1
	}
	return node.Module.Info().SlotCost()
}

func nodeRequiresExclusive(node *resolver.Node) bool {
	if node == nil {
		return false
	}
	return node.Module.Info().RequiresExclusiveExecution()
}

func (s *Scheduler) recordConcurrencySkip(batch *RunnableBatch, detail string) {
	if batch == nil {
		return
	}
	ready := s.resolver.Ready()
	if len(ready) == 0 {
		return
	}
	batch.addSkip(ready[0].ID, SkipReason{Reason: SkipReasonConcurrency, Detail: detail})
}
