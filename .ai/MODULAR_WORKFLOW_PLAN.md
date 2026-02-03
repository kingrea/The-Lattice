# Modular Workflow Architecture Plan

## Executive Summary

This document outlines a refactoring plan to transform the current rigid, linear workflow system into a modular, composable architecture where:

1. **Modes become Workflows** - High-level orchestrations that compose modules
2. **Stages become Modules** - Self-contained units with explicit inputs/outputs
3. **Files have Provenance** - Frontmatter tracks which module created each artifact
4. **Dependencies are Declarative** - Modules declare what they need; missing dependencies trigger their creation
5. **Everything is Pluggable** - Custom modules can replace built-in ones

---

## Current Architecture Analysis

### What Works Well

| Pattern | Benefit |
|---------|---------|
| `Mode` interface | Clean abstraction for workflow stages |
| File-based state | Git-trackable, crash-recoverable |
| Marker files | Simple phase detection |
| `tea.Cmd` async pattern | Non-blocking TUI |
| Polling for completion | Works across platforms |

### What's Rigid

| Issue | Impact |
|-------|--------|
| Hard-coded phase sequence | Can't easily create different workflows |
| Monolithic modes (Planning has 7 substeps) | Can't reuse individual substeps |
| No input/output contracts | Implicit dependencies between phases |
| No artifact provenance | Can't know which module created a file |
| Phase detection is file-path based | Brittle to changes |

---

## Proposed Architecture

### Core Concepts

```
┌─────────────────────────────────────────────────────────────────┐
│                          WORKFLOW                               │
│  "Commission Work", "Quick Start", "Resume from Checkpoint"     │
│  Composes modules into a directed execution graph               │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                          MODULES                                │
│  Self-contained units with declared inputs and outputs          │
│  Examples: "anchor-docs", "action-plan", "staff-review"         │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                         ARTIFACTS                               │
│  Files with frontmatter indicating source module and metadata   │
│  Examples: COMMISSION.md, ARCHITECTURE.md, workers.json         │
└─────────────────────────────────────────────────────────────────┘
```

### 1. Module Definition

A **Module** is a self-contained unit of work that:
- Declares required inputs (files or other artifacts)
- Declares outputs it produces
- Can be run independently if inputs are satisfied
- Knows how to check if it's already complete

```go
// internal/module/module.go

type Module interface {
    // Identity
    ID() string           // Unique identifier, e.g., "anchor-docs"
    Name() string         // Display name, e.g., "Create Anchor Documents"
    Description() string  // What this module does

    // Contracts
    Inputs() []ArtifactRef   // What this module needs to run
    Outputs() []ArtifactRef  // What this module produces

    // Execution
    IsComplete(ctx *ModuleContext) bool
    Run(ctx *ModuleContext) tea.Cmd

    // TUI (optional - for modules that need user interaction)
    Update(msg tea.Msg) (Module, tea.Cmd)
    View() string
}

type ArtifactRef struct {
    ID       string       // e.g., "commission-doc"
    Path     string       // e.g., ".lattice/plan/COMMISSION.md"
    Type     ArtifactType // File, Directory, Marker
    Required bool         // Is this mandatory?
}

type ArtifactType int
const (
    ArtifactFile ArtifactType = iota
    ArtifactDirectory
    ArtifactMarker  // Empty marker file
    ArtifactJSON    // JSON with schema
)
```

### 2. Artifact Frontmatter

Every file created by a module includes YAML frontmatter:

```markdown
---
lattice:
  module: anchor-docs
  version: 1
  created: 2024-01-15T10:30:00Z
  workflow: commission-work
  inputs:
    - user-requirements  # What was available when this was created
  checksum: sha256:abc123...
---

# Commission: Project Title

[Actual content...]
```

For JSON files, use a `_lattice` metadata key:

```json
{
  "_lattice": {
    "module": "hiring",
    "version": 1,
    "created": "2024-01-15T10:30:00Z",
    "workflow": "commission-work"
  },
  "workers": [...]
}
```

