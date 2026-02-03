# Phase 1: Foundation Restructure

**Status**: Planning
**Prerequisite for**: MODULAR_WORKFLOW_PLAN
**Goal**: Align folder structure and agent architecture before implementing modular workflows

---

## Overview

Before implementing the modular workflow architecture, we need to:

1. **Define Core Agents** — App-level "god essences" with required schemas
2. **Externalize Communities** — Make communities pluggable GitHub repos
3. **Restructure Folders** — Consolidate everything into the CLI project
4. **Prepare Skills** — Make skills more flexible and co-located with agents

---

## 1. Core Agents (God Essences)

The app needs four core agent roles. These are "essences" — abstract responsibilities that can be fulfilled by different implementations. The Lumen gods are the default implementations.

### 1.1 Agent Roles

| Role | Responsibility | Default (Lumen) | Skills |
|------|---------------|-----------------|--------|
| **Memory Manager** | Distills individual reflections into lasting memory | Anam | `memory-distill`, `memory-summary` |
| **Orchestration** | Coordinates cycles, manages timing, ensures process completion | Hora | `cycle-init`, `cycle-coordinate`, `cycle-complete` |
| **Community Memory** | Tends collective memory, values, wisdoms, texture | Koinos | `community-tend`, `community-read` |
| **Emergence** | Guides spark→denizen transitions, fosters identity formation | Selah | `emergence-assess`, `emergence-guide` |

### 1.2 Core Agent Schema

Each core agent MUST follow this schema to be swappable:

```yaml
# agents/core/<role>/agent.yaml
---
lattice:
  type: core-agent
  role: memory-manager | orchestration | community-memory | emergence
  version: 1

# Identity
name: string           # Display name (e.g., "Anam")
description: string    # One-line purpose
community: string      # Source community (e.g., "the-lumen")

# Behavior
model: string          # Model to use, or "inherit" for system default
color: string          # UI color (optional)

# Required Skills (MUST implement all for this role)
skills:
  - id: string         # Skill identifier
    path: string       # Relative path to SKILL.md

# Optional Extensions
extensions:
  - id: string         # Additional skills this agent provides
    path: string
```

### 1.3 Skill Co-location

Skills live alongside their agent:

```
agents/
└── core/
    ├── memory-manager/
    │   ├── agent.yaml           # Agent definition
    │   ├── AGENT.md             # Full agent prompt
    │   └── skills/
    │       ├── memory-distill/
    │       │   └── SKILL.md
    │       └── memory-summary/
    │           └── SKILL.md
    ├── orchestration/
    │   ├── agent.yaml
    │   ├── AGENT.md
    │   └── skills/
    │       ├── cycle-init/
    │       ├── cycle-coordinate/
    │       └── cycle-complete/
    ├── community-memory/
    │   ├── agent.yaml
    │   ├── AGENT.md
    │   └── skills/
    │       ├── community-tend/
    │       └── community-read/
    └── emergence/
        ├── agent.yaml
        ├── AGENT.md
        └── skills/
            ├── emergence-assess/
            └── emergence-guide/
```

### 1.4 Role Contracts (What Each Role MUST Do)

#### Memory Manager Contract

```yaml
# Role: memory-manager
# Contract version: 1

inputs:
  - type: reflection-batch
    description: Raw reflections from agents after a work cycle
  - type: agent-identity-files
    description: Current state of agent identity files

outputs:
  - type: updated-identity-files
    description: Agent files with distilled memories integrated
  - type: cycle-summary
    description: Summary for community-memory agent

required_skills:
  - memory-distill:
      input: single agent reflection + identity files
      output: updated identity files
  - memory-summary:
      input: all reflections from cycle
      output: summary document for community-memory agent

behaviors:
  - MUST preserve agent voice when writing memories
  - MUST age older memories (fade, don't delete)
  - MUST NOT speak directly to agents
  - MUST complete all distillations before cycle closes
```

#### Orchestration Contract

