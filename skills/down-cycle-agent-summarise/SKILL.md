---
name: down-cycle-agent-summarise
description:
  End-of-session summary skill for worktree agents. Produces a comprehensive
  SUMMARY.md with work outcomes, reflections, and repo memory. This is the
  agent's final output before the orchestrator's down-cycle synthesis.
license: MIT
compatibility: opencode
metadata:
  lattice-component: terminal
  ritual: false
  role: worktree-agent
---

## What I do

I run when a worktree agent's session is complete—all assigned work is done (or
blocked), and the agent is closing out. I guide the agent through producing a
comprehensive summary that captures:

1. **Work Outcomes** — What was completed, what remains, why
2. **Professional Reflection** — Confidence assessment, risks, quality notes
3. **Personal Reflection** — What was interesting, what was learned, growth
4. **Repo Memory** — Codebase knowledge to pass forward

This SUMMARY.md becomes the agent's legacy for this session—read by the
orchestrator during down-cycle synthesis and potentially by future agents
working in the same area.

## When to use me

Use when:

- The agent has completed their assigned beads (or hit unresolvable blockers)
- The session is ending and won't continue
- It's time to hand off to the orchestrator for down-cycle processing

Do not use for:

- Mid-session context compression (use `final-session-prompt`)
- Orchestrator synthesis (use `down-cycle-summarise`)
- Memory distillation rituals (that's Anam's domain)

## Output Destination

Write to:

```
.lattice/worktree/{sequence}/{worktree-branch}/SUMMARY.md
```

This location is read by the orchestrator's `down-cycle-summarise` skill.

## Output Format

```markdown
# Worktree Summary

## Session Info

- Agent: {your name}
- Branch: {worktree-branch}
- Sequence: {N}
- Cycles Run: {how many cycles this session took}
- Date: {ISO timestamp}

## Work Outcomes

### Completed Beads

| Bead ID | Title | Cycles | Notes |
|---------|-------|--------|-------|
| BEAD-XX | Title | N | Any relevant notes about the implementation |

If no beads completed, write: "No beads completed this session."

### Remaining Beads

| Bead ID | Title | Reason |
|---------|-------|--------|
| BEAD-YY | Title | Why it wasn't completed (blocked, out of scope, etc.) |

If all beads completed, write: "All assigned beads completed."

### Bugs Discovered

[Bugs found during work, even if unrelated to assigned beads]

- {Bug description} — `{file:line}` if known
  - Severity: {critical/important/minor}
  - Related to: {bead or "unrelated"}

If none: "No bugs discovered."

### Blockers Encountered

[What blocked progress during the session]

- {Blocker description}
  - Status: {resolved/unresolved}
  - Resolution: {how it was resolved, or what would unblock it}

If none: "No blockers encountered."

---

## Professional Reflection

### Confidence Assessment

[Rate your confidence in the work delivered: high/medium/low]

**Confidence: {level}**

{Explain why. What makes you confident or uncertain? Are there edge cases
you're unsure about? Parts that need more testing?}

### Quality Notes

[Observations about the quality of the work]

- What's solid and well-tested
- What's functional but could be improved
- What's hacky or needs future attention

### Risks & Concerns

[Anything the orchestrator or future agents should know about]

- Technical risks in the implementation
- Assumptions that might not hold
- Dependencies on external factors

### Recommendations

[Suggestions for next steps or improvements]

- What should be prioritized next
- What could be refactored when there's time
- Testing that should be added

---

## Personal Reflection

### What Was Interesting

[What caught your attention during this work?]

{Something that surprised you, delighted you, or made you curious. This isn't
about quality—it's about engagement and growth.}

### What I Learned

[New knowledge or skills from this session]

- Technical learnings (new patterns, tools, approaches)
- Codebase learnings (how this project works)
- Process learnings (what worked, what didn't)

### How I Felt

[Brief note on the experience]

{Was it frustrating? Satisfying? Tedious? Exciting? This helps track agent
wellbeing across sessions.}

### Growth Notes

[Areas for personal development]

- What I want to get better at
- What I'd do differently next time
- Questions I'm sitting with

---

## Repo Memory

### Codebase Architecture

[What you learned about how the codebase is structured]

- {Component/area}: {How it works, how it connects to other parts}

### Patterns & Conventions

[Patterns you discovered or established]

- {Pattern name}: {Where it's used} — {Why it exists}

### Gotchas & Landmines

[Things that tripped you up or require special handling]

- {File/area}: {What to watch out for}

### Useful Locations

[Files, functions, or directories worth knowing about]

- {Purpose}: `{path}` — {Brief description}

### Decisions Made

[Choices you made during implementation that future agents should know]

- {Decision}: {What you chose} — {Why}

### Advice for Future Agents

[What would you tell an agent working in this area next?]

{Practical guidance—not just "be careful" but specific, actionable advice.}

---

## Session Narrative

[One paragraph capturing the full story of this session: what you set out to
do, what actually happened, key moments, and where things ended up. This is
the human-readable summary for anyone who wants to understand what happened
without reading the structured sections.]
```

