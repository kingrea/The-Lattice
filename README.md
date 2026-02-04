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

## Customization

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
