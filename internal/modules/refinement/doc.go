package refinement

// Package refinement documents the IO contract for the post-work refinement
// module. The module is responsible for turning the `.refinement-needed` gate
// emitted by work-process into stakeholder audits, synthesized follow-up beads,
// and a cleanup pass that returns the workflow to a releasable state.
//
// Preconditions + required inputs:
//   - `.lattice/workflow/work/.refinement-needed`
//     (`artifact.RefinementNeededMarker`). Refinement only runs after the work
//     module reports that no ready beads remain or an audit is explicitly
//     requested. If the marker is missing the module should no-op.
//   - `workflow/team/workers.json` (`artifact.WorkersJSON`) and the generated
//     agent dossiers inside `.lattice/agents/**/AGENT.md`. They allow the module
//     to detect which denizens already touched the project so stakeholder roles
//     can prioritise fresh eyes.
//   - `workflow/work/.complete` (`artifact.WorkCompleteMarker`) and the work log
//     / worktree archives created by the work-process module. Auditors read
//     those artifacts plus the repo itself to judge quality. (The module does
//     not rewrite them but expects them in place.)
//   - Repository metadata under `ModuleContext.Config.ProjectDir` (package.json,
//     go.mod, etc.). Refinement samples these files to label the project profile
//     and select the 10 stakeholder roles to run.
//
// Runtime + configuration requirements:
//   - `ModuleContext.Orchestrator` must be initialised with a functional `bd`,
//     `tmux`, and `opencode` toolchain. The module shells out to
//     `RunStakeholderAudit`, `RunAuditSynthesis`, `PrepareWorkCycle`, and
//     `RunUpCycle`, all of which depend on the same helpers as work-process
//     (opencode-worktree plugin, skills directory, etc.).
//   - The workflow directories exposed through `ModuleContext.Workflow` must be
//     writable: `TeamDir()` for `stakeholders.json`, `Dir()/audit/` for audit
//     markdown, and `WorkDir()` for `.in-progress`, `.complete`, and
//     `.refinement-needed` markers. The module also stages manual-review tmux
//     windows, so callers must allow it to spawn shells.
//
// Outputs + side effects:
//   - `workflow/team/stakeholders.json` – JSON manifest describing the detected
//     project profile and the agents assigned to ~10 stakeholder roles. Each
//     entry records whether the reviewer already worked on the cycle so future
//     audits can reuse or rotate coverage intentionally.
//   - `workflow/audit/` – Directory populated with `<role>-audit.md` files and a
//     `SYNTHESIS.md` summary. The orchestrator reads every audit, calls `bd
//     create` for each actionable finding, and documents which beads were
//     opened plus any “no action” notes.
//   - Work markers – Refinement ensures `workflow/work/.in-progress` exists
//     while it drives the follow-up cycle, rewrites `.complete` on success, and
//     removes `.refinement-needed` after the operator acknowledges completion.
//     If `PrepareWorkCycle` reports `ErrNoReadyBeads` the module records that no
//     additional work was required but still clears the refinement marker so
//     release can proceed.
//   - Follow-up beads – Audit synthesis shells out to `bd` and may also spawn a
//     manual review tmux window. Those beads, plus any tmux session summary, act
//     as the downstream contract for release and future work cycles.
