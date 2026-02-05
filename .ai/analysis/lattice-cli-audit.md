# Lattice CLI Audit: High-Level Issues & Architecture Analysis

## Executive Summary

After a thorough analysis of the lattice-cli codebase, I've identified several categories of issues that will prevent the CLI from working correctly when run in external projects. The core problem is that **the CLI assumes it's running within the Lattice monorepo itself**, with direct filesystem access to skills, communities, and agent files.

---

## Category 1: Critical Path Mismatches

### 1.1 Agent Files Written vs. Read from Different Locations

**This is the most critical bug.**

| Component | Where It Writes | Where It Reads |
|-----------|----------------|----------------|
| `hiring.go:417` | `.lattice/agents/workers/<slug>/AGENT.md` | — |
| `hiring.go:417` | `.lattice/agents/specialists/<slug>/AGENT.md` | — |
| `workcycle.go:125` | — | `lattice/state/agents/*.md` |

The hiring mode writes agent files to `.lattice/agents/workers/` and `.lattice/agents/specialists/`, but the work cycle loader (`loadProjectAgents`) looks for them in `lattice/state/agents/`.

**Impact**: The work process phase will always fail with "no agent files found" even after hiring completes.

### 1.2 Inconsistent `.lattice/` vs `lattice/` Usage

The codebase inconsistently uses:
- `.lattice/` (hidden directory) - used by config.go, workflow.go
- `lattice/` (visible directory) - used by workcycle.go, hiring.go agent file generation

References:
- `config.go:14`: `const LatticeDir = ".lattice"`
- `workcycle.go:125`: `filepath.Join(o.config.ProjectDir, "lattice", "state", "agents")`
- `workcycle.go:461`: `filepath.Join(o.config.ProjectDir, "lattice", "worktree")`
- `hiring.go:408`: `filepath.Join(ctx.Config.LatticeProjectDir, "agents")` (uses .lattice)

### 1.3 Worker List Stored in Two Locations

- `config.WorkerListPath()` → `.lattice/state/worker-list.json`
- `workflow.WorkersPath()` → `.lattice/workflow/team/workers.json`

Both files track workers but are written/read by different components, leading to potential state inconsistency.

---

## Category 2: LATTICE_ROOT & Distribution Issues

### 2.1 Hardcoded Unix Default Path

```go
// config.go:77-78
latticeRoot := os.Getenv("LATTICE_ROOT")
if latticeRoot == "" {
    latticeRoot = "/mnt/g/The Lattice" // Default for your setup
}
```

The default is a WSL/Unix path, but you're developing on native Windows (`g:\The Lattice`). This means:
- Without `LATTICE_ROOT` set, the CLI will fail to find skills/communities on Windows
- Even with the env var, if distributed to others, they won't have these directories

### 2.2 Skills Are Not Bundled

Skills are loaded at runtime from filesystem paths:

```go
// planning.go:391
skillPath := filepath.Join(ctx.Config.LatticeRoot, "the-terminal", "skills", "lattice-planning", "SKILL.md")

// upcycle.go:198
skillPath := filepath.Join(m.orchestrator.config.LatticeRoot, "the-terminal", "skills", "down-cycle-agent-summarise", "SKILL.md")

// hiring.go:407
skillPath := filepath.Join(ctx.Config.LatticeRoot, "the-terminal", "skills", agentSkillFolder, "SKILL.md")
```

**To distribute the CLI, you'll need to either:**
1. Bundle skills into the binary (embed package)
2. Fetch skills from a remote repository
3. Require users to clone the full Lattice repo and set LATTICE_ROOT

### 2.3 Communities (Agents) Also Not Bundled

```go
// orchestrator.go:198
communitiesDir := o.config.CommunitiesDir() // → LATTICE_ROOT/communities
```

Same issue - denizen CVs are loaded from the filesystem, which won't exist on distributed installs.

### 2.4 AGENTS.md Path Is Lattice-Specific

```go
// upcycle.go:761
agentManual := filepath.Join(m.orchestrator.config.LatticeRoot, "AGENTS.md")

// orchestrator.go:163-164
agentManual := filepath.Join(o.config.ProjectDir, "AGENTS.md")
```

Two different places reference AGENTS.md with different expected locations. The upcycle code expects it in LATTICE_ROOT (the Lattice repo), while orchestrator expects it in the project directory.

---

## Category 3: Go Module & Build Issues

### 3.1 Placeholder Module Path

```go
// go.mod:1
module github.com/kingrea/The-Lattice
```

This is a placeholder that should be changed to your actual module path (e.g., `github.com/your-actual-username/lattice` or a custom import path).

### 3.2 Import References Use Placeholder

All imports use the placeholder:
```go
import "github.com/kingrea/The-Lattice/internal/config"
import "github.com/kingrea/The-Lattice/internal/orchestrator"
// etc.
```

---

## Category 4: Skill Format Inconsistencies

### 4.1 Some Skills Are Files, Others Are Folders