### 3. Workflow Definition

A **Workflow** composes modules into an execution graph:

```go
// internal/workflow/definition.go

type WorkflowDefinition struct {
    ID          string                  // e.g., "commission-work"
    Name        string                  // e.g., "Commission Work"
    Description string
    Modules     []ModuleRef             // Ordered list of modules
    Graph       map[string][]string     // Module ID -> dependencies
}

type ModuleRef struct {
    ModuleID    string            // Which module to run
    Config      map[string]any    // Module-specific config overrides
    Optional    bool              // Can be skipped if inputs missing
}
```

**Example Workflow Definition (YAML for clarity, would be Go struct):**

```yaml
id: commission-work
name: Commission Work
description: Full planning and execution workflow

modules:
  - id: anchor-docs
    name: Create Anchor Documents

  - id: action-plan
    depends_on: [anchor-docs]

  - id: staff-review
    depends_on: [action-plan]

  - id: parallel-reviews
    depends_on: [staff-review]
    config:
      reviewers: [pragmatist, simplifier, advocate, skeptic]

  - id: consolidation
    depends_on: [parallel-reviews]

  - id: bead-creation
    depends_on: [consolidation]

  - id: orchestrator-selection
    depends_on: [bead-creation]

  - id: hiring
    depends_on: [orchestrator-selection]

  - id: work-process
    depends_on: [hiring]

  - id: refinement
    depends_on: [work-process]
    optional: true

  - id: release
    depends_on: [work-process]
```

### 4. Dependency Resolution

When a workflow starts or resumes:

```go
// internal/workflow/resolver.go

type Resolver struct {
    registry *ModuleRegistry
    artifacts *ArtifactStore
}

func (r *Resolver) ResolveNextModules(def *WorkflowDefinition) ([]Module, error) {
    // 1. Build dependency graph
    // 2. Find modules with all inputs satisfied
    // 3. Filter out completed modules
    // 4. Return runnable modules (could be parallel)

    // If a module's input is missing:
    // - Find which module produces that output
    // - Add that module to the run queue first
}

func (r *Resolver) CheckArtifact(ref ArtifactRef) ArtifactStatus {
    // Check if file exists
    // If exists, parse frontmatter
    // Verify module provenance matches expected
    // Return: Missing, Valid, Invalid, Outdated
}
```

### 5. Module Registry

Modules are registered at startup:

```go
// internal/module/registry.go

type Registry struct {
    modules map[string]ModuleFactory
}

type ModuleFactory func(config map[string]any) Module

func (r *Registry) Register(id string, factory ModuleFactory) {
    r.modules[id] = factory
}

func (r *Registry) Get(id string, config map[string]any) (Module, error) {
    factory, ok := r.modules[id]
    if !ok {
        return nil, fmt.Errorf("unknown module: %s", id)
    }
    return factory(config), nil
}
```

### 6. Plugin System (Future)

For custom modules:

```go
// Modules can be loaded from:
// 1. Built-in (compiled into binary)
// 2. .lattice/modules/*.go (interpreted via yaegi or similar)
// 3. .lattice/modules/*.yaml (declarative skill-based modules)

type PluginModule struct {
    definition ModuleDefinition  // From YAML
    skill      string            // Skill to execute
}
```

---

## Migration Strategy

### Phase 1: Extract Modules from Planning Mode

The current Planning mode has 7 substeps. Extract each as a module:

| Current Substep | New Module ID | Inputs | Outputs |
|-----------------|---------------|--------|---------|
| phaseAnchorDocs | `anchor-docs` | user context | COMMISSION.md, ARCHITECTURE.md, CONVENTIONS.md |
| phaseActionPlan | `action-plan` | anchor docs | MODULES.md, PLAN.md |
| phaseStaffReview | `staff-review` | action plan | STAFF_REVIEW.md |
| phaseStaffIncorporation | `staff-incorporate` | staff review | updated MODULES.md, PLAN.md |
| phaseParallelReviews | `parallel-reviews` | updated plan | 4 review files |
| phaseConsolidation | `consolidation` | reviews | .reviews-applied marker |
| phaseBeadCreation | `bead-creation` | consolidation | .beads-created marker |