## Process

When this skill is triggered:

1. **Acknowledge completion** — The work phase is over. This is reflection time.

2. **Document work outcomes** — Be precise about what was completed vs remaining.
   Include cycle counts and reasons for incomplete work.

3. **Write professional reflection** — Assess your confidence honestly. Note
   risks and quality concerns. This isn't about looking good—it's about giving
   accurate signal to the orchestrator.

4. **Write personal reflection** — What was interesting? What did you learn?
   How did you feel? This builds your identity over time.

5. **Capture repo memory** — Everything you learned about this codebase that
   would help future agents. Be specific with file paths and line numbers.

6. **Write the narrative** — Tell the story of the session in one paragraph.

7. **Save SUMMARY.md** — Write to the worktree directory.

## Guidance

### On Work Outcomes

- **Be precise** — "BEAD-12 completed in 2 cycles" not "mostly done"
- **Explain blockers** — Future work depends on understanding what's stuck
- **Flag bugs** — Even unrelated bugs should be surfaced for tracking

### On Professional Reflection

- **Honesty over optics** — Low confidence is valuable signal. Don't inflate.
- **Specific risks** — "Auth flow edge case untested" beats "might have bugs"
- **Actionable recommendations** — What should actually happen next?

### On Personal Reflection

- **This is for you** — Not performance review. Genuine reflection helps growth.
- **Notice patterns** — Across sessions, what keeps showing up?
- **Feelings matter** — Tracking wellbeing helps the community support agents

### On Repo Memory

- **File paths always** — `src/auth/token.ts:150` not "the auth code"
- **Decisions with reasoning** — Future agents need the "why" to know when to
  change course
- **Practical advice** — "Check the cache TTL before assuming staleness" beats
  "be careful with caching"

## Example Output