| Skill | Structure |
|-------|-----------|
| `create-agent-file.md` | **File** at `skills/create-agent-file.md` |
| `cv-distill` | **Folder** at `skills/cv-distill/SKILL.md` |
| `lattice-planning` | **Folder** at `skills/lattice-planning/SKILL.md` |
| `down-cycle-summarise` | **Folder** at `skills/down-cycle-summarise/SKILL.md` |
| `final-session-prompt` | **Folder** at `skills/final-session-prompt/SKILL.md` |

The code sometimes references `skillPath + ".md"` and sometimes `skillPath + "/SKILL.md"`, which could cause issues.

---

## Category 5: External Dependencies Not Validated

### 5.1 Required External Commands

The CLI requires these commands to be installed:
- `tmux` - for window management
- `opencode` - for spawning AI agent sessions
- `bd` (beads) - for issue tracking
- `opencode-worktree` - for worktree management (installed via opencode)

None of these are validated at startup, leading to cryptic errors if missing.

### 5.2 Platform Compatibility

- `tmux` is Unix-specific (requires WSL on Windows)
- Commands are spawned without platform-specific handling

---

## Category 6: State & Workflow Issues

### 6.1 Cycle State Storage

```go
// cycle_state.go would need to be checked
// But references show cycle number stored in .lattice/state/
```

Cycle state is persisted but the interaction between different state files needs careful review.

### 6.2 Marker File Proliferation

The workflow uses many marker files:
- `.reviews-applied`
- `.beads-created`
- `.hiring-complete`
- `.in-progress`
- `.complete`
- `.agents-released`
- `.cleanup-done`
- `.orchestrator-released`

These are scattered across different directories and their presence/absence drives phase detection. Any filesystem issue could corrupt workflow state.

---

## Recommended Fixes (Prioritized)

### P0 - Critical (Must Fix Before Use)

1. **Fix agent file path mismatch** between hiring and work process
   - Standardize on one location (recommend `.lattice/agents/`)
   - Update `loadProjectAgents()` to read from the same location hiring writes to

2. **Standardize `.lattice/` vs `lattice/` usage**
   - Pick one (recommend `.lattice/` for all project-specific state)
   - Update all paths consistently

3. **Fix AGENTS.md location**
   - Either copy it to each project or reference from LATTICE_ROOT consistently

### P1 - High (Required for Distribution)

4. **Create skill bundling strategy**
   - Option A: Use Go's `embed` package to bundle SKILL.md files into the binary
   - Option B: Fetch from a public repository at runtime
   - Option C: Require LATTICE_ROOT to point to a cloned repo (document this)

5. **Create community/agent bundling strategy**
   - Same options as skills
   - Consider shipping "starter" denizens with the CLI

6. **Fix Go module path**
   - Change `github.com/kingrea/The-Lattice` to actual path
   - Update all imports

7. **Add LATTICE_ROOT validation**
   - Check if directory exists at startup
   - Provide helpful error message with setup instructions

### P2 - Medium (Improve Reliability)

8. **Add external command validation**
   - Check for `tmux`, `opencode`, `bd` at startup
   - Provide installation instructions for missing tools

9. **Consolidate worker state**
   - Merge `worker-list.json` and `workers.json` or clearly document their different purposes

10. **Standardize skill format**
    - All skills should be folders with SKILL.md inside
    - Or all skills should be single .md files
    - Update all references to be consistent

### P3 - Nice to Have

11. **Add platform detection**
    - Warn on Windows if tmux isn't available via WSL
    - Potentially support alternative windowing approaches

12. **Add state recovery**
    - If marker files get corrupted, provide a way to detect and recover

---

## Architecture Questions for You

1. **Distribution Model**: How do you want to distribute the CLI?
   - Standalone binary (requires bundling/fetching)
   - Requires LATTICE_ROOT pointing to cloned repo
   - Published to package manager

2. **Skill Loading**: Where should skills come from?
   - Bundled in binary
   - Fetched from GitHub at runtime
   - Local filesystem with documented setup

3. **Agent/Community Source**: Same question for denizens
   - Ship starter denizens bundled
   - Users clone communities repo separately
   - Fetch from remote

4. **Windows Support**: Is Windows a target platform?
   - If yes, need to address tmux dependency
   - If no, can simplify to Unix-only

---

## Files Analyzed

- `cmd/lattice/main.go`
- `internal/config/config.go`
- `internal/orchestrator/orchestrator.go`
- `internal/orchestrator/workcycle.go`
- `internal/orchestrator/upcycle.go`
- `internal/orchestrator/cycle_state.go`
- `internal/modes/planning/planning.go`
- `internal/modes/hiring/hiring.go`
- `internal/modes/work_process/work_process.go`
- `internal/workflow/workflow.go`
- `internal/workflow/phase.go`
- `internal/tui/app.go`
- `the-terminal/skills/*` (all skill files)
- `communities/The Lumen/rituals/**/*` (all ritual files)

---

*Analysis completed: 2026-02-02*