### Phase 2: Define Artifact Contracts

Create explicit types for each artifact:

```go
// internal/artifact/types.go

var CommissionDoc = ArtifactRef{
    ID:   "commission-doc",
    Path: ".lattice/plan/COMMISSION.md",
    Type: ArtifactFile,
}

var ArchitectureDoc = ArtifactRef{
    ID:   "architecture-doc",
    Path: ".lattice/plan/ARCHITECTURE.md",
    Type: ArtifactFile,
}

// ... etc
```

### Phase 3: Add Frontmatter Support

```go
// internal/artifact/frontmatter.go

type LatticeMeta struct {
    Module    string    `yaml:"module"`
    Version   int       `yaml:"version"`
    Created   time.Time `yaml:"created"`
    Workflow  string    `yaml:"workflow"`
    Inputs    []string  `yaml:"inputs,omitempty"`
    Checksum  string    `yaml:"checksum,omitempty"`
}

func ParseFrontmatter(content []byte) (*LatticeMeta, []byte, error)
func WriteFrontmatter(meta *LatticeMeta, content []byte) []byte
```

### Phase 4: Implement Workflow Engine

```go
// internal/workflow/engine.go

type Engine struct {
    registry   *ModuleRegistry
    resolver   *Resolver
    artifacts  *ArtifactStore
    current    *WorkflowRun
}

func (e *Engine) Start(def *WorkflowDefinition) tea.Cmd
func (e *Engine) Resume() tea.Cmd
func (e *Engine) Update(msg tea.Msg) tea.Cmd
func (e *Engine) View() string
```

### Phase 5: Create Alternative Workflows

Once modular, create new workflows:

```go
// Quick Start - Skip reviews, minimal planning
var QuickStartWorkflow = WorkflowDefinition{
    ID: "quick-start",
    Modules: []ModuleRef{
        {ModuleID: "anchor-docs"},
        {ModuleID: "action-plan"},
        {ModuleID: "bead-creation"},
        {ModuleID: "orchestrator-selection"},
        {ModuleID: "hiring"},
        {ModuleID: "work-process"},
    },
}

// Solo Mode - Single agent, no hiring
var SoloWorkflow = WorkflowDefinition{
    ID: "solo",
    Modules: []ModuleRef{
        {ModuleID: "anchor-docs"},
        {ModuleID: "action-plan"},
        {ModuleID: "solo-work"},  // New module for single-agent work
    },
}
```

---

## Detailed Module Design

### Example: anchor-docs Module

```go
// internal/modules/anchor_docs/module.go

type AnchorDocsModule struct {
    ctx    *ModuleContext
    phase  anchorPhase
    window string
}

func (m *AnchorDocsModule) ID() string { return "anchor-docs" }
func (m *AnchorDocsModule) Name() string { return "Create Anchor Documents" }

func (m *AnchorDocsModule) Inputs() []ArtifactRef {
    return []ArtifactRef{} // No file inputs, just user context
}

func (m *AnchorDocsModule) Outputs() []ArtifactRef {
    return []ArtifactRef{
        artifact.CommissionDoc,
        artifact.ArchitectureDoc,
        artifact.ConventionsDoc,
    }
}

func (m *AnchorDocsModule) IsComplete(ctx *ModuleContext) bool {
    for _, out := range m.Outputs() {
        status := ctx.Artifacts.Check(out)
        if status != ArtifactValid {
            return false
        }
    }
    return true
}

func (m *AnchorDocsModule) Run(ctx *ModuleContext) tea.Cmd {
    m.ctx = ctx
    return m.startSkillSession()
}

func (m *AnchorDocsModule) startSkillSession() tea.Cmd {
    return func() tea.Msg {
        skillPath, _ := skills.Ensure("lattice-planning", m.ctx.Config)
        m.window = createTmuxWindow("anchor-docs")
        runOpenCode(skillPath, m.window)
        return pollTickMsg{}
    }
}
```

