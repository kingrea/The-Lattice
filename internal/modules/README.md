# Built-in Modules

This directory will host the concrete module implementations extracted from the
legacy Bubble Tea modes. The initial wave maps directly to the planning phase
substeps outlined in `.ai/MODULAR_WORKFLOW_PLAN.md`:

1. `anchor_docs` – Generate COMMISSION.md, ARCHITECTURE.md, CONVENTIONS.md
2. `action_plan` – Produce MODULES.md and PLAN.md
3. `staff_review` – Capture orchestrator review feedback
4. `staff_incorporate` – Apply staff feedback to the plan
5. `parallel_reviews` – Run Pragmatist/Simplifier/Advocate/Skeptic reviews
6. `consolidation` – Merge reviewer feedback and emit `.reviews-applied`
7. `bead_creation` – Create beads + `.beads-created` marker
8. `orchestrator_selection` – Choose orchestrator + populate orchestrator.json
   - Inputs: consolidation-stamped MODULES.md/PLAN.md plus `.reviews-applied`
     and `.beads-created` markers.
   - Outputs: `.lattice/workflow/orchestrator.json` (with `_lattice` metadata)
     and an updated `workflow/team/workers.json` referencing the selected
     denizen.
9. `hiring` – Convert the consolidated plan into a roster and AGENT briefs
   - Inputs: `.lattice/workflow/orchestrator.json`,
     `.lattice/action/MODULES.md`, `.lattice/action/PLAN.md`, and
     `.lattice/action/.beads-created`.
   - Outputs: `.lattice/workflow/team/workers.json` with `_lattice` provenance,
     refreshed
     `.lattice/agents/{workers,specialists}/<slug>/{AGENT,AGENT_SUP}.md`
     folders, staged CV copies under `.lattice/setup/cvs/<community>/<name>/`,
     plus a `HIRE` epic in bd with per-agent beads tracking dossier authorship.
10. `work_process` – Drive agents through execution
    - Inputs: `.lattice/workflow/team/workers.json`,
      `.lattice/workflow/orchestrator.json`, roster AGENT/MEMORY files, and a
      ready bead queue in bd.
    - Outputs:
      `.lattice/workflow/work/{.in-progress,.complete,.refinement-needed}`
      markers, `workflow/work/current-cycle.json`, per-agent worktrees under
      `.lattice/worktree/<cycle>/`, agent/orchestrator summaries in
      `.lattice/state/cycle-*/SUMMARY.md`, refreshed `state/REPO_MEMORY.md`, and
      appended `workflow/work/work-log.md` entries.
11. `refinement` – Handle post-work QA loops
    - Inputs: `.lattice/workflow/work/.refinement-needed` gate from
      work-process, the roster artifacts (`workflow/team/workers.json` plus
      generated `.lattice/agents/**/AGENT.md` dossiers), existing work-cycle
      outputs (`.complete`, work-log, worktree archives), and repository
      metadata (e.g. package.json, go.mod) used to classify the project profile.
    - Runtime deps: same orchestrator plumbing as work-process – tmux,
      opencode + opencode-worktree plugin, bd CLI, and writable workflow
      directories for audit + work markers.
    - Outputs: `workflow/team/stakeholders.json`, Markdown audits under
      `workflow/audit/`, a synthesized `SYNTHESIS.md` that records every bead
      opened from the audits, and refreshed `.in-progress`/`.complete` markers
      (plus removal of `.refinement-needed`). Audit synthesis also shells out to
      `bd create`, so the follow-up beads appear in the queue for release.

Each subdirectory contains a Go package reserved for the module. They only
expose a `doc.go` placeholder today so future patches can land actual logic
without rebasing directory changes.
