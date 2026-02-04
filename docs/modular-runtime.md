## Modular Runtime Primer

This document clarifies how the upcoming module system fits alongside the
existing Bubble Tea modes. The two abstractions solve different problems, and
the runtime needs both to stay healthy.

### Why add modules?

- Modes are great at UI orchestration but they accumulate every concern:
  dependency wiring, artifact validation, retry policy, and agent prompts.
- The same logic is re‑implemented by every mode (planning, refinement, work,
  cleanup, ...), making it hard to reuse successful patterns across phases.
- Downstream epics (`lattice-8hm`, `lattice-z47`) need a portable execution
  substrate that can run outside the Bubble Tea event loop (CLI automation,
  headless runs, tests).

Modules give us a composable runtime where each unit owns a single artifact
contract ("produce MODULES.md from anchor docs", "render PLAN.md"). Modes stay
focused on guiding the human through the phase.

### Responsibilities at a glance

| Concern           | Mode (Bubble Tea)                             | Module (Runtime)                               |
| ----------------- | --------------------------------------------- | ---------------------------------------------- |
| Primary goal      | Drive the user through a workflow phase       | Produce or transform versioned artifacts       |
| IO surface        | Screen, keyboard, status LEDs, notifications  | Files in `.lattice/`, ArtifactStore helpers    |
| Long‑running work | Spawns tmux/OpenCode sessions, monitors logs  | Executes a single deterministic routine        |
| Error handling    | Surfaces actionable messages to the operator  | Returns typed errors + status codes            |
| Progress          | Tracks milestones, transitions workflow.Phase | Emits structured progress events               |
| Orchestration     | Decides which module(s) to run and when       | Declares dependencies on artifacts only        |
| State             | Holds UI state and mode configuration         | Stateless aside from ModuleContext + artifacts |

### Practical split

**Modes**

- Remain Bubble Tea models (`internal/modes/*.go`).
- Hold onto `ModeContext` (config, workflow paths, orchestrator handle).
- Decide if/when a module must run. They prepare prompts, gather user input,
  start/stop tmux windows, and display progress.

**Modules**

- New Go interfaces under `internal/modules/`.
- Do one thing: read known artifacts, write exactly one artifact, and report
  status.
- Have no TUI concerns; they can execute inside tests or background workers.
- Depend only on `ModuleContext`, `ArtifactStore`, and other modules' outputs.

### Configuration overrides

Workflow operators can tune module behavior without recompiling Go code. The
runtime recognises three override channels, applied in the following order:

1. **Environment variables** feed into `config.Config` and the orchestrator.
   Core toggles include `LATTICE_ROOT` (required so modules can resolve skills
   and defaults), `LATTICE_PLUGIN_AUTO_INSTALL` (controls automatic installation
   of the `opencode-worktree` plugin), and `LATTICE_ASSIGN_SPARK` (opt-in spark
   assignments when building work cycles). Modules can read these through
   `ModuleContext.Config` or directly from the environment.
2. **Config files** describe persistent workflow and module intent. Project
   config (`.lattice/config.yaml`) sets the default workflow ID and community
   sources, while workflow definitions (`workflows/<id>.yaml`) attach a `config`
   map to each `ModuleRef`. During execution the engine stores that map on the
   node, and both the TUI (`convertModuleConfig`) and resolver pass it to
   `module.Registry.Resolve` as a `module.Config`.
3. **CLI flags/runtime inputs** sit on top for ad-hoc changes. The TUI exposes
   `engine.RuntimeOverrides` (targets, manual gates, max parallelism) and the
   `module-runner` binary accepts `--config-file` (YAML/JSON map) plus repeated
   `--set key=value` pairs. `module-runner` builds a `module.Config` from those
   flags and hands it to the resolved module factory, so operators can toggle
   reviewers, prompts, etc. without editing workflow YAML.

Precedence flows upward: module defaults < workflow `ModuleRef.Config` <
`module-runner` overrides (file first, then inline `--set`). Validation happens
when the workflow definition is normalized and when the CLI parses override
flags (bad files or malformed `key=value` pairs fail before modules run). Once a
module is instantiated, the config map is immutable for the run and rides along
with `engine.State` so retries and resumes see the same overrides.

### Collaboration flow

1. Mode inspects workflow state (e.g., `MODULES.md` missing).
2. Mode resolves the module from the registry and calls `Module.Run(ctx)`.
3. Module uses `ArtifactStore` helpers to read inputs, ensures frontmatter
   matches expectations, and writes the new artifact with the declared version.
4. Module returns a typed result (success, no-op, needs human input, etc.).
5. Mode reacts: update UI, emit `ModeProgressMsg`, mark itself complete, or ask
   the operator to intervene.

