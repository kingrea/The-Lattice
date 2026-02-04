## Workflow Error Recovery Guide

The workflow engine persists everything it needs to restart safely after a
failure. This guide explains where failure signals come from, how the resolver
and scheduler react, and what the operator should do from the TUI or CLI to get
back to a healthy run.

### Signals when a module fails

- **Engine snapshot** – `.lattice/workflow/engine/state.json` stores the
  workflow definition, runtime overrides, and the last `ModuleRun` result for
  every node.
- **TUI workflow view** – Failed modules show `last run: failed` plus the error
  string returned by `module.Run`. The engine status banner flips to
  `Status: Error · <module-id> failed` so you immediately know which module
  broke.
- **Resolver metadata** – `ModuleStatus.Artifacts` lists every artifact along
  with its `ready/missing/outdated/error` classification. Invalid metadata or
  fingerprint mismatches are attached directly to the module so you can see
  whether the failure came from filesystem drift or from the module itself.
- **Scheduler skips** – When you request runnable modules, the scheduler records
  skip reasons (`not-ready`, `manual-gate`, `concurrency`, `already-running`).
  Those reasons are echoed in the TUI (e.g., `skipped:max parallel 2 reached`)
  so you know whether something is blocked by capacity versus a true failure.

### Example: failure → manual fix → recovery

1. `anchor-docs` runs from the workflow view but crashes because
   `commission-doc` metadata is missing. The module returns `StatusFailed` and
   the engine snapshot records `last_run.status = failed` with the error string.
2. The resolver refresh marks `anchor-docs` as `pending`, and its output
   artifacts are `invalid` (metadata mismatch). All downstream nodes see
   `Blocked by: anchor-docs`.
3. You inspect `/path/to/project/.lattice/modules/MODULES.md`, fix the metadata,
   or re-run the module headlessly:

   ```bash
   module-runner --project /path/to/project --module anchor-docs
   ```

   The CLI uses the same registry/config, so once it finishes the artifacts have
   fresh metadata.

4. Back in the TUI, press `r` to refresh or choose **Resume Work** from the main
   menu. `engine.Resume` reloads the persisted state, the resolver sees that
   `anchor-docs` is now complete, and the scheduler unblocks the next modules.
5. Select the next ready module and continue; the status banner returns to
   `running` or `complete` depending on remaining work.

### Manual intervention tools

- **Rerun a module** – Select it in the workflow view and press `Enter`. The TUI
  calls `engine.Claim` (to mark it running) and immediately executes the module
  in-process. If the module exits with `StatusNeedsInput`, it stays in the
  running set until you provide whatever external input it asked for.
- **Manual gates** – Highlight a module and press `g` to require manual
  approval. Press `a` to toggle approval once you're ready. The scheduler emits
  `SkipReasonManualGate` events until the gate is approved, which is a safe way
  to hold a module while you review artifacts.
- **Skip optional nodes** – For modules declared with `optional: true`, pressing
  `s` removes them from the active targets. The resolver/scheduler stop trying
  to run them, and the engine records the reduced target list so resumptions
  behave identically.
- **Headless retries** – `module-runner` accepts `--config-file` and repeatable
  `--set key=value` overrides, allowing you to patch prompts or reviewer lists
  without editing workflow YAML. The CLI prints `Waiting for <module> outputs…`
  while polling `Module.IsComplete`, so it works for modules that ask for manual
  input (write a file, capture a token, etc.).

### Resuming after restarts or crashes

1. Launch `lattice` and pick **Resume Work (<phase>)** from the main menu.
2. The workflow view calls `engine.Resume` which reloads the last snapshot,
   keeps manual gate state, and immediately refreshes the resolver so any disk
   edits you made while the CLI was closed are captured.
3. If the engine still shows `Status: Error`, select the failed module and press
   `Enter` (or rerun it headlessly). Once it succeeds, press `r` to refresh –
   the status should return to `running` and new runnable modules appear.

### Artifact invalidation reference

- `missing` – File not found. Resolver marks the producing module as `pending`
  and downstream nodes stay blocked until it runs again.
- `invalid` – Metadata does not match the artifact contract (wrong module ID,
  missing version, malformed YAML/JSON). The runtime emits an `InvalidMetadata`
  reason so manual fixes can repair the file before rerunning.
- `outdated` – Stored fingerprint or module version changed. This is the normal
  signal when upstream inputs changed; rerun the module to refresh outputs.
- `error` – Filesystem issues (permission denied, unreadable JSON, etc.).
  Address the underlying filesystem problem, then re-run to confirm the
  artifacts can be read.

Because invalidation events travel through `module.ArtifactInvalidationHandler`
(when a module implements it), modules can clean up their own derived outputs,
log warnings, or notify the operator before the next run.

### Scheduler behaviour recap

- Only modules in `resolver.NodeStateReady` are eligible to run. Any dependency
  stuck in `pending/blocked/error` leaves the dependent module blocked.
- Active modules stay in `Runtime.Running` until they finish with a status other
  than `needs-input`. You can safely restart the CLI: `engine.Resume` keeps that
  running set so duplicate work is avoided.
- Skip reasons recorded in `state.Skipped` surface why a module could not run:
  `not-ready` (resolver says it's still blocked), `manual-gate` (approval flag
  unset), `concurrency` (MaxParallel or exclusive requirements), or
  `already-running`. Clear the underlying condition and press `r` to refresh.

Use this checklist whenever a module fails: inspect the error text, fix the
artifact or configuration, rerun (TUI or `module-runner`), then refresh/resume
so the resolver can recalculate readiness. The workflow engine never requires
manual edits to `state.json`; everything rehydrates from modules plus artifacts.
