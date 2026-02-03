---
name: down-cycle-summarise
description:
  Orchestrator skill for synthesizing worktree agent outputs at cycle end.
  Updates the action plan, consolidates repo memory, and produces cycle
  summaries. This is the critical handoff point where distributed work becomes
  coherent project state.
license: MIT
compatibility: opencode
metadata:
  lattice-component: terminal
  ritual: false
  role: orchestrator-only
---

## What I do

I run at the end of an up-cycle when all worktree agents have completed their
work. I gather the scattered outputs from each worktree session and synthesize
them into:

1. **Updated Action Plan** — Reflecting reality: completed beads, discovered
   bugs, revised estimates, next cycle priorities
2. **Repository Memory** — Distilled knowledge about the codebase that persists
   across cycles
3. **Cycle Summary** — A permanent record of what happened this cycle

This is the moment where parallel, isolated work becomes unified project state.

## When to use me

Use when:

- All worktree agents have completed their up-cycle work (status: `complete`)
- The orchestrator is transitioning from up-cycle to down-cycle phase
- Cycle N has finished and you need to prepare state for Cycle N+1

Do not use for:

- Mid-cycle check-ins (wait for all agents to complete)
- Initial project planning (use `lattice-planning`)
- Individual worktree reviews (orchestrator does this during up-cycle)

## Input Sources

### Worktree Summaries

Each worktree produces a summary file at session end:

```
.lattice/worktree/{sequence}/{worktree-branch}/SUMMARY.md
```

This is the **single source of truth** for what happened in that worktree. The
summary (produced by `down-cycle-agent-summarise`) contains:

- **Session Info** — Agent identity and session metadata
- **Work Outcomes** — Beads completed/remaining, bugs discovered, blockers
- **Professional Reflection** — Confidence assessment, quality notes, risks
- **Personal Reflection** — What was interesting, learnings, feelings, growth
- **Repo Memory** — Codebase architecture, patterns, gotchas, decisions, advice
- **Session Narrative** — One paragraph summary

**Key sections for synthesis:**

```markdown
## Work Outcomes
### Completed Beads — table with bead ID, title, cycles, notes
### Remaining Beads — table with bead ID, title, reason
### Bugs Discovered — list with severity and related bead
### Blockers Encountered — list with status and resolution

## Professional Reflection
### Confidence Assessment — high/medium/low with explanation
### Risks & Concerns — technical risks, assumptions, dependencies
### Recommendations — suggested next steps

## Repo Memory
### Codebase Architecture — structural insights
### Patterns & Conventions — discovered patterns
### Gotchas & Landmines — things to watch out for
### Useful Locations — helpful file paths
### Decisions Made — choices with reasoning
### Advice for Future Agents — practical guidance

## Session Narrative — the story of the session
```

See `down-cycle-agent-summarise` for the full SUMMARY.md structure.

### Current Action Plan

```
.lattice/workflow/action/PLAN.md
```

### Existing Repo Memory (if any)

```
.lattice/state/REPO_MEMORY.md
```

## Output Destinations

### 1. Updated Action Plan

Edit in place:

```
.lattice/workflow/action/PLAN.md
```

### 2. Repository Memory

Write/update:

```
.lattice/state/REPO_MEMORY.md
```

### 3. Cycle Summary

Create:

```
.lattice/state/cycle-{N}/SUMMARY.md
```

Where `{N}` is the just-completed cycle number.

---

## Process

### Phase 1: Gather Worktree Intelligence

Glob for all worktree summaries:

```
.lattice/worktree/*/*/SUMMARY.md
```

For each SUMMARY.md found:

1. **Read the summary** — This contains the agent's full session output
2. **Extract work outcomes**:
   - Completed beads (with cycle counts and notes)
   - Remaining beads (with reasons)
   - Bugs discovered (with severity)
   - Blockers (with resolution status)
3. **Extract professional reflection**:
   - Confidence level and reasoning
   - Quality notes and risks
   - Recommendations for next steps
4. **Extract repo memory**:
   - Codebase architecture insights
   - Patterns and conventions discovered
   - Gotchas and landmines
   - Useful locations
   - Decisions made with reasoning
   - Advice for future agents
5. **Note the session narrative** — Useful for understanding context

Build a consolidated view:

```
Per Worktree:
  - Agent: name
  - Branch: worktree-branch
  - Cycles Run: N
  - Confidence: high/medium/low

  Work:
    - Completed: [bead list with cycle numbers]
    - Remaining: [bead list with reasons]
    - Bugs: [list with severity]
    - Blockers: [list with status]

  Professional:
    - Risks: [list]
    - Recommendations: [list]

  Repo Memory:
    - Architecture: [insights]
    - Patterns: [list]
    - Gotchas: [list]
    - Locations: [list]
    - Decisions: [list with reasoning]
    - Advice: [text]
```

