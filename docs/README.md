# Lattice Docs Overview

This directory houses reference material for the module runtime. Start here for
an overview of how the planning, staffing, work, refinement, and release modules
connect, then dive into the focused primers alongside it when you need deeper
context.

## Workflow Options

### Choosing a workflow

1. Launch `lattice` and highlight **Commission Work** on the home screen.
2. Press _enter_ to open the workflow picker, then use ↑/↓ to highlight
   `commission-work`, `quick-start`, `solo`, or any custom workflow that was
   discovered from `.lattice/config.yaml`.
3. Press _enter_ again to start the highlighted workflow. The picker writes your
   decision back to `.lattice/config.yaml`, so the next run starts from the same
   workflow unless you change it.

The picker lives inside the TUI, so you can review ready/running/blocked modules
before committing to a run. Selecting **Resume Work** from the main menu reloads
the persisted engine snapshot and continues executing whichever workflow is
stored on disk.

### Pinning the default workflow

Projects can pin (and optionally limit) workflows by editing
`.lattice/config.yaml`:

```yaml
version: 1
workflows:
  default: solo # launch solo runs by default
  available:
    - commission-work
    - quick-start
    - solo
```

The runtime loads every workflow defined under `${LATTICE_ROOT}/workflows/` plus
any project-scoped files under `<project>/workflows/`. Providing an `available`
list hides everything else from the picker while still allowing power users to
swap IDs manually by editing the config.

### Built-in workflows at a glance

| Workflow          | When to choose it                                                            | Module sequence                                                                                                                                                                         | Prerequisites                                                                                                                  |
| ----------------- | ---------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `commission-work` | Full delivery cycle with persona reviews, consolidation, refinement, release | anchor-docs → action-plan → staff-review → staff-incorporate → parallel-reviews → consolidation → bead-creation → orchestrator-selection → hiring → work-process → refinement → release | Crew slots available for reviewer personas, tmux/OpenCode capacity for parallel runs, appetite for full gating before staffing |
| `quick-start`     | Fast quotes + staffed cycle without persona fan-out                          | anchor-docs → action-plan → staff-review → bead-creation → orchestrator-selection → hiring → work-process → release                                                                     | Comfortable skipping consolidation/refinement; need at least one orchestrator + crew ready to staff immediately                |
| `solo`            | Single operator wants planning + execution without hiring overhead           | anchor-docs → action-plan → solo-work → release                                                                                                                                         | Solo operator with `solo-work` module installed; no orchestration/hiring dependencies                                          |

- `commission-work` is the safest path for high-risk or multi-stakeholder work.
  It expects that persona reviewers, consolidation, refinement, and release all
  run in a single engagement before beads can move downstream.
- `quick-start` keeps anchor docs, action planning, staff review, staffing, and
  release but skips the persona, consolidation, and refinement loops to reduce
  turnaround time.
- `solo` is the lightweight preset; `solo-work` produces work logs and release
  markers without touching orchestrator-selection, hiring, or work-process
  modules.

### Creating custom workflows

1. Copy an existing file in `workflows/` (or start from scratch) and set a new
   `id`, `name`, and `description`.
2. Declare the modules you need under `modules:` and wire dependencies via
   `depends_on`. Optional `config` maps travel alongside each module and are
   injected into the runtime at execution time.
3. Place the file under `${LATTICE_ROOT}/workflows/` for a global preset or
   inside your project at `workflows/<id>.yaml` so it lives with the repo.
4. Either edit `.lattice/config.yaml` → `workflows.default: <id>` or select the
   new ID once from the TUI picker. The picker automatically appends new IDs to
   `workflows.available` after their first run.

Custom workflows inherit the same resolver, scheduler, and artifact guarantees,
so prerequisites (missing artifacts, invalid metadata, manual gates) are still
enforced even when the graph diverges from the presets below.

## Module Pipeline

The default `commission-work` workflow wires the following modules in order. The
engine enforces these dependencies automatically:

