package release

// Package release documents the release module contract. The module only runs
// once work-process has written `workflow/work/.complete` (exposed as
// artifact.WorkCompleteMarker) and any refinement follow-ups cleared the
// `.refinement-needed` gate. Its inputs include the final work log, per-agent
// summaries, audit synthesis, orchestrator + roster manifests, and the
// reconciled bead queue so it can describe what shipped versus what remains. On
// success the module emits release notes under `.lattice/workflow/release`
// (capturing the shipped artifacts, commit IDs, and refinement deltas), archives
// the final package/diagnostics bundle, and serializes the three release markers
// `.agents-released`, `.cleanup-done`, and `.orchestrator-released`. These
// markers let the workflow engine short-circuit future release attempts during
// restarts, and the notes/package become immutable release records for auditors
// and downstream tooling.