### Example: hiring Module

```go
// internal/modules/hiring/module.go

func (m *HiringModule) Inputs() []ArtifactRef {
    return []ArtifactRef{
        artifact.OrchestratorJSON,
        artifact.ModulesDoc,
        artifact.ActionPlanDoc,
        artifact.BeadsCreatedMarker,
    }
}

func (m *HiringModule) Outputs() []ArtifactRef {
    return []ArtifactRef{
        artifact.WorkersJSON,
        // Dynamic: also creates AGENT.md files for each worker
    }
}

func (m *HiringModule) Run(ctx *ModuleContext) tea.Cmd {
    // Check all inputs exist
    for _, input := range m.Inputs() {
        status := ctx.Artifacts.Check(input)
        if status != ArtifactValid {
            return func() tea.Msg {
                return ModuleMissingInputMsg{
                    Module: m.ID(),
                    Missing: input,
                }
            }
        }
    }

    return m.startHiring()
}
```

---

## File Structure After Refactor

```
lattice-cli/
├── cmd/
│   └── lattice/
│       └── main.go
├── internal/
│   ├── tui/
│   │   └── app.go                    # Simplified, delegates to engine
│   ├── module/
│   │   ├── module.go                 # Module interface
│   │   ├── registry.go               # Module registry
│   │   ├── context.go                # ModuleContext
│   │   └── base.go                   # BaseModule helper
│   ├── modules/                      # Built-in modules
│   │   ├── anchor_docs/
│   │   │   └── module.go
│   │   ├── action_plan/
│   │   │   └── module.go
│   │   ├── staff_review/
│   │   │   └── module.go
│   │   ├── parallel_reviews/
│   │   │   └── module.go
│   │   ├── consolidation/
│   │   │   └── module.go
│   │   ├── bead_creation/
│   │   │   └── module.go
│   │   ├── orchestrator_selection/
│   │   │   └── module.go
│   │   ├── hiring/
│   │   │   └── module.go
│   │   ├── work_process/
│   │   │   └── module.go
│   │   ├── refinement/
│   │   │   └── module.go
│   │   └── release/
│   │       └── module.go
│   ├── workflow/
│   │   ├── definition.go             # WorkflowDefinition
│   │   ├── engine.go                 # Workflow execution engine
│   │   ├── resolver.go               # Dependency resolution
│   │   └── builtin.go                # Built-in workflow definitions
│   ├── artifact/
│   │   ├── types.go                  # ArtifactRef definitions
│   │   ├── store.go                  # ArtifactStore
│   │   ├── frontmatter.go            # Parse/write frontmatter
│   │   └── validator.go              # Validate artifact integrity
│   ├── orchestrator/                 # Existing, mostly unchanged
│   ├── config/                       # Existing
│   ├── skills/                       # Existing
│   └── logbook/                      # Existing
└── workflows/                        # YAML workflow definitions (optional)
    ├── commission-work.yaml
    ├── quick-start.yaml
    └── solo.yaml
```

---

## Key Design Decisions

### 1. Module Granularity

**Decision**: Each "stage" in the current planning mode becomes its own module.

**Rationale**:
- Maximum reusability
- Clear input/output contracts
- Can skip/replace individual stages

**Trade-off**: More files, more wiring

### 2. Frontmatter Format

**Decision**: YAML frontmatter in markdown, `_lattice` key in JSON.

**Rationale**:
- Industry standard (Jekyll, Hugo, etc.)
- Easy to parse
- Human readable
- Doesn't interfere with content

### 3. Dependency Resolution Strategy

**Decision**: Eager resolution with automatic module invocation.

**Rationale**:
- If module A needs file X, and X is missing, find module B that produces X and run it first
- Enables self-healing workflows
- Reduces manual intervention

**Alternative considered**: Fail-fast with manual retry

### 4. Plugin Loading