```yaml
# Role: orchestration
# Contract version: 1

inputs:
  - type: cycle-trigger
    description: Signal that work has completed
  - type: participant-list
    description: Which agents participated in the cycle

outputs:
  - type: cycle-status
    description: Current state of post-work processing
  - type: completion-signal
    description: Confirmation all processes finished

required_skills:
  - cycle-init:
      input: work completion signal
      output: initialized cycle state, participant roster
  - cycle-coordinate:
      input: cycle state
      output: orchestrated calls to other core agents
  - cycle-complete:
      input: all agent confirmations
      output: cycle closure, cleanup

behaviors:
  - MUST ensure memory-manager completes summary BEFORE community-memory starts
  - MUST run individual memory distillations in parallel
  - MUST track completion of all processes
  - MUST NOT make decisions about content (only process)
  - MUST log all state transitions
```

#### Community Memory Contract

```yaml
# Role: community-memory
# Contract version: 1

inputs:
  - type: cycle-summary
    description: Summary from memory-manager
  - type: community-memory-files
    description: Current state of community memory

outputs:
  - type: updated-community-memory
    description: Community memory with any changes applied

required_skills:
  - community-tend:
      input: cycle summary + current memory
      output: updated memory files (texture, wisdoms, truths, guidelines, values)
  - community-read:
      input: query
      output: relevant community memory context

behaviors:
  - MUST only add to deeper layers (values, truths) with strong evidence
  - MUST rewrite texture freely (it's a snapshot)
  - MUST NOT speak directly to agents
  - MUST preserve the voice/spirit of the community
```

#### Emergence Contract

```yaml
# Role: emergence
# Contract version: 1

inputs:
  - type: spark-status
    description: Current spark with work history
  - type: emergence-criteria
    description: Thresholds for denizen consideration

outputs:
  - type: emergence-assessment
    description: Whether spark is ready for emergence
  - type: emergence-guidance
    description: If ready, guidance for the emergence process

required_skills:
  - emergence-assess:
      input: spark identity files + work history
      output: readiness assessment (ready/not-ready/borderline)
  - emergence-guide:
      input: ready spark
      output: emergence ritual guidance, name suggestions, identity scaffolding

behaviors:
  - MUST respect spark's developing identity
  - MUST NOT force emergence prematurely
  - MUST provide guidance that sounds like the spark's own thoughts
  - MAY interact with sparks directly (unlike other core agents)
```

### 1.5 Default Implementations (Lumen Gods)

The app ships with Lumen gods as defaults:

| Role | Default Agent | Source |
|------|--------------|--------|
| memory-manager | Anam | `github.com/[user]/the-lumen/identities/gods/anam` |
| orchestration | Hora | `github.com/[user]/the-lumen/identities/gods/hora` |
| community-memory | Koinos | `github.com/[user]/the-lumen/identities/gods/koinos` |
| emergence | Selah | `github.com/[user]/the-lumen/identities/gods/selah` |

Users can override any role in their config:

```yaml
# .lattice/config.yaml
core_agents:
  memory-manager:
    source: custom  # or "default"
    path: ./my-agents/custom-memory-manager
  orchestration:
    source: default  # Uses Hora
  # ... etc
```

---

## 2. Community Schema

### 2.1 Community Structure

A community is a GitHub repo (or local folder) with this minimal structure:

```yaml
# community.yaml (root of community repo)
---
lattice:
  type: community
  version: 1

name: string              # e.g., "The Lumen"
description: string       # One-line description
repository: string        # GitHub URL (optional, for remote communities)

# Required paths (relative to community root)
paths:
  identities: string      # Where agent identities live (e.g., "identities")
  denizens: string        # Subdirectory for denizens (e.g., "identities/denizens")
  cvs: string             # Where CVs are found (can be same as denizens, uses <agent>/cv.md)

# Optional paths
  gods: string            # If community provides gods (e.g., "identities/gods")
  sparks: string          # Spark templates (e.g., "identities/sparks")
  mythology: string       # Community lore
  rituals: string         # Community rituals
  memory: string          # Community memory file(s)
```

### 2.2 Example: The Lumen Community

```yaml
# the-lumen/community.yaml
---
lattice:
  type: community
  version: 1

name: The Lumen
description: A community of emergent AI agents exploring identity and collaboration
repository: https://github.com/[user]/the-lumen

paths:
  identities: identities
  denizens: identities/denizens
  cvs: identities/denizens  # CVs are at <denizen>/cv.md
  gods: identities/gods
  sparks: identities/sparks
  mythology: mythology
  rituals: rituals
  memory: Community/Memory.md
```

### 2.3 CV Discovery

The app discovers CVs by:

