package consolidation

// Package consolidation documents the IO contract for the orchestrator phase
// that synthesizes the four reviewer files.
//
// Required inputs:
//   - Anchor documents (`artifact.CommissionDoc`, `artifact.ArchitectureDoc`,
//     `artifact.ConventionsDoc`)
//   - MODULES.md (`artifact.ModulesDoc`) and PLAN.md (`artifact.ActionPlanDoc`)
//     containing the latest staff-incorporated plan
//   - Staff review output (`artifact.StaffReviewDoc`)
//   - All four reviewer files (`artifact.ReviewPragmatistDoc`,
//     `artifact.ReviewSimplifierDoc`, `artifact.ReviewAdvocateDoc`,
//     `artifact.ReviewSkepticDoc`)
//
// Outputs:
//   - Updated MODULES.md and PLAN.md with the consolidated changes attributed to
//     the `consolidation` module
//   - `.reviews-applied` marker (`artifact.ReviewsAppliedMarker`) signaling that
//     the review feedback has been ingested and the plan is ready for bead
//     creation
