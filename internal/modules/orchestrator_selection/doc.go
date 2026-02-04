package orchestrator_selection

// Package orchestrator_selection documents the IO contract for the module that
// selects an orchestrator and captures their roster entry.
//
// Required inputs (read-only artifacts under `.lattice/action/`):
//   - MODULES.md (`artifact.ModulesDoc`) annotated with provenance from the
//     `consolidation` module
//   - PLAN.md (`artifact.ActionPlanDoc`) stamped by consolidation
//   - `.reviews-applied` marker (`artifact.ReviewsAppliedMarker`) proving all
//     reviewer feedback has been merged
//   - `.beads-created` marker (`artifact.BeadsCreatedMarker`) to ensure the plan
//     has been converted into executable beads before hiring orchestration begins
//
// Runtime dependencies:
//   - `ModuleContext.Config.Communities()` must resolve to at least one
//     installed community so denizen CVs can be loaded. The module scans
//     `<LATTICE_ROOT>/communities/*/cvs/**/cv.md` via `config.CommunitiesDir()`
//     and `community.Load`.
//   - The `.lattice/agents/` tree must be writable because the orchestrator's
//     AGENT.md brief is (re)generated beneath
//     `.lattice/agents/orchestrator/<slug>/`.
//
// Outputs:
//   - `orchestrator.json` (`artifact.OrchestratorState`) in
//     `.lattice/workflow/` describing the selected agent. The JSON payload
//     includes `name`, `community`, and `cvPath` alongside a `_lattice` metadata
//     block stamped with `module="orchestrator-selection"`, the module version,
//     workflow identifier, and `inputs: ["modules-doc", "action-plan",
//     "reviews-applied", "beads-created"]`.
//   - `workers.json` (`artifact.WorkersJSON`) updated so `orchestrator` matches
//     the selected agent and `updatedAt` reflects the current UTC timestamp. The
//     module should also ensure the roster JSON carries `_lattice` metadata tied
//     to the same module/version for provenance.
//
// Side effects expected by downstream modules:
//   - A refreshed `opencode.jsonc` via `orchestrator.RefreshOpenCodeConfig()` so
//     future `opencode` sessions default to the chosen orchestrator identity.
//   - A generated AGENT.md brief for the orchestrator role; hiring depends on
//     these files when cloning the roster.