| Order | Module ID                | Purpose                                                                                           |
| ----- | ------------------------ | ------------------------------------------------------------------------------------------------- |
| 1     | `anchor-docs`            | Launches the planning skill to produce COMMISSION/ARCHITECTURE/CONVENTIONS.                       |
| 2     | `action-plan`            | Converts anchor docs into MODULES/PLAN.                                                           |
| 3     | `staff-review`           | Runs the staff engineer review on MODULES/PLAN.                                                   |
| 4     | `staff-incorporate`      | Applies staff feedback, stamping readiness markers.                                               |
| 5     | `parallel-reviews`       | Executes the persona reviews in tmux.                                                             |
| 6     | `consolidation`          | Synthesizes reviewer feedback back into PLAN.md.                                                  |
| 7     | `bead-creation`          | Initializes `bd`, creates beads, and writes the `.beads-created` marker.                          |
| 8     | `orchestrator-selection` | Chooses the orchestrator and refreshes `workflow/orchestrator.json` plus `workers.json`.          |
| 9     | `hiring`                 | Builds the worker roster, generates AGENT briefs, and records support packets.                    |
| 10    | `work-process`           | Stages work cycles, runs the orchestrator loop, and updates work logs/markers.                    |
| 11    | `refinement`             | Runs stakeholder audits when `.refinement-needed` is present and clears it on completion.         |
| 12    | `release`                | Packages artifacts, writes release notes, and clears runtime directories for the next commission. |

Every module declares its artifact inputs/outputs inside `internal/artifact` and
is registered through `internal/modules/modules.go`. Updating the workflow YAML
is enough to change the pipeline order so the TUI, engine, and headless CLIs all
see the same topology.

Use the **Commission Work** workflow picker in the TUI to choose which
definition to launch. Arrow keys move between `commission-work`, `quick-start`,
`solo`, and any custom workflows discovered from `.lattice/config.yaml`. Press
_enter_ to start the highlighted workflow; the picker records your selection
back to the config file so later sessions start with the same default.

For rapid engagements the repository also includes the `quick-start` workflow:

| Order | Module ID                | Purpose                                                                |
| ----- | ------------------------ | ---------------------------------------------------------------------- |
| 1     | `anchor-docs`            | Capture the scope definition with COMMISSION/ARCHITECTURE/CONVENTIONS. |
| 2     | `action-plan`            | Generate MODULES/PLAN directly from the anchor docs.                   |
| 3     | `staff-review`           | Run a single staff pass for quality without persona fan-out.           |
| 4     | `bead-creation`          | Convert the reviewed plan into beads for quick scheduling.             |
| 5     | `orchestrator-selection` | Lock the orchestrator roster and refresh workflow/orchestrator.json.   |
| 6     | `hiring`                 | Hire the workers and generate AGENT briefs for the scoped cycle.       |
| 7     | `work-process`           | Run the work cycle through the orchestrator and log outputs.           |
| 8     | `release`                | Package the deliverables and reset runtime directories.                |

Select `quick-start` in the workflow picker (or set `.lattice/config.yaml` →
`workflows.default: quick-start`) when you need this abbreviated path; both
workflows share the same runtime + resolver semantics.

For solo operators there is a dedicated `solo` workflow:

| Order | Module ID     | Purpose                                                                                               |
| ----- | ------------- | ----------------------------------------------------------------------------------------------------- |
| 1     | `anchor-docs` | Establish the three anchor docs so the solo run has the same intake context as larger workflows.      |
| 2     | `action-plan` | Generate MODULES/PLAN to outline the solo execution steps.                                            |
| 3     | `solo-work`   | Create the solo execution log, synthesize the worker/orchestrator metadata, and mark work completion. |
| 4     | `release`     | Package artifacts and close the workflow immediately after the solo log marks completion.             |

Choose `solo` in the workflow picker (or set `.lattice/config.yaml` →
`workflows.default: solo`) when one operator needs the anchor docs, plan, and
release cadence without going through hiring or the work-process orchestrator
loop.

## Running Modules

### Bubble Tea workflow view

The TUI's **Workflow** pane reflects the current engine snapshot. Use the inline
shortcuts to run modules, approve manual gates, or rerun nodes that reported
`needs-input`. Once a module completes, the resolver automatically unblocks its
dependents based on the YAML definition above.

### `module-runner` CLI

For headless or automated runs, use `cmd/module-runner`:

```bash
module-runner \
  --project /path/to/project \
  --module work-process \
  --config-file overrides/work-process.yaml \
  --set max_parallel=2
```

`module-runner` bootstraps the same registry and workflow context as the TUI.
The CLI respects workflow state, so running `module-runner --module refinement`
does nothing until the `.refinement-needed` marker exists.

Refer to `docs/modular-runtime.md` for deep dives into the resolver, scheduler,
and artifact metadata, plus `docs/error-recovery.md` for troubleshooting flows.