**Decision**: Start with Go-only modules, add YAML-based skill modules later.

**Rationale**:
- Go modules are fast and type-safe
- YAML modules enable non-programmers to create workflows
- Can be added incrementally

### 5. State Storage

**Decision**: Keep file-based state, add frontmatter for provenance.

**Rationale**:
- Maintains git-trackability
- Crash recovery still works
- Frontmatter adds missing metadata

---

## Implementation Roadmap

### Milestone 1: Foundation (Core Abstractions)

- [ ] Define `Module` interface
- [ ] Define `ArtifactRef` and `ArtifactStore`
- [ ] Implement frontmatter parsing/writing
- [ ] Create `ModuleRegistry`
- [ ] Create `ModuleContext`

### Milestone 2: Extract First Module

- [ ] Extract `anchor-docs` module from Planning mode
- [ ] Test running it standalone
- [ ] Verify frontmatter is written correctly

### Milestone 3: Extract Remaining Planning Modules

- [ ] `action-plan`
- [ ] `staff-review`
- [ ] `staff-incorporate`
- [ ] `parallel-reviews`
- [ ] `consolidation`
- [ ] `bead-creation`

### Milestone 4: Workflow Engine

- [ ] Implement `WorkflowDefinition` parser
- [ ] Implement `Resolver` for dependency resolution
- [ ] Implement `Engine` for execution
- [ ] Integrate with TUI

### Milestone 5: Convert Remaining Modes

- [ ] `orchestrator-selection`
- [ ] `hiring`
- [ ] `work-process`
- [ ] `refinement`
- [ ] `release`

### Milestone 6: Alternative Workflows

- [ ] Define `quick-start` workflow
- [ ] Define `solo` workflow
- [ ] Add workflow selection to main menu

### Milestone 7: Plugin System

- [ ] Design plugin API
- [ ] Implement YAML-based module definitions
- [ ] Add plugin discovery from `.lattice/modules/`

---

## Open Questions

1. **Module versioning**: How to handle module version upgrades that change output format?
   - Suggestion: Version in frontmatter, migration functions in modules

2. **Parallel module execution**: Should the engine run independent modules in parallel?
   - Suggestion: Yes, if dependencies allow (already do this for parallel-reviews)

3. **Module configuration**: How granular should config overrides be?
   - Suggestion: Start simple (map[string]any), add typed config later

4. **Error recovery**: If a module fails mid-execution, how to resume?
   - Suggestion: Modules are atomic; re-run from start if incomplete

5. **Artifact invalidation**: If an input changes, should downstream outputs be invalidated?
   - Suggestion: Track input checksums in frontmatter, warn on mismatch

---

## Feasibility Assessment

**Is this doable for a web developer new to Go?**

Yes, with these considerations:

1. **Go is straightforward**: The patterns used (interfaces, structs, methods) are similar to TypeScript interfaces and classes.

2. **Bubbletea is well-documented**: The Elm Architecture (Model-Update-View) is familiar to anyone who's used React/Redux.

3. **Start small**: Extract one module first (anchor-docs), get it working, then iterate.

4. **Existing code is a guide**: The current Mode interface is very close to what Module needs.

5. **File operations are simple**: Go's `os` and `path/filepath` packages are intuitive.

**Estimated complexity by milestone**:
- Milestone 1: Medium (new abstractions)
- Milestone 2: Low (pattern established)
- Milestone 3: Low (repetitive extraction)
- Milestone 4: Medium (graph traversal)
- Milestone 5: Low (apply patterns)
- Milestone 6: Low (composition)
- Milestone 7: High (plugin system)

---

## Summary

This architecture transforms the rigid linear workflow into a composable system where:

- **Modules** are self-contained with explicit contracts
- **Workflows** compose modules into execution graphs
- **Artifacts** track their provenance via frontmatter
- **Dependencies** are resolved automatically
- **Plugins** allow customization without modifying core code

The migration can be done incrementally, starting with extracting modules from the existing Planning mode while keeping the system functional throughout.
