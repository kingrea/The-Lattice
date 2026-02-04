# Lattice - Terminal Orchestration System

A TUI (Terminal User Interface) for orchestrating AI agent workflows via tmux.

## Prerequisites

- Go 1.21+ installed
- tmux installed
- WSL (if on Windows)
- OpenCode CLI (adjust the command in `orchestrator.go` to match your setup)
- OpenCode `opencode-worktree` plugin (install with
  `opencode install opencode-worktree`)

## Quick Start

```bash
# 1. Clone this repo
git clone https://github.com/the-lattice/lattice.git
cd lattice

# 2. Build
chmod +x build.sh
./build.sh install

# 3. Add to PATH and set LATTICE_ROOT (add to ~/.bashrc or ~/.zshrc)
export PATH="$HOME/.local/bin:$PATH"
export LATTICE_ROOT="$(pwd)"  # Point to your Lattice installation

# 4. Run from any project!
cd /path/to/some/project
lattice
```

## How It Works

```
┌────────────────────────────────────────────────────┐
│                                                    │
│   You run: $ lattice                               │
│   From: /path/to/some/project                      │
│                                                    │
└───────────────────────┬────────────────────────────┘
                        │
                        ▼
┌────────────────────────────────────────────────────┐
│  tmux session: "lattice"                           │
│                                                    │
│  ┌──────────────────────────────────────────────┐  │
│  │  Window 0: "terminal"                        │  │
│  │  ┌────────────────────────────────────────┐  │  │
│  │  │  ⬡ THE TERMINAL                        │  │  │
│  │  │                                        │  │  │
│  │  │  > Commission Work                     │  │  │
│  │  │    View Agents                         │  │  │
│  │  │    Settings                            │  │  │
│  │  │    Exit                                │  │  │
│  │  │                                        │  │  │
│  │  └────────────────────────────────────────┘  │  │
│  └──────────────────────────────────────────────┘  │
│                                                    │
└────────────────────────────────────────────────────┘

               [Commission Work] pressed
                        │
                        ▼

┌────────────────────────────────────────────────────┐
│  tmux session: "lattice"                           │
│                                                    │
│  ┌───────────────────┐  ┌────────────────────────┐ │
│  │ Window 0          │  │ Window 1: "worker"     │ │
│  │ (waiting...)      │  │                        │ │
│  │                   │  │  $ opencode --prompt   │ │
│  │                   │  │    "Call MCP server.." │ │
│  │                   │  │                        │ │
│  │                   │  │  [Running...]          │ │
│  │                   │  │                        │ │
│  └───────────────────┘  └────────────────────────┘ │
│                                                    │
└────────────────────────────────────────────────────┘

                OpenCode completes
                        │
                        ▼

┌────────────────────────────────────────────────────┐
│  Window 1 killed, back to Window 0                 │
│                                                    │
│  ┌──────────────────────────────────────────────┐  │
│  │  Select an Agent:                            │  │
│  │                                              │  │
│  │  > Orchestrator Alpha                        │  │
│  │    Code Specialist                           │  │
│  │    Research Agent                            │  │
│  │                                              │  │
│  │  Work complete! Select an agent to continue. │  │
│  └──────────────────────────────────────────────┘  │
│                                                    │
└────────────────────────────────────────────────────┘
```

## Project Structure

```
lattice/
├── cmd/
│   └── lattice/
│       └── main.go           # Entry point
├── internal/
│   ├── tui/
│   │   └── app.go            # The terminal UI (bubbletea)
│   ├── orchestrator/
│   │   └── orchestrator.go   # tmux + OpenCode management
│   └── config/
│       └── config.go         # Configuration handling
├── build.sh                  # Build script
├── go.mod
└── README.md
```

## Files Created in Your Project

When you run `lattice` in a project, it creates:

```
your-project/
├── .lattice/
│   ├── setup/
│   │   └── cvs/              # Agent CVs written here
│   ├── logs/                 # Orchestration logs
│   └── state/                # Persistent state
└── ... your project files
```

## Configuration

### Required: LATTICE_ROOT

The `LATTICE_ROOT` environment variable **must** be set to point to your Lattice
CLI installation directory (where this repo is cloned). This is used to locate:

- Core agent definitions (`agents/core/`)
- Workflow skills (`skills/`)
- Default community configuration (`defaults/community.yaml`)

```bash
# Add to ~/.bashrc or ~/.zshrc
export LATTICE_ROOT="/path/to/lattice"

# Examples:
# Linux/WSL:  export LATTICE_ROOT="/mnt/g/lattice"
# macOS:      export LATTICE_ROOT="$HOME/projects/lattice"
# Windows:    set LATTICE_ROOT=G:\lattice
```

Without this variable, the CLI will fail to find required assets.