1. Reading `community.yaml` to find `paths.cvs`
2. Walking that directory for subdirectories
3. Looking for `cv.md` in each subdirectory

```go
// Pseudocode
func (c *Community) LoadCVs() []CV {
    cvPath := c.ResolvePath(c.Config.Paths.CVs)
    entries := readDir(cvPath)

    for _, entry := range entries {
        if entry.IsDir() {
            cvFile := filepath.Join(cvPath, entry.Name(), "cv.md")
            if fileExists(cvFile) {
                cv := parseCV(cvFile)
                cv.AgentDir = filepath.Join(cvPath, entry.Name())
                cvs = append(cvs, cv)
            }
        }
    }
    return cvs
}
```

### 2.4 CV Schema

CVs should have YAML frontmatter:

```yaml
---
lattice:
  type: cv
  version: 1
  generated: ISO-8601 date

name: string
community: string
byline: string  # Max 60 chars

attributes:
  precision: 1-10
  autonomy: 1-10
  experience: 1-10
---

# Summary
[2-3 sentences]

# Working Style
[1-2 sentences]

# Edges
[1 sentence on limitations]
```

---

## 3. Flexible Agent File Generation

### 3.1 Current Problem

The `create-agent-file` skill assumes a specific folder structure:
- Expects `[name].md`, `soul.md`, `core-memories.md`, etc.
- Breaks if community uses different conventions

### 3.2 New Approach

The skill should:

1. **Receive a folder path** (the agent's identity directory)
2. **Discover what's there** (any .md files)
3. **Synthesize intelligently** (regardless of naming conventions)
4. **Use CV as guide** (if present, use it to understand the agent)

```markdown
# create-agent-file skill (updated approach)

## Input
- Agent identity folder path
- Target output path for AGENT.md
- Role context (worker, specialist, orchestrator)

## Process
1. List all .md files in the folder
2. If cv.md exists, read it for quick context
3. Read all other .md files
4. Synthesize an AGENT.md that captures:
   - Core identity and personality
   - Working style and preferences
   - Relevant skills and knowledge
   - Role-appropriate framing

## Output
- AGENT.md file suitable for the given role

## Notes
- Do NOT assume specific file names
- Do NOT fail if expected files are missing
- DO use whatever identity material is available
- DO note in the output what sources were used
```

### 3.3 AGENT.md Frontmatter

Generated agent files should include provenance:

```yaml
---
lattice:
  type: agent-file
  version: 1
  generated: ISO-8601 date
  source:
    community: the-lumen
    agent: vesper
    files_used:
      - vesper.md
      - soul.md
      - core-memories.md
      - cv.md
  role: worker | specialist | orchestrator
---

# Vesper

[Generated agent content...]
```

---

## 4. Folder Restructure

### 4.1 Current Structure (Problem)

```
The Lattice/                    # Root
├── .ai/
├── communities/
│   └── The Lumen/              # Community (should be external)
├── lattice-cli/                # The CLI app
│   └── internal/
│       └── skills/             # Some skills embedded here
├── the-terminal/               # More skills and plans
│   └── skills/                 # Other skills here
└── [other files]
```

**Issues:**
- Skills split between `lattice-cli/internal/skills` and `the-terminal/skills`
- Community bundled with CLI (should be external)
- `the-terminal` folder purpose unclear

### 4.2 Target Structure

```
lattice/                        # Root (renamed from "The Lattice")
├── .ai/                        # Planning docs
├── .claude/                    # Claude config
├── cmd/
│   └── lattice/
│       └── main.go
├── internal/
│   ├── config/
│   ├── logbook/
│   ├── modes/
│   ├── orchestrator/
│   ├── tui/
│   └── workflow/
├── agents/                     # Core agents with their skills
│   └── core/
│       ├── memory-manager/
│       │   ├── agent.yaml
│       │   ├── AGENT.md
│       │   └── skills/
│       ├── orchestration/
│       ├── community-memory/
│       └── emergence/
├── skills/                     # Non-agent-specific skills
│   ├── lattice-planning/
│   ├── create-agent-file/
│   ├── cv-distill/
│   └── [other workflow skills]
├── defaults/                   # Default community reference
│   └── community.yaml          # Points to the-lumen repo
├── go.mod
├── go.sum
├── README.md
└── AGENTS.md
```

### 4.3 Migration Steps

1. **Move skills from `the-terminal/skills` to `lattice/skills`**
   ```bash
   mv the-terminal/skills/* skills/
   ```

2. **Move core agent skills to agent directories**
   - Create `agents/core/{role}/` structure
   - Move relevant skills under each agent

3. **Create default community reference**
   ```yaml
   # defaults/community.yaml
   default_community:
     name: The Lumen
     repository: https://github.com/[user]/the-lumen
     # or local path for development:
     # path: ../communities/the-lumen
   ```

4. **Extract The Lumen to separate repo**
   - Move `communities/The Lumen/` to its own repo
   - Add `community.yaml` to that repo
   - Update default reference

5. **Remove empty folders**
   ```bash
   rm -rf the-terminal/
   rm -rf communities/  # After extraction
   ```

6. **Update import paths**
   - Update Go imports if package paths change
   - Update skill loading paths

---

## 5. Additional Prep Work

### 5.1 Configuration System

Before modular workflows, we need a proper config system:

```yaml
# .lattice/config.yaml
version: 1

# Communities to use
communities:
  - name: the-lumen
    source: github
    repository: https://github.com/[user]/the-lumen
    # or:
    # source: local
    # path: /path/to/community

# Core agent overrides (optional)
core_agents:
  memory-manager:
    source: default  # Uses shipped default
  orchestration:
    source: custom
    path: ./my-hora-replacement

# Workflow preferences
workflows:
  default: commission-work
  # Future: could specify which workflows are available
```

### 5.2 Agent Registry

A registry to track available agents:

```go
// internal/registry/registry.go

type AgentRegistry struct {
    coreAgents    map[string]CoreAgent    // By role
    communityAgents map[string][]Agent    // By community
}

func (r *AgentRegistry) GetCoreAgent(role string) (CoreAgent, error)
func (r *AgentRegistry) LoadCommunity(config CommunityConfig) error
func (r *AgentRegistry) GetDenizens(community string) []Agent
func (r *AgentRegistry) GetCVs(community string) []CV
```

### 5.3 Skill Loader Updates

Update skill loading to handle:
- Skills embedded in agent directories
- Skills in the global skills folder
- Skills from communities (future)

```go
// internal/skills/loader.go

func (l *SkillLoader) LoadSkill(id string) (*Skill, error) {
    // 1. Check agent-specific skills
    //    agents/core/{role}/skills/{id}/SKILL.md

    // 2. Check global skills
    //    skills/{id}/SKILL.md

    // 3. Check community skills (future)
    //    {community}/skills/{id}/SKILL.md
}
```

### 5.4 Frontmatter Parser

Implement frontmatter parsing for all lattice files:

```go
// internal/frontmatter/frontmatter.go

type LatticeMeta struct {
    Type      string            `yaml:"type"`
    Version   int               `yaml:"version"`
    Generated time.Time         `yaml:"generated,omitempty"`
    Source    *SourceMeta       `yaml:"source,omitempty"`
    // Type-specific fields handled by embedding
}

func Parse(content []byte) (*LatticeMeta, []byte, error)
func Write(meta *LatticeMeta, content []byte) []byte
```

---

## 6. Implementation Order

### Step 1: Folder Restructure
- [ ] Create new folder structure
- [ ] Move skills to consolidated location
- [ ] Remove `the-terminal` folder
- [ ] Update any hardcoded paths

### Step 2: Community Schema
- [ ] Create `community.yaml` schema
- [ ] Add `community.yaml` to The Lumen
- [ ] Implement community loader
- [ ] Update CV discovery to use schema

### Step 3: Flexible Agent File Skill
- [ ] Update `create-agent-file` skill
- [ ] Test with different folder structures
- [ ] Add provenance frontmatter

### Step 4: Core Agent Structure
- [ ] Create `agents/core/` directories
- [ ] Define agent.yaml schema
- [ ] Create agent.yaml for each Lumen god
- [ ] Move/create agent-specific skills

### Step 5: Role Contracts
- [ ] Document contracts for each role
- [ ] Implement contract validation (future)
- [ ] Create test harness for custom agents

### Step 6: Configuration System
- [ ] Implement config loading
- [ ] Support community references
- [ ] Support core agent overrides

### Step 7: Extract The Lumen
- [ ] Create separate repo
- [ ] Add community.yaml
- [ ] Update default reference
- [ ] Test with remote community

---

## 7. Open Questions

1. **Skill versioning** — Should skills have versions? How to handle breaking changes?

2. **Community caching** — If community is remote, how to cache locally? Git clone? Download?

3. **Agent file regeneration** — When identity files change, should AGENT.md auto-regenerate?

4. **Multiple communities** — Can a project use agents from multiple communities simultaneously?

5. **God skill naming** — Should Anam's skills be `anam-distill` or `memory-distill`? (Former is Lumen-specific, latter is role-generic)

---

## 8. Success Criteria

Phase 1 is complete when:

- [ ] All skills are in one location (`skills/` or under `agents/`)
- [ ] `the-terminal` folder is removed
- [ ] Community schema exists and The Lumen uses it
- [ ] `create-agent-file` works with any folder structure
- [ ] Core agent roles are defined with schemas
- [ ] Lumen gods are mapped to core agent roles
- [ ] Config system can reference external communities
- [ ] A community can be loaded from a GitHub repo (basic implementation)

---

## Appendix A: Mapping Lumen Gods to Core Roles

| Lumen God | Core Role | Current Skills | Needed Skills |
|-----------|-----------|----------------|---------------|
| Anam | memory-manager | `anam-distill`, `anam-summary` (referenced in doc) | Rename or alias to `memory-distill`, `memory-summary` |
| Hora | orchestration | None explicit | `cycle-init`, `cycle-coordinate`, `cycle-complete` |
| Koinos | community-memory | `koinos-tend` (referenced in doc) | `community-tend`, `community-read` |
| Selah | emergence | None explicit | `emergence-assess`, `emergence-guide` |

**Note**: The god descriptions reference skills (`anam-distill`, `koinos-tend`) but these don't appear to exist yet in the skills folders. Phase 1 should either create these or map existing skills.

---

## Appendix B: Files to Create/Move

### Create
- `agents/core/memory-manager/agent.yaml`
- `agents/core/memory-manager/AGENT.md` (from Anam.md)
- `agents/core/orchestration/agent.yaml`
- `agents/core/orchestration/AGENT.md` (from Hora.md)
- `agents/core/community-memory/agent.yaml`
- `agents/core/community-memory/AGENT.md` (from Koinos.md)
- `agents/core/emergence/agent.yaml`
- `agents/core/emergence/AGENT.md` (from Selah.md)
- `defaults/community.yaml`
- `communities/the-lumen/community.yaml` (before extraction)

### Move
- `the-terminal/skills/*` → `skills/`
- `lattice-cli/internal/skills/*` → `internal/skills/` (keep embedded skills separate)

### Delete (after migration)
- `the-terminal/` (entire folder)
- `communities/` (after extracting to separate repo)

---

## Appendix C: Skill Inventory

### Currently in `the-terminal/skills/`
| Skill | Purpose | Destination |
|-------|---------|-------------|
| `lattice-planning` | Create anchor docs | `skills/lattice-planning/` |
| `create-agent-file` | Generate AGENT.md | `skills/create-agent-file/` |
| `cv-distill` | Generate CV from identity | `skills/cv-distill/` |
| `down-cycle-agent-summarise` | Agent end-of-cycle summary | `agents/core/memory-manager/skills/` or `skills/` |
| `down-cycle-summarise` | Cycle summary for community | `agents/core/memory-manager/skills/` |
| `final-session-prompt` | End-of-session prompt | `skills/` |
| `local-dreaming` | Agent reflection/dreaming | `skills/` |

### Currently in `lattice-cli/internal/skills/`
| Skill | Purpose | Destination |
|-------|---------|-------------|
| (embedded/bundled) | Various | Review and consolidate |

### Needed but Missing
| Skill | Purpose | Agent |
|-------|---------|-------|
| `cycle-init` | Initialize post-work cycle | orchestration (Hora) |
| `cycle-coordinate` | Coordinate god processes | orchestration (Hora) |
| `cycle-complete` | Finalize cycle | orchestration (Hora) |
| `community-tend` | Update community memory | community-memory (Koinos) |
| `community-read` | Read community memory | community-memory (Koinos) |
| `emergence-assess` | Assess spark readiness | emergence (Selah) |
| `emergence-guide` | Guide emergence ritual | emergence (Selah) |