### Workflow dependency resolver

The workflow engine consumes workflow definitions via the dependency resolver in
`internal/workflow/resolver`. The resolver:

- Normalizes the workflow graph (merging inline `depends_on` edges) and
  instantiates each module from the shared registry.
- Evaluates completion state by calling `Module.IsComplete`, then marks modules
  as `ready`, `blocked`, `error`, or `complete` based on upstream readiness.
- Exposes a queue builder that returns the modules required to satisfy a set of
  targets, automatically inserting prerequisites when inputs are missing.
- Provides metadata for the engine/TUI (module reference, dependencies,
  dependents) so higher layers can render intent without re-building the graph.

Modes and the future workflow engine should call `Resolver.Refresh` to capture a
fresh snapshot, then read `Resolver.Ready()` or `Resolver.Queue()` when deciding
what to run next.

### Runnable selection + scheduling

`internal/workflow/scheduler` layers on top of the resolver to produce runnable
module batches for the engine. It evaluates the resolver queue, filters out
nodes that are still pending (missing artifacts, invalid fingerprints, or
blocked dependencies), and applies runtime constraints such as:

- Concurrency limits (`MaxParallel`) so we never launch more modules than the
  workflow can safely run in parallel (useful when tmux/OpenCode slots are
  limited).
- Manual gates that require operator approval before continuing. The scheduler
  records skip reasons so modes can surface "awaiting approval" states.
- Active run tracking so the same module isn't dispatched twice while it is
  still working.

Callers pass a `RunnableRequest` (targets, currently running IDs, manual gate
status, optional batch size) and receive a `RunnableBatch`. The batch includes
both runnable nodes and an explicit `Skipped` map explaining why otherwise-ready
nodes were withheld (manual gate, concurrency cap, not-ready state). This lets
Bubble Tea modes and the upcoming engine request work in a loop without knowing
about artifact invalidation details.

For the full concurrency strategy (slot accounting, exclusive modules, and
worker claim APIs) see `docs/parallel-execution.md`.

This explicit split lets us evolve the runtime incrementally: we can convert one
mode at a time to modules without blocking the rest of the CLI.

### Workflow engine state machine

The `internal/workflow/engine` package drives resolver + scheduler decisions and
persists snapshots to `.lattice/workflow/engine/state.json`. Each snapshot
includes the workflow definition, run identifier, module metadata, scheduler
skips, runtime constraints (targets, batch size, manual gates, running IDs), and
the last known module result.

- `Start(def)` normalizes the workflow, refreshes module states, chooses
  runnable nodes, and writes the first snapshot.
- `Resume()` reloads persisted state after a restart and re-applies
  `Resolver.Refresh` so artifact changes are reflected immediately.
- `Update(results...)` merges module run results (completed, failed, needs
  input) plus runtime overrides before recomputing runnable batches.

The engine emits a coarse status for the UI:

| Status     | Trigger                                                          |
| ---------- | ---------------------------------------------------------------- |
| `running`  | Runnable modules exist or modules are currently running          |
| `blocked`  | No runnable modules, but pending/blocked nodes remain            |
| `complete` | Every module resolved to `complete`                              |
| `error`    | Resolver surfaced `NodeStateError` or a module run reported fail |

Consumers can call `View()` to read the last snapshot or rely on
`Start/Resume/ Update` to mutate state. Because snapshots store the normalised
workflow definition, the engine can rebuild resolver/scheduler instances without
the UI needing to stash additional context.

### Error recovery path

When something breaks mid-run, the runtime records enough context to make the
next steps deterministic:

1. **Module execution → result tracking** – Every `module.Module` returns a
   `module.Result` with one of four statuses (`completed`, `no-op`,
   `needs-input`, `failed`). `workflowView` wraps those inside
   `engine.ModuleStatusUpdate` when the Bubble Tea worker finishes. The engine
   stores the result (plus any error) in `state.Runs[id]` so the UI and future
   automation can display "last run: failed" or "needs input" alongside the
   module description.
2. **Resolver refresh** – On the next `engine.Update` or `engine.Resume`, the
   resolver calls `Module.IsComplete` again. If the failure prevented outputs
   from being written (or you edited artifacts manually), the resolver
   downgrades the node back to `pending` and evaluates artifact metadata.
   Invalid or outdated artifacts emit `module.ArtifactInvalidation` events,
   attach details to `ModuleStatus.Artifacts`, and keep downstream nodes blocked
   until the offending module runs cleanly.
3. **Scheduler enforcement** – The scheduler inspects the refreshed node states
   and `Runtime.ManualGates`. Failed modules simply fall out of the `running`
   set, so their dependents stay blocked. Nodes that require manual approval are
   skipped with `SkipReasonManualGate` until the operator toggles approval in
   the workflow view.