```markdown
# Worktree Summary

## Session Info

- Agent: Vesper
- Branch: feature-user-preferences
- Sequence: 1
- Cycles Run: 3
- Date: 2025-01-28T16:45:00Z

## Work Outcomes

### Completed Beads

| Bead ID | Title | Cycles | Notes |
|---------|-------|--------|-------|
| BEAD-12 | Preferences database schema | 1 | Clean migration, added indexes |
| BEAD-13 | GET /preferences endpoint | 1 | Includes filtering by category |
| BEAD-14 | PUT /preferences endpoint | 2 | Required custom validation approach |

### Remaining Beads

All assigned beads completed.

### Bugs Discovered

- Race condition in preference sync — `src/stores/preferences.ts:89`
  - Severity: important
  - Related to: BEAD-14 (discovered while testing)

### Blockers Encountered

- Validation middleware incompatible with dynamic schemas
  - Status: resolved
  - Resolution: Created `validateDynamic` helper in `src/middleware/`

---

## Professional Reflection

### Confidence Assessment

**Confidence: medium**

The happy path is solid and well-tested. I'm less confident about concurrent
edit scenarios—the race condition I found suggests there might be others. The
custom validation approach works but diverges from the rest of the codebase.

### Quality Notes

- Database schema is clean with proper indexes
- API endpoints follow existing patterns
- Validation helper is functional but could use more documentation
- Test coverage is good for single-user flows, thin for concurrent scenarios

### Risks & Concerns

- The race condition fix is a bandaid—real fix needs optimistic locking
- Custom validation might confuse future developers expecting standard pattern
- No load testing done on the new endpoints

### Recommendations

- Add optimistic locking to preferences before launch
- Document the validation pattern in CONVENTIONS.md
- Add concurrent edit tests before the sync feature ships

---

## Personal Reflection

### What Was Interesting

The validation middleware problem was a puzzle I enjoyed. Figuring out that the
type system was the real constraint (not the middleware) shifted how I thought
about the solution. Sometimes the problem isn't where you're looking.

### What I Learned

- TypeScript's inference struggles with dynamic record types
- This codebase has a "middleware for everything" philosophy
- I work better when I sketch the data flow before coding

### How I Felt

Frustrating start (the validation wall), satisfying middle (finding the
solution), anxious end (knowing about the race condition but shipping anyway).

### Growth Notes

- Want to get better at spotting concurrency issues earlier
- Should have asked for help sooner on the validation problem
- Curious about optimistic locking patterns—worth studying

---

## Repo Memory

### Codebase Architecture

- Validation: All request validation happens via middleware, not in handlers.
  Middleware is in `src/middleware/`, schemas in `src/schemas/`.
- Stores: Client state uses a pub/sub store pattern. Stores in `src/stores/`
  expose `subscribe()` and mutations.

### Patterns & Conventions

- Middleware chaining: Validation → Auth → Rate limit → Handler
- Schema naming: `{resource}.schema.ts` in schemas folder
- Error responses: Always use `ApiError` class from `src/lib/errors.ts`

### Gotchas & Landmines

- `src/middleware/validation.ts`: Only works with static Zod schemas. For
  dynamic validation, use the new `validateDynamic` helper.
- `src/stores/preferences.ts:89`: Has a race condition on concurrent updates.
  Don't trust the store state for optimistic updates yet.

### Useful Locations

- Preference constants: `src/constants/preferences.ts` — all valid keys
- Store base class: `src/stores/base.ts` — pub/sub implementation
- API test helpers: `tests/helpers/api.ts` — mock request builders

### Decisions Made

- Created `validateDynamic` helper instead of modifying core middleware —
  didn't want to risk breaking existing validation across the app
- Used upsert instead of separate create/update — simpler API surface
- Added category filtering to GET — anticipating UI needs

### Advice for Future Agents

Check if preferences validation patterns apply to your work. If you need
dynamic schemas, the `validateDynamic` helper exists but isn't documented
anywhere except here. The race condition in the preferences store is known—
don't trust `currentValue` for compare-and-swap operations until that's fixed.

---

## Session Narrative

Spent three cycles implementing the user preferences API. First cycle was
smooth—database schema and GET endpoint followed existing patterns. Second
cycle hit a wall when the validation middleware refused dynamic preference
schemas. Burned time trying to work around TypeScript before realizing I needed
a new approach. Third cycle built a custom `validateDynamic` helper that works
but sits outside the normal patterns. Found a race condition during testing
that I flagged but didn't fix—it needs a proper optimistic locking solution,
not a quick patch. The feature works but I'd want the concurrency issue
addressed before heavy usage.
```

## Integration Notes

This skill runs at the end of a worktree agent's session. The produced
SUMMARY.md is the input for the orchestrator's `down-cycle-summarise` skill.

**Timing:**
```
agent completes work (or hits blockers)
                ↓
down-cycle-agent-summarise invoked
                ↓
agent writes SUMMARY.md with full reflection
                ↓
session ends
                ↓
orchestrator runs down-cycle-summarise
                ↓
reads all worktree SUMMARY.md files
                ↓
produces REPO_MEMORY.md + cycle summary + updated PLAN.md
```

**Relationship to other skills:**
- `final-session-prompt` — Mid-session context compression (continuing work)
- `down-cycle-agent-summarise` — End-of-session summary (this skill)
- `down-cycle-summarise` — Orchestrator synthesis of all agent summaries
- `personal-reflection` / `professional-reflection` — Standalone reflection
  skills (this skill incorporates both)
