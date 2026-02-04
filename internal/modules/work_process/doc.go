package work_process

// Package work_process documents the IO contract for the module that drives
// execution cycles once hiring is finished.
//
// Required inputs (read-only artifacts under `.lattice/workflow/`):
//   - `workers.json` (`artifact.WorkersJSON`) produced by the hiring module so
//     scheduled agents, their roles, and point capacities are known when work
//     sessions are created.
//   - `orchestrator.json` (`artifact.OrchestratorState`) exposing the selected
//     conductor so OpenCode prompts can default to the right persona and the
//     orchestrator summary skill can attribute cycle decisions correctly.
//   - Generated AGENT dossiers inside `.lattice/agents/{workers,specialists}/`
//     and any companion MEMORY.md files. The module binds roster entries to
//     these dossiers before seeding prompts, so missing dossiers aborts the run.
//   - A ready bead queue in `bd` (the module shells out to `bd ready --json`).
//     Hiring must have created beads already (`artifact.BeadsCreatedMarker`) or
//     no sessions can be scheduled.
//
// Runtime dependencies:
//   - `ModuleContext.Orchestrator` must be initialised; the module delegates all
//     scheduling to `PrepareWorkCycle`/`RunUpCycle`, which expect `bd`, `tmux`,
//     `opencode`, and the `opencode-worktree` plugin on `$PATH`.
//   - `ModuleContext.Config.WorktreeDir()` and `SkillsDir()` must be writable so
//     per-agent worktrees, `SUMMARY.md`, question inbox/outbox folders, and
//     skill payloads (`final-session`, `down-cycle`, `down-cycle-agent`,
//     `local-dreaming`) can be installed on demand.
//   - `ModuleContext.Config.WorkerListPath()` and `AgentsDir()` must remain in
//     sync with hiring outputs; the orchestrator cross-links roster entries to
//     dossiers before dispatching OpenCode windows.
//   - `ModuleContext.Config.StateDir()` stores `state/cycle.json` (cycle counter)
//     plus per-cycle summaries (`state/cycle-*/SUMMARY.md`) and
//     `state/REPO_MEMORY.md`, all of which the module rewrites while landing a
//     cycle.
//
// Outputs:
//   - Work markers beneath `.lattice/workflow/work/`: `.in-progress` stamped
//     before sessions launch, `.complete` written after down-cycle cleanup, and
//     `.refinement-needed` emitted when no ready beads exist (refinement treats
//     this as a gate signal).
//   - `current-cycle.json` in `.lattice/workflow/work/` tracking prepared
//     sessions so the TUI and scheduler can resume partially finished cycles.
//   - Worktree directories under `.lattice/worktree/<cycle>/<session>/` with
//     refreshed `WORKTREE.md`, `LOG.md`, per-cycle archives, and question
//     mailboxes. These directories act as the agent IO contract for OpenCode.
//   - Down-cycle artifacts: a cycle summary for each agent and the orchestrator
//     (`state/cycle-<n>/SUMMARY.md`), appended personal memories in
//     `.lattice/agents/**/MEMORY.md`, and a refreshed `state/REPO_MEMORY.md`.
//   - `work-log.md` (`artifact.WorkLogDoc`) under `.lattice/workflow/work/`
//     appended with every cycle report (completed beads, remaining work,
//     orchestrator notes) so refinement/release modules can audit execution.
//
// Side effects consumed later:
//   - New beads created during down-cycle orchestration (unrelated bugs or help
//     tickets) plus updated PLAN.md references that keep hiring/refinement in
//     sync with real progress.
//   - A restarted orchestrator prompt with the next cycle number so future runs
//     can be resumed without manual reconfiguration.