4. **Engine status + resumptions** – If any module run reports `failed` or a
   resolver node transitions to `NodeStateError`, `deriveEngineStatus` marks the
   run as `error` and surfaces the offending module ID. Selecting **Resume
   Work** from the TUI menu triggers `engine.Resume`, which reloads
   `.lattice/workflow/engine/state.json`, replays runtime overrides (targets,
   manual gates, running IDs), and immediately re-runs the resolver so new file
   edits are reflected.
5. **Retry mechanics** – Operators can rerun the failed module directly in the
   workflow view (select the module → `Enter`). For headless retries use
   `module-runner --project /path/to/project --module <module-id>`; the CLI uses
   the same registry/config pipeline as the TUI. Once the module completes, call
   **Resume Work** (or wait for the automatic refresh) so the engine ingests the
   new status and unblocks downstream nodes.

Because every failure transitions through the resolver → scheduler → engine
loop, recovery is always "fix the artifact or code, rerun the module, refresh".
No manual database surgery is required—the persisted state plus artifact
metadata is enough to recompute readiness.

### Artifact metadata + versioning

Every document artifact now receives a `lattice` YAML frontmatter block (JSON
artifacts receive a `_lattice` object). The shared schema mirrors
`artifact.Metadata`:

- `artifact`: stable ID (e.g., `modules-doc`)
- `module`: module ID that created the file
- `version`: semver-like module version string
- `created`: RFC3339 timestamp when the artifact was written
- `workflow`: workflow identifier (defaults to `commission-work`)
- `inputs`: list of artifact IDs that were consumed
- `checksum`: optional sha256 of the body for invalidation
- `notes`: optional key/value hints (e.g., prompt variant)

Modules are responsible for bumping their `Info.Version` whenever the output
contract changes (new frontmatter fields, different markdown headings, etc.).
`ArtifactStore.Write` automatically copies `Info.ID` into `artifact` and fills
in timestamps so every artifact can be reasoned about later.

### Invalidation policy

`ArtifactStore.Check` evaluates artifacts into four states:

- `ready`: file exists, metadata is readable, and the artifact ID matches the
  ref; downstream modules may consume it
- `missing`: artifact not on disk yet
- `invalid`: metadata missing/incorrect, stale version, or checksum mismatch
- `error`: filesystem failure when reading

Modules should treat `invalid` the same as `missing` and re-run their
dependencies. Later iterations will extend `Check` to compute dependency hashes
so we can automatically invalidate downstream outputs when upstream inputs
change. The policy is straightforward:

1. If any input `Check` returns `invalid`, the upstream module must be re-run
2. When a module runs with different `Inputs` or a new `Version`, every artifact
   it writes receives the updated provenance, ensuring downstream modules notice
3. When a `checksum` is provided, `Check` compares it to the current body to
   guard against manual edits outside the runtime

### Artifact fingerprints + invalidation hooks

- Modules that implement `module.Fingerprinter` can return a map of artifact IDs
  to fingerprint strings (hash of inputs, config digest, etc.).
- When a module writes an artifact it can persist the fingerprint inside the
  metadata by calling `runtime.WithFingerprint(ref, value)`.
- `Resolver.CheckArtifact` automatically compares the stored fingerprint (notes
  entry `fingerprint:<artifact-id>`) to the module's current value.
- Matching fingerprints mark the artifact as `fresh`; mismatches emit
  `module.ArtifactInvalidation` events via `module.ArtifactInvalidationHandler`.
- Invalidation reasons include missing files, malformed metadata, module version
  drift, or fingerprint mismatches. Modules can react to these events to clean
  up derived artifacts or schedule dependent work.

### Orchestrator-selection module IO

- **Inputs** – The module may only run once the consolidation phase has stamped
  `.lattice/action/MODULES.md` and `.lattice/action/PLAN.md` plus the
  `.reviews-applied` and `.beads-created` markers. These artifacts prove the
  plan is final and beads exist for quoting downstream work.
- **Configuration dependencies** – `ModuleContext.Config` must reference an
  initialized `.lattice` tree so `Config.CommunitiesDir()` finds installed
  communities and their `cvs/**/cv.md` files. The module regenerates the
  orchestrator’s AGENT.md beneath `.lattice/agents/orchestrator/` and rewrites
  `opencode.jsonc`, so those directories must be writable.
