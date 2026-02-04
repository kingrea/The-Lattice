package parallel_reviews

// Package parallel_reviews documents the IO contract for spawning the four
// reviewer personas (Pragmatist, Simplifier, User Advocate, Skeptic).
//
// Required inputs (read-only):
//   - MODULES.md (`artifact.ModulesDoc`) and PLAN.md (`artifact.ActionPlanDoc`)
//     from the staff-incorporated plan
//   - The three anchor documents (`artifact.CommissionDoc`,
//     `artifact.ArchitectureDoc`, `artifact.ConventionsDoc`) used to provide
//     broader context to each reviewer
//
// Output artifacts, each a markdown document stored under `.lattice/action/`:
//   - REVIEW_PRAGMATIST.md (`artifact.ReviewPragmatistDoc`)
//   - REVIEW_SIMPLIFIER.md (`artifact.ReviewSimplifierDoc`)
//   - REVIEW_USER_ADVOCATE.md (`artifact.ReviewAdvocateDoc`)
//   - REVIEW_SKEPTIC.md (`artifact.ReviewSkepticDoc`)
