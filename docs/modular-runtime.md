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

This explicit split lets us evolve the runtime incrementally: we can convert one
mode at a time to modules without blocking the rest of the CLI.

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