- **Outputs** – `artifact.OrchestratorState`
  (`.lattice/workflow/orchestrator.json`) capturing the selected denizen’s
  `name`, `community`, and `cvPath` alongside a `_lattice` metadata block
  stamped with `module: orchestrator-selection`, the module version, workflow
  identifier, and `inputs` referencing `modules-doc`, `action-plan`,
  `reviews-applied`, and `beads-created`. The module also updates
  `artifact.WorkersJSON` (`workflow/team/workers.json`) so hiring can see the
  orchestrator roster entry with matching provenance.

### Hiring module IO

- **Inputs** – Hiring will not run until orchestration is locked in. It consumes
  `.lattice/workflow/orchestrator.json` (`artifact.OrchestratorState`) plus the
  consolidated plan artifacts: `.lattice/action/MODULES.md`
  (`artifact.ModulesDoc`), `.lattice/action/PLAN.md` (`artifact.ActionPlanDoc`),
  and the `.beads-created` marker (`artifact.BeadsCreatedMarker`). These inputs
  guarantee bead creation is finished and the workload snapshot is stable.
- **Configuration dependencies** – The module needs a fully initialised
  `ModuleContext.Orchestrator` capable of `LoadDenizenCVs()` so it can enumerate
  denizens from `<LATTICE_ROOT>/communities/*/cvs/**`. `ModuleContext.Config`
  must expose writable `AgentsDir()` and `WorkerListPath()` locations because
  the module rewrites
  `.lattice/agents/{workers,specialists}/<slug>/{AGENT,AGENT_SUP}.md` files
  alongside `workflow/team/workers.json`. Hiring shells out to `tmux`,
  `opencode`, and `skills.Ensure` to run the bundled `create-agent-file` skill
  for each hire, and it shells out to `bd ready --json` plus repeated
  `bd create` commands to size the workload and mint follow-up beads.
- **Outputs** – At minimum the module writes `artifact.WorkersJSON`
  (`workflow/team/workers.json`) populated with worker/specialist entries,
  capacities, `isSpark` flags, and `_lattice` provenance metadata referencing
  the artifacts above. It also generates dual dossiers (`AGENT.md` for the
  worker and `AGENT_SUP.md` for supervisors) beneath `.lattice/agents/`, staging
  source CVs into `.lattice/setup/cvs/<community>/<name>/` before invoking the
  skill. Additionally an epic titled `HIRE` and one bead per agent are created
  in bd so subsequent modules (work-process, refinement) can trace AGENT brief
  creation tasks.

### Work-process module IO

- **Inputs** – The module only runs once hiring has produced
  `workflow/team/workers.json` (`artifact.WorkersJSON`) and the orchestrator
  module has written `workflow/orchestrator.json`
  (`artifact.OrchestratorState`). It also expects the roster’s generated
  AGENT/MEMORY files under `.lattice/agents/` plus a ready queue of beads in
  `bd` (populated after the `artifact.BeadsCreatedMarker`). Without those
  dossiers or beads the module cannot bind sessions to agents.
- **Configuration dependencies** – `ModuleContext.Orchestrator` must be wired so
  `PrepareWorkCycle`/`RunUpCycle` can launch `tmux` → `opencode` flows, install
  the `opencode-worktree` plugin, and issue `bd ready --json`. The module relies
  on writable `WorktreeDir()` and `SkillsDir()` paths for per-agent worktrees
  and bundled skills (`final-session`, `down-cycle`, `down-cycle-agent`,
  `local-dreaming`). `WorkerListPath()`, `AgentsDir()`, and `StateDir()` must
  point to the same `.lattice` tree so cycle trackers, plan updates, and memory
  files live alongside previous modules’ outputs.
- **Outputs** – Each run writes `workflow/work/.in-progress` before dispatching
  sessions, `workflow/work/current-cycle.json` to persist the prepared roster,
  and `workflow/work/.complete` once the down-cycle finishes (or
  `.refinement-needed` when no beads are ready, which refinement treats as its
  entry point). It materialises worktrees under `.lattice/worktree/<cycle>/`
  with refreshed `WORKTREE.md`, question inbox/outbox mailboxes, logs, and
  `SUMMARY.md` files per agent plus orchestrator summaries in
  `.lattice/state/cycle-<n>/SUMMARY.md`. The module appends every cycle report
  to `.lattice/workflow/work/work-log.md` (`artifact.WorkLogDoc`) and refreshes
  `.lattice/state/REPO_MEMORY.md` plus agent-level `MEMORY.md` entries as part
  of the down-cycle skills.
- **Downstream signals** – Work-process restarts the orchestrator prompt with
  the next cycle number, opens `bd` tickets for unrelated bugs logged during the
  down-cycle, and guarantees `workflow/work/.complete` exists before release
  starts. Refinement and release modules read the work log, per-agent summaries,
  and `.refinement-needed` marker to plan audits or deploy artifacts.
