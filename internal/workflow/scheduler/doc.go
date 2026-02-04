// Package scheduler turns resolver snapshots into runnable batches that respect
// dependency order plus runtime constraints such as concurrency limits and
// manual approvals. It is a thin layer that higher-level engines can call to
// decide which modules to execute next without re-implementing filtering logic.
package scheduler
