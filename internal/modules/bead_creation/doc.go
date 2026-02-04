package bead_creation

// Package bead_creation documents the IO for converting the final plan into a
// beads backlog.
//
// Required inputs:
//   - MODULES.md (`artifact.ModulesDoc`) and PLAN.md (`artifact.ActionPlanDoc`)
//     that already reflect consolidated feedback
//   - `.reviews-applied` marker (`artifact.ReviewsAppliedMarker`) proving the
//     plan is ready for tracking
//
// Outputs:
//   - `.beads-created` marker (`artifact.BeadsCreatedMarker`) indicating every
//     module/task has corresponding beads entries. The module also initializes
//     bd in the repo and leaves the freshly created beads in `.beads/`.
