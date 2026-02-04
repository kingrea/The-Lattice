package action_plan

// Package action_plan will host the action-plan module implementation. The
// module stitches together the anchor documents produced by lattice-planning
// and emits the actionable plan artifacts that subsequent planning phases rely
// on.
//
// Required inputs (all under `.lattice/plan/`):
//   - COMMISSION.md (`artifact.CommissionDoc`)
//   - ARCHITECTURE.md (`artifact.ArchitectureDoc`)
//   - CONVENTIONS.md (`artifact.ConventionsDoc`)
//
// Outputs (written to `.lattice/action/` with lattice frontmatter):
//   - MODULES.md (`artifact.ModulesDoc`) describing each execution module
//   - PLAN.md (`artifact.ActionPlanDoc`) detailing sequencing, owners, and flow
//
// Downstream modules (`staff_review`, `staff_incorporate`, `parallel_reviews`,
// etc.) read MODULES.md and PLAN.md verbatim, so the action-plan module must
// ensure both files exist, are up to date with the latest anchor docs, and carry
// provenance metadata (`module="action-plan"`) before reporting completion.
