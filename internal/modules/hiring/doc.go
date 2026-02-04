package hiring

// Package hiring documents the IO contract for translating a consolidated plan
// and orchestrator selection into an executable roster.
//
// Required inputs (read-only artifacts under `.lattice/`):
//   - MODULES.md (`artifact.ModulesDoc`) stamped by consolidation so the module
//     can derive the list of beads and role mix.
//   - PLAN.md (`artifact.ActionPlanDoc`) for story point totals and task
//     descriptions referenced inside the generated worker briefs.
//   - `.beads-created` marker (`artifact.BeadsCreatedMarker`) signalling every
//     bead exists in bd before hiring clones the roster.
//   - `orchestrator.json` (`artifact.OrchestratorState`) describing the selected
//     conductor whose CV directory seeds denizen lookups.
//
// Configuration + runtime dependencies:
//   - `ModuleContext.Orchestrator` must be initialised; hiring calls
//     `LoadDenizenCVs` to enumerate candidates from
//     `<LATTICE_ROOT>/communities/*/cvs/**/cv.md`.
//   - `ModuleContext.Config` must expose writable `AgentsDir()` and `WorkerListPath()`
//     locations because the module rewrites `.lattice/agents/{workers,specialists}`
//     folders and `workflow/team/workers.json` during every run.
//   - The skills runtime (`skills.Ensure`) requires the `create-agent-file` skill
//     under `skills/` plus a functioning `tmux` + `opencode` toolchain; hiring
//     shells into tmux windows to drive the skill for each agent.
//   - `bd` must be on `$PATH` because workload sizing parses `bd ready --json`
//     and the module opens a dedicated `HIRE` epic plus one bead per agent to
//     track AGENT.md generation.
//
// Outputs:
//   - `workflow/team/workers.json` (`artifact.WorkersJSON`) populated with
//     workers and specialists (including SPARK placeholders). The JSON payload
//     must carry `_lattice` metadata tying it to `module="hiring"`, the module
//     version, workflow identifier, and the inputs listed above so downstream
//     modules can reason about provenance.
//   - Agent dossiers inside `.lattice/agents/`. For each worker the module writes
//     `agents/workers/<slug>/AGENT.md` plus a companion `AGENT_SUP.md` support
//     packet; specialists render to `agents/specialists/<slug>/{AGENT,AGENT_SUP}.md`.
//     SPARK hires receive stub documents, while non-SPARK hires run the
//     `create-agent-file` skill with staged CVs in
//     `.lattice/setup/cvs/<community>/<name>/`.
//
// Side effects consumed later:
//   - `bd create` tickets (a parent `HIRE` epic and per-agent beads) which the
//     work-process module references when bootstrapping execution.
//   - A refreshed `workers.json` timestamp that keeps orchestrator dashboards in
//     sync with the roster used by work-process and refinement.
