# lattice-cli — Senior Developer Audit

## Context

- Repo: `lattice-cli`
- Date: 2026-02-02
- Scope: architecture & workflow review with focus on orchestration lifecycle

## Key Findings

1. **tmux launch failures are silenced**  
   The entrypoint attaches to or creates the controlling tmux session by calling
   `cmd.Run()` but discards the returned error (`cmd/lattice/main.go:84-103`).
   When `tmux` is missing, the session name is taken, or the command fails for
   any reason, the CLI prints a success message and keeps running until later Go
   code panics because no session exists. Users get no actionable output.
   - _Impact_: Hard-to-diagnose startup failures; impossible to bubble
     actionable exit codes to scripts.
   - _Suggestion_: capture and bubble the error (e.g.,
     `if err := cmd.Run(); err != nil { fmt.Fprintf(os.Stderr, ...) ; os.Exit(1) }`).

2. **`LATTICE_ROOT` defaults to a single WSL path**  
   `internal/config/config.go:78-85` hard-codes `/mnt/g/The Lattice` whenever
   `LATTICE_ROOT` is not set. On macOS/Linux hosts or even other WSL drives the
   CLI silently points at a directory that probably does not exist, so community
   CVs, skills, and logs are never found.
   - _Impact_: Non-WSL users cannot run the CLI without setting env vars, and
     the failure mode is mysterious (missing skills/CVs elsewhere).
   - _Suggestion_: fail fast when the env var is absent, or derive the default
     relative to the binary location; add validation that `LatticeRoot` exists
     before continuing.

3. **Mode-level cancellation never runs, leaving automation running headless**  
   The bubbletea app intercepts `Esc` globally and returns to the main menu
   before forwarding the key to the active mode (`internal/tui/app.go:156-175`).
   Every mode implements its own cancellation logic—killing tmux windows and
   emitting errors when `Esc` is pressed
   (`internal/modes/planning/planning.go:198-235`,
   `internal/modes/work_process/work_process.go:65-75`, etc.)—but those handlers
   never execute. When a user tries to abort a phase, the UI disappears while
   tmux windows, opencode prompts, and hiring/work cycles keep running with no
   cleanup or feedback.
   - _Impact_: Lattice cannot be stopped safely; automation keeps mutating the
     repository after the operator has already exited the mode.
   - _Suggestion_: either stop intercepting `Esc` at the app level or propagate
     it to the active mode and wait for its cleanup to finish before returning
     to the main menu. The mode should control how and when it tears down
     windows and goroutines.

4. **Release/cleanup modes are placeholders that only touch marker files**  
   `internal/modes/agent_release/agent_release.go`, `internal/modes/work_cleanup/work_cleanup.go`,
   and `internal/modes/orchestrator_release/orchestrator_release.go` all
   acknowledge "TODO" comments and merely create `.agents-released`,
   `.cleanup-done`, and `.orchestrator-released` marker files. README copy
   advertises multi-step release, cleanup, and orchestration handoffs, but no
   logs are archived, no worktrees are cleaned, and agents are not actually
   detached.
   - _Impact_: Commission close-out does nothing beyond creating empty marker
     files, so state is never cleared and follow-on sessions will misread the
     workflow phase.
   - _Suggestion_: Either scope the README to match reality or implement the
     documented behavior (close tmux windows, archive logs, delete worktrees,
     reset worker lists, etc.) before writing the markers.

5. **Build script mutates module files**  
   `build.sh` runs `go mod tidy` on every build (`build.sh:18-21`). That command
   rewrites `go.mod`/`go.sum`, so a simple `./build.sh` dirties the worktree and
   can even drop replace directives developers rely on.
   - _Impact_: Accidental module churn in CI and local builds; developers cannot
     trust clean working trees after building.
   - _Suggestion_: move `go mod tidy` into a dedicated task (or CI) and keep
     `./build.sh` side-effect free beyond compilation.

## Recommended Next Steps

- Surface tmux/env problems immediately: validate `LATTICE_ROOT`, check that the
  path exists, and stop the program when `tmux` commands fail.
- Allow active modes to handle cancellation (propagate `Esc`, wait for their
  cleanup routines, and make sure background goroutines stop when users exit).
- Flesh out release/cleanup implementations or pare back the advertised surface
  area until the promised behavior exists.
- Update build tooling to avoid unwanted edits (drop `go mod tidy` from the
  build script).
- Add regression coverage for tmux start-up, cancellation, and release flows so
  these regressions are caught automatically.
