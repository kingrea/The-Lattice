# Lattice Docs Overview

This directory houses reference material for the module runtime. Start here for
an overview of how the planning, staffing, work, refinement, and release modules
connect, then dive into the focused primers alongside it when you need deeper
context.

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
