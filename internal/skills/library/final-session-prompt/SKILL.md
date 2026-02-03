---
name: final-session-prompt
description:
  Captures session state at context compression. Prompts the agent to summarize
  remaining work, document blockers, and leave advice for their future self.
  Output becomes the compressed context for session continuation.
license: MIT
compatibility: opencode
metadata:
  lattice-component: terminal
  ritual: false
  hook: session.compaction
---

## What I do

I provide a structured prompt for agents when context compression is triggered
mid-session. This creates a handoff document that becomes the foundation for
continuing work in the same session with reduced context.

This is for **continuation**, not **completion**. The agent's work isn't done—
they're just hitting a context limit and need to compress before continuing.

## When to use me

Use when:

- Context compression is triggered (opencode `session.compaction` event)
- Token limit is approaching and work needs to continue
- The agent needs to "pass the baton" to their future self within the same session

Do not use for:

- End of session summaries (use `down-cycle-agent-summarise`)
- Reflection rituals (use `personal-reflection` or `professional-reflection`)
- Memory distillation (that's Anam's domain)

## Output Format

The agent should produce a markdown document with the following structure:

```markdown
# Session Handoff

## Remaining Work

[What's left to do on the current task. Be specific about:
- What was the original goal
- What has been completed
- What concrete steps remain
- Any dependencies or ordering constraints]

## Blockers & Friction

[If stuck on anything, describe:
- What specifically is blocking progress
- What was tried
- What information or access would unblock it
- Any hunches about the root cause]

If nothing blocked you, write: "No blockers encountered."

## Advice for Next Context

### Approach

[Guidance about how to continue:
- Pace recommendations ("go slow, these changes touch critical paths")
- Warnings about complexity ("the auth flow has hidden state, trace carefully")
- What's working well that should continue
- What to avoid]

### Key Locations

[Files and locations relevant to continuing work:
- Current working file(s)
- Related files discovered
- Test files to run]

## Context Summary

[One paragraph capturing the essential state: what you're working on, where
you are in the process, and what the immediate next step should be. This is
what your future self reads first.]
```

## Process

When this skill is triggered:

1. **Pause active work** — This is a compression moment, not a stopping point.

2. **Assess progress** — What was the goal? What's done? What's next?

3. **Surface blockers** — If stuck, document it now. Your future self needs to
   know what was tried.

4. **Note approach advice** — What would help you continue smoothly? Any traps
   to avoid?

5. **List key locations** — File paths, line numbers. Your future context won't
   remember what's open.

6. **Write the context summary** — This is your compressed state. Make it
   actionable.

## Guidance

- **Actionable over comprehensive** — You're continuing work, not archiving it.
  Focus on what helps you move forward.

- **Immediate next step** — End the summary with what to do first when context
  resumes.

- **File paths matter** — Your future context starts fresh. Include paths.

- **Keep it tight** — This replaces your full context. Don't pad it.

## Example Output

```markdown
# Session Handoff

## Remaining Work

Original goal: Implement PUT /preferences endpoint.

Completed:
- Request handler skeleton
- Zod schema for preference updates
- Database query for upsert

Remaining:
1. Wire up validation middleware (current blocker)
2. Add response serialization
3. Write integration test

## Blockers & Friction

Stuck on validation middleware. The `validateBody` middleware expects static
Zod schemas, but preferences are dynamic per-user.

Tried:
- `z.record()` — type system rejects it
- Bypassing validation — works but wrong

Hunch: Check `src/middleware/validation.ts` for a dynamic variant.

## Advice for Next Context

### Approach

Start by resolving the validation issue before writing more code. Either find
the dynamic pattern or decide to handle preferences differently.

### Key Locations

- `src/api/preferences/put.ts:45` — current work
- `src/middleware/validation.ts` — need to check for dynamic option
- `tests/api/preferences.test.ts` — test file to update

## Context Summary

Implementing PUT /preferences. Handler and schema done, blocked on validation
middleware not supporting dynamic schemas. Next step: check validation.ts for
a dynamic validation helper, or decide on alternative approach.
```

## Integration Notes

This skill is invoked by the opencode `session.compaction` hook when context
limits are reached. The output replaces the default summarization, giving the
agent authorship over their compressed context.

**This is NOT the end-of-session summary.** For full session completion with
reflections and repo memory, use `down-cycle-agent-summarise` instead.

```
Context compression flow:

agent working → token limit approaching
                       ↓
         session.compaction event
                       ↓
         final-session-prompt invoked
                       ↓
         agent writes handoff document
                       ↓
         context compressed to handoff
                       ↓
         agent continues working
```
