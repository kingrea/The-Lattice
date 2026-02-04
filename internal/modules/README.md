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
9. `hiring` – Build workers.json + agent folders
10. `work_process` – Drive agents through execution
11. `refinement` – Handle post-work QA loops

Each subdirectory contains a Go package reserved for the module. They only
expose a `doc.go` placeholder today so future patches can land actual logic
without rebasing directory changes.
