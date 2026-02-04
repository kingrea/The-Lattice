package staff_review

// Package staff_review documents the contract for the staff-review planning
// module. The module acts as the Staff Engineer who reviews the generated plan
// before it is shown to the user.
//
// Required inputs (read-only):
//   - MODULES.md (`artifact.ModulesDoc`) describing the execution modules
//   - PLAN.md (`artifact.ActionPlanDoc`) sequencing the work
//
// Output artifact:
//   - STAFF_REVIEW.md (`artifact.StaffReviewDoc`) containing the staff engineer
//     feedback with lattice frontmatter and provenance metadata
