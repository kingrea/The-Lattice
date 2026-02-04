package staff_incorporate

// Package staff_incorporate documents the inputs/outputs for the module that
// applies the Staff Engineer review directly to the plan files.
//
// Required inputs:
//   - STAFF_REVIEW.md (`artifact.StaffReviewDoc`) containing the feedback to
//     apply
//   - MODULES.md (`artifact.ModulesDoc`) and PLAN.md (`artifact.ActionPlanDoc`)
//     as the documents being updated in-place
//
// Outputs:
//   - Updated MODULES.md and PLAN.md with provenance set to `staff-incorporate`
//   - `.staff-feedback-applied` marker (`artifact.StaffFeedbackApplied`) that
//     signals the user can now review the improved plan
