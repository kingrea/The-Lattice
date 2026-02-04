# Anchor Docs Module – Manual Test Checklist

Use this checklist whenever you need to verify the module outside of the TUI.

1. **Prepare the project**
   - `export LATTICE_ROOT=/path/to/lattice/source`
   - `cd /path/to/your/project`
   - Remove any previous anchor docs:
     `rm -f .lattice/plan/{COMMISSION.md,ARCHITECTURE.md,CONVENTIONS.md}`
2. **Launch the module runner**
   - From the repo root run
     `go run ./cmd/module-runner --module anchor-docs --project $PWD`
   - Watch the log: it should print the tmux window name it spawned.
3. **Complete the skill session**
   - Switch to the tmux window (e.g. `tmux select-window -t anchor-docs-<ts>`)
   - Follow the lattice-planning skill instructions until the three docs exist.
4. **Verify outputs**
   - Wait for the runner to report "Module completed successfully."
   - Inspect each file and confirm it begins with lattice frontmatter where
     `module: anchor-docs` and `version: 1.0.0`.
   - Optional: run `rg "module: anchor-docs" -n .lattice/plan` to double-check
     the metadata was injected.

If any step fails, rerun the module runner after fixing the underlying issue; it
will restart the tmux session if needed and re-apply frontmatter when the files
appear.