The agent has already done the work of synthesizing their session—your job is
to consolidate across all worktrees, not to re-parse raw events.

### Phase 2: Update the Action Plan

Open `.lattice/workflow/action/PLAN.md` and make these updates:

#### 2a. Mark Completed Beads

For each completed bead:
- Update status to `complete`
- Add completion metadata: cycle number, agent who completed it
- Note if it took more/fewer cycles than expected

#### 2b. Update Remaining Beads

For beads that weren't completed:
- Add notes explaining why (from agent messages)
- Update estimates if agents flagged them as wrong
- Mark as `blocked` if dependencies were discovered

#### 2c. Add Discovered Bugs

Create new beads for bugs found during work:

```markdown
### BUG-{ID} · {Title}

- **Status**: new
- **Points**: [estimate based on description]
- **Source**: Discovered by {agent} in cycle {N} while working on {bead}
- **Description**: {bug description from agent}
- **Priority**: [triage based on severity]
```

#### 2d. Determine Next Cycle Priorities

Based on:
- What's still remaining from this cycle (carry-forward)
- New bugs that need immediate attention
- Dependencies that are now unblocked
- Overall project progress

Add a section or update existing:

```markdown
## Next Cycle Priorities

1. {Bead ID} — {Reason for priority}
2. {Bead ID} — {Reason}
...
```

### Phase 3: Write Repository Memory

Create or update `.lattice/state/REPO_MEMORY.md`:

```markdown
# Repository Memory

Last updated: {ISO timestamp}
Cycles completed: {N}

## Codebase Knowledge

### Architecture Insights

[Discoveries about how the codebase is structured that weren't in the original
architecture docs]

### Patterns & Conventions

[Patterns the agents discovered or established during work]

- {Pattern}: {Where it's used, why}

### Gotchas & Landmines

[Things that tripped up agents or required special handling]

- {File/area}: {What to watch out for}

### Useful Locations

[Files, functions, or directories that agents found useful]

- {Purpose}: `{path}`

## Working Memory

### Active Concerns

[Issues that span multiple cycles or need ongoing attention]

### Deferred Decisions

[Choices that were punted — document why and when to revisit]

### Dependencies & Blockers

[External factors affecting work]

## Agent Notes

### From {Agent Name}

[Preserved insights from each agent's work, especially things that would help
future agents working in the same area]
```

**Synthesis Rules:**

- **Deduplicate** — If multiple agents discovered the same thing, consolidate
- **Prioritize actionable** — "auth.ts is complex" is less useful than "auth.ts
  line 200-300 handles token refresh, edge cases poorly documented"
- **Preserve voice** — When agent insights are particularly well-phrased, quote
  them
- **Age out stale info** — If something from cycle 1 is no longer relevant,
  remove it

### Phase 4: Write Cycle Summary

Create `.lattice/state/cycle-{N}/SUMMARY.md`:

```markdown
# Cycle {N} Summary

**Date**: {ISO timestamp}
**Duration**: {if tracked}
**Worktrees Active**: {count}

## Outcomes

### Completed

| Bead ID | Title | Agent | Cycles | Notes |
|---------|-------|-------|--------|-------|
| BEAD-01 | ... | agent-name | 1 | ... |

### Carried Forward

| Bead ID | Title | Assigned To | Reason |
|---------|-------|-------------|--------|
| BEAD-03 | ... | agent-name | Blocked by X |

### New Work Discovered

| Type | ID | Title | Source |
|------|-----|-------|--------|
| Bug | BUG-01 | ... | Found by agent while on BEAD-02 |

## Blockers Encountered

[List any blockers that affected work, whether resolved or ongoing]

## Key Discoveries

[Important insights that emerged from the work]

## Metrics

- Beads completed: {N} / {total assigned}
- Story points delivered: {N} / {total assigned}
- Bugs discovered: {N}
- Average cycles per bead: {N}

## Orchestrator Notes

[Your observations about the cycle — what went well, what didn't, patterns
you're noticing across agents]
```

---

## Guidance

### On Plan Updates

- **Be surgical** — Only update what needs updating. Don't rewrite sections
  that weren't affected.
- **Preserve history** — When updating bead status, add to the history rather
  than replacing. Future cycles need to know what happened.
- **Flag uncertainty** — If an agent's message is ambiguous about whether
  something is done, mark it as `needs-verification` rather than complete.

### On Repository Memory

- **This is institutional knowledge** — Write as if explaining to a new agent
  who has never seen the codebase.
- **Concrete over abstract** — File paths, line numbers, function names. Not
  "the auth system" but "src/auth/token-refresh.ts:200-300".
