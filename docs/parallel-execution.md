## Parallel Execution Strategy

The workflow engine now treats module execution as a capacity-managed worker
queue. Independent modules can run concurrently as long as dependency order and
resource constraints are satisfied. This document explains how the coordinator
decides which modules may run together, how operators can configure capacity,
and how modules describe their own concurrency requirements.

### Queueing model

1. **Resolver snapshot** – The resolver evaluates every module instance to mark
   it `ready`, `blocked`, `complete`, or `error`.
2. **Scheduler filter** – The scheduler walks the ready queue, removes nodes
   that are still blocked (manual gates, invalid artifacts, missing inputs), and
   applies concurrency constraints:
   - `MaxParallel` defines the total number of _slots_ that may run at once.
   - Each module consumes either one slot (default) or a custom slot cost via
     `module.Info.Concurrency.Slots`.
   - Modules may set `Concurrency.Exclusive = true` to require exclusive access.
3. **Work claims** – Workers call `engine.Claim` to reserve runnable modules.
   The engine marks the claimed modules as `running` and persists the snapshot
   so other workers see the updated capacity. Claims may be filtered to specific
   module IDs, enabling the Bubble Tea UI to run the operator-selected module
   while still honoring central constraints.
4. **Completion updates** – When work finishes, operators call `engine.Update`
   with `ModuleStatusUpdate` entries. Any result other than `needs-input`
   releases the slot so new claims may proceed.

This handshake guarantees that concurrent workers cannot oversubscribe shared
resources—`Claim` is the single gateway that mutates the running set.

### Configuring workflow parallelism

Workflows can declare their default concurrency budget in the definition file:

```yaml
id: commission-work
runtime:
  max_parallel: 3
modules:
  - id: anchor-docs
    module: anchor-docs
  # ...
```

`runtime.max_parallel` sets the total slot capacity for the workflow. Operators
can override this at runtime via `engine.RuntimeOverrides.MaxParallel` (e.g.,
the Bubble Tea “Workflow” view or automation tooling) to temporarily scale
capacity up or down.

### Module concurrency hints

Modules declare their concurrency needs through `module.Info.Concurrency`:

```go
info := module.Info{
    ID:      "parallel-reviews",
    Name:    "Parallel Reviews",
    Version: "1.0.0",
    Concurrency: module.ConcurrencyProfile{
        Slots: 4,          // consumes four scheduler slots
        Exclusive: false,  // set to true for exclusive execution
    },
}
```

- `Slots` controls how much of the `MaxParallel` budget the module occupies.
  Values <= 0 default to one slot.
- `Exclusive` enforces mutual exclusion; the scheduler will not run anything
  else while an exclusive module is active, even if slots remain.

Modules with expensive resource footprints (e.g., `parallel-reviews` spawning
four tmux windows) should bump their slot cost or request exclusivity so the
engine can throttle other work automatically.

### Worker coordination APIs

- `engine.Claim(ctx, ClaimRequest)` – Returns `ClaimResult` containing
  `WorkClaim` entries and the updated engine state. Use `Limit` to cap how many
  modules you want to reserve and `Modules` to request specific workflow IDs.
- `engine.Update(ctx, UpdateRequest)` – Records module results and releases
  slots for any claim that finished (`completed`, `failed`, or `no-op`).
  `needs-input` results remain in the running set so the operator can resume
  later.

Adopting this pattern allows the CLI UI, tmux workers, or external automation to
cooperate without race conditions: every worker consults the same persisted
state, and claims are serialized through the engine.