### Workflow Definitions

Workflows are now driven by the workflow engine. The TUI loads YAML definitions
from `<project>/workflows/` or from `${LATTICE_ROOT}/workflows/`. The default
`commission-work` definition lives in `workflows/commission-work.yaml` in this
repository. Each definition lists the modules to run plus their dependencies.

- Selecting **Commission Work** starts a fresh engine run using the configured
  workflow definition.
- Selecting **Resume Work** calls the engine's resume path so it can refresh the
  dependency graph from disk.
- The workflow pane shows ready modules, running modules, and manual gate
  status. Use the inline key bindings to run modules, approve manual gates, or
  skip optional nodes.

To customize the workflow, add additional YAML definitions under `workflows/`
and point `.lattice/config.yaml` → `workflows.default` at the new ID.

Every module entry in the YAML may include a `config` map. The keys/values are
opaque to the runtime but they are passed straight into the module factory as a
`module.Config`. Example:

```yaml
modules:
  - id: parallel-reviews
    module: parallel-reviews
    depends_on: [staff-incorporate]
    config:
      reviewers:
        - pragmatist
        - advocate
      openai_model: gpt-4.1
```

Those overrides live in the workflow definition so the TUI, resolver, and the
headless CLI all see the same configuration.

## Error Recovery

The workflow engine persists a snapshot of every run under
`.lattice/workflow/engine/state.json`. When a module fails, the workflow view
shows which node broke (`last run: failed`) and the engine status banner flips
to `Status: Error`. Recovery is deterministic:

1. Inspect the failed module in the workflow view to read the stored error
   message and artifact status.
2. Fix the underlying issue (edit the artifact, adjust config, or rerun the
   module with the built-in workflow view or headless via
   `module-runner --project <dir> --module <module-id>`).
3. Press `r` in the workflow view or start `lattice` and select **Resume Work**
   so the engine refreshes the resolver snapshot, unblocks downstream modules,
   and returns to the `running` status once everything is healthy.

Manual gates (`g`/`a`), optional module skipping (`s`), and the persisted
`state.json` make it safe to pause, fix artifacts, and resume without touching
internal files. See `docs/error-recovery.md` for the full walkthrough covering
resolver invalidation events, scheduler skip reasons, and CLI-driven recoveries.

## Customization

### Module configuration overrides

Operators can tune module behavior via three layers:

1. **Environment** – set `LATTICE_ROOT` (required),
   `LATTICE_PLUGIN_AUTO_INSTALL` (controls automatic OpenCode plugin installs),
   or `LATTICE_ASSIGN_SPARK` (allow Spark agents during work-cycle planning).
2. **Project + workflow files** – `.lattice/config.yaml` selects the default
   workflow and community sources, while `workflows/<id>.yaml` supplies the
   `modules[].config` map shown above.
3. **CLI overrides** – the `module-runner` binary accepts `--config-file` (YAML
   or JSON map) and repeatable `--set key=value` flags. The CLI builds the same
   `module.Config` map the TUI would pass to `module.Registry.Resolve`, and
   inline `--set` pairs win over the file when both are provided.

Example headless run with overrides:

```bash
module-runner \
  --project /path/to/project \
  --module parallel-reviews \
  --config-file overrides/reviewers.yaml \
  --set reviewer_mode=fast-track
```

### Changing the OpenCode command

### Changing the OpenCode command

Edit `internal/orchestrator/orchestrator.go`, specifically the `runOpenCode()`
function. Adjust the command to match how you invoke OpenCode.

### Adding MCP server integration

The orchestrator currently sends a prompt to OpenCode. You'll want to:

1. Update the prompt in `runOpenCode()` to match your MCP server's capabilities
2. Or, directly call your MCP server from Go if you prefer

### Adding new menu items

Edit `internal/tui/app.go`:

1. Add items to `menuItems` in `NewApp()`
2. Handle the new item in `handleSelection()`

## Key Bindings

| Key        | Action        |
| ---------- | ------------- |
| ↑/↓        | Navigate menu |
| Enter      | Select item   |
| Esc        | Go back       |
| q / Ctrl+C | Quit          |

## OpenCode Plugin Installation

Lattice needs the `opencode-worktree` plugin to manage worktrees. By default it
will attempt to install it by running `opencode install opencode-worktree`. If
you prefer to install the plugin yourself (for example when npm needs elevated
privileges), set `LATTICE_PLUGIN_AUTO_INSTALL=0` in your environment and install
the plugin manually with `opencode install opencode-worktree` (or
`npm install -g opencode opencode-worktree`).

## Next Steps

- [ ] Implement agent selection action
- [ ] Add actual MCP server integration
- [ ] Add settings screen
- [ ] Add logging
- [ ] Add persistent state (remember last project, etc.)