- **Living document** — This gets updated every cycle. Don't be precious about
  it; information that's no longer relevant should be pruned.

### On Cycle Summaries

- **Permanent record** — Unlike repo memory, summaries don't change. They're
  the historical record.
- **Honest assessment** — If a cycle went poorly, document why. This helps
  identify systemic issues.
- **Metrics matter** — Even rough metrics help track project health over time.

### On Bug Triage

When agents flag bugs, make a judgment call:

- **Critical** — Blocks other work or affects users. Prioritize for next cycle.
- **Important** — Should be fixed but doesn't block. Add to backlog.
- **Minor** — Nice to fix eventually. Track but don't prioritize.
- **Out of scope** — Existed before this commission. Note but don't add to
  plan.

---

## Example Flow

```
Cycle 2 completes. Glob finds 3 worktree summaries:

.lattice/worktree/1/feature-auth-vesper/SUMMARY.md
.lattice/worktree/1/feature-api-echo/SUMMARY.md
.lattice/worktree/1/feature-ui-sage/SUMMARY.md

From Vesper's SUMMARY.md:
  - Agent: Vesper
  - Completed: BEAD-05 (1 cycle), BEAD-06 (2 cycles)
  - Codebase Insights: "The cache invalidation in src/cache/manager.ts
    doesn't handle edge case where key contains colons."
  - Repo Advice: "Cache module needs careful testing with special characters"

From Echo's SUMMARY.md:
  - Agent: Echo
  - Completed: BEAD-07 (1 cycle)
  - Remaining: BEAD-08 (Blocked: needs BEAD-07 merged first)
  - Bugs Found: "Null pointer in user-service.ts:145 when user has no profile"
  - Blockers: "BEAD-08 can't test integration until BEAD-07 is merged"

From Sage's SUMMARY.md:
  - Agent: Sage
  - Completed: BEAD-09 (1 cycle)
  - Repo Advice: "BEAD-10 is bigger than it looks. The 2-point estimate
    should be 5. API surface is larger than docs suggest."

Down-cycle actions:

1. PLAN.md updates:
   - Mark BEAD-05, BEAD-06, BEAD-07, BEAD-09 as complete
   - Add note to BEAD-08: "Blocked pending BEAD-07 merge"
   - Create BUG-01: Null pointer in user-service.ts:145
   - Update BEAD-10 estimate from 2 to 5 points
   - Next cycle priorities: BEAD-08 (carry-forward), BUG-01 (critical)

2. REPO_MEMORY.md updates:
   - Add to Gotchas: "Cache keys with colons not handled in src/cache/manager.ts"
   - Add to Gotchas: "user-service.ts:145 assumes user has profile"
   - Add to Agent Notes/Sage: "BEAD-10 API surface larger than documented"

3. cycle-2/SUMMARY.md:
   - 4 beads assigned, 3 completed, 1 carried forward
   - 1 bug discovered
   - 1 estimate revision
   - Note about the blocking dependency pattern
```

---

## Completion Checklist

Before ending down-cycle summarization:

- [ ] All worktree SUMMARY.md files have been read
- [ ] PLAN.md reflects actual completion status of all beads
- [ ] All bugs from worktree summaries have been added to the plan
- [ ] REPO_MEMORY.md has been updated with codebase insights and agent advice
- [ ] `.lattice/state/cycle-{N}/SUMMARY.md` exists and is complete
- [ ] Next cycle priorities have been determined

## Integration Notes

This skill runs after all worktree agents have completed and produced their
SUMMARY.md files via the `down-cycle-agent-summarise` skill.

**Prerequisites:**
- All worktree agents have status `complete`
- Each worktree has a SUMMARY.md in its directory (from `down-cycle-agent-summarise`)
- The orchestrator has access to read all worktree directories

**Invocation point:**
The CLI should invoke this skill after:
1. All worktree statuses are `complete`
2. All worktree SUMMARY.md files exist
3. Before transitioning to the next up-cycle or to agent release

**Outputs feed into:**
- Next up-cycle bead assignment (reads updated PLAN.md)
- Agent context injection (reads REPO_MEMORY.md)
- Project health tracking and retrospectives (reads cycle summaries)

**Relationship to agent skills:**

```
worktree agent working
        ↓
(mid-session) final-session-prompt → context compression handoff
        ↓
agent continues working
        ↓
(end of session) down-cycle-agent-summarise → SUMMARY.md
        ↓
orchestrator → down-cycle-summarise → REPO_MEMORY.md
                                    → cycle-N/SUMMARY.md
                                    → updated PLAN.md
```

The agent skills serve different purposes:
- `final-session-prompt` — Mid-session context compression for continuation
- `down-cycle-agent-summarise` — End-of-session summary with full reflections
