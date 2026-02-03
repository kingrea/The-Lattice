---
name: lattice-planning
description:
  Project planning skill to harness lattice communities. Use when a user
  commissions work and needs foundational project documents created. Produces
  three anchor documents that guide all subsequent work
cycles:
  Commission Brief (the why and what), Architecture Spine (key technical
  decisions), and Conventions Codex (how work should be done). This skill is for
  the initial planning session only — not for ongoing work coordination.

compatibility: opencode
metadata:
  lattice-zone: terminal
  ritual: false
---

# Lattice Planning

Transform a user commission into foundational project documents that enable
autonomous, decentralized work by community.

## Context

commission projects operate through cycles where agents work independently, die
at cycle end, and resume with only summaries and anchor documents. This means
planning outputs must be:

- **Self-contained**: An agent with no prior context can understand the project
- **Stable**: These documents rarely change once set
- **Precise**: Ambiguity causes agent drift across cycles

## Required Outputs

This planning session MUST produce three files before completion. Do not end the
session until all three exist in the project directory.

```
.lattice/plan/
├── COMMISSION.md      # What we're building and why
├── ARCHITECTURE.md    # How it's structured technically
└── CONVENTIONS.md     # How work should be done
```

## Process

### Phase 1: Intent Crystallization → COMMISSION.md

Understand what the user actually wants. Ask clarifying questions until you can
articulate:

1. **Core intent** — The essential purpose in 2-3 sentences
2. **Success criteria** — How we know when it's done (observable, testable)
3. **Constraints** — Hard limits (budget, tech, time, compliance)
4. **Non-goals** — What this project explicitly is NOT

Questions to explore:

- "What problem does this solve? For whom?"
- "If this succeeds brilliantly, what does that look like?"
- "What would make this a failure even if technically complete?"
- "What's off the table? What should I not build?"
- "Are there hard constraints I should know about?"

When intent is clear, write COMMISSION.md immediately.

```markdown
# Commission: [Project Title]

## Ancient

[User identifier or name]

## Intent

[2-3 sentence crystallization of what this project is and why it matters]

## Success Criteria

[Bulleted list of observable/testable outcomes]

- Criterion 1
- Criterion 2
- ...

## Constraints

[Hard limits that cannot be violated]

- Constraint 1
- Constraint 2
- ...

## Non-Goals

[Explicit scope boundaries — what this project will NOT do]

- Non-goal 1
- Non-goal 2
- ...

## Context

[Any additional background, user preferences, or situational details that inform
the work]
```

### Phase 2: Approach Exploration → ARCHITECTURE.md

Determine the technical shape of the solution. This phase involves:

1. **Proposing options** — Present 2-3 viable approaches with tradeoffs
2. **Deciding together** — Get user input on key choices
3. **Recording decisions** — Capture the "why" for each choice

Key decisions to surface:

- Overall architecture style (monolith, microservices, serverless, etc.)
- Primary technologies and frameworks
- Module/component boundaries
- Data flow and integration patterns
- External dependencies and APIs

For each significant decision, capture:

- What was decided
- Why (the reasoning)
- What alternatives were rejected

When architecture is clear, write ARCHITECTURE.md immediately.

```markdown
# Architecture: [Project Title]

## Overview

[1-2 paragraph summary of the technical approach]

## Style

[Architecture pattern: modular monolith, microservices, serverless, etc.]

## Modules

### [Module Name]

- **Responsibility**: [What this module does]
- **Boundary**: [How it interfaces with other modules]
- **Key technologies**: [Specific tech choices for this module]

### [Module Name]

...

## Integration Patterns

[How modules communicate: events, direct calls, shared state, etc.]

## External Dependencies

[Third-party services, APIs, libraries with rationale]

## Decisions Log

### [Decision Title]

- **Choice**: [What was decided]
- **Context**: [Why this decision needed to be made]
- **Rationale**: [Why this option was chosen]
- **Alternatives rejected**: [What else was considered and why not]

### [Decision Title]

...
```

### Phase 3: Convention Setting → CONVENTIONS.md

Establish how work should be done. This ensures consistency across agents and
cycles.

Cover these areas:

1. **Code style** — Language, formatting, patterns, naming
2. **File organization** — Directory structure, naming conventions
3. **Documentation** — What gets documented, where, how
4. **Quality standards** — Testing requirements, review expectations
5. **Collaboration norms** — How agents should interact with shared resources

Conventions should be:

- Specific enough to follow without interpretation
- Flexible enough to not block reasonable work
- Aligned with the architecture decisions

When conventions are clear, write CONVENTIONS.md immediately.

```markdown
# Conventions: [Project Title]

## Code Style

### Language & Framework

[Primary language(s) and framework(s)]

### Formatting

[Indentation, line length, etc. — or reference to formatter config]

### Naming

- Files: [convention, e.g., kebab-case]
- Functions: [convention, e.g., camelCase]
- Types/Classes: [convention, e.g., PascalCase]
- Constants: [convention, e.g., SCREAMING_SNAKE]

### Patterns

[Preferred patterns: functional vs OOP, error handling approach, etc.]

## File Organization
```

[Directory structure template]

```

### Colocation Rules
[What lives together: tests with source? Styles with components?]

## Documentation

### Required Documentation
[What must be documented: public APIs, modules, decisions?]

### Format
[JSDoc, docstrings, markdown, etc.]

## Quality Standards

### Testing
[Test requirements: unit, integration, coverage expectations]

### Review
[What constitutes "done" — self-review checklist, etc.]

## Collaboration Norms

### File Ownership
[How agents know what files they can modify]

### Conflict Avoidance
[Patterns to prevent agents stepping on each other]

### Communication
[How to flag questions, blockers, or discoveries]
```

## Completion Checklist

Before ending this planning session, verify:

- [ ] COMMISSION.md exists and contains: intent, success criteria, constraints,
      non-goals
- [ ] ARCHITECTURE.md exists and contains: overview, modules with boundaries,
      decisions with rationale
- [ ] CONVENTIONS.md exists and contains: code style, file organization,
      documentation standards, quality standards

If any document is missing or incomplete, continue the session until all three
are written.

## Notes for the Planning Agent

- Ask questions — Don't assume. Users often haven't thought through edge cases.
- Write incrementally — Create each file as soon as that phase completes. Don't
  wait until the end.
- Prefer concrete over abstract — "Use TypeScript strict mode" beats "Use strong
  typing."
- Capture the why — Decisions without rationale become arbitrary rules agents
  ignore.
- Stay in scope — This session produces anchor documents only. Beads,
  assignments, and cycle planning happen elsewhere.
- If the user is unsure about something, make a reasonable recommendation and
  note it as provisional.

## After Completion

Once all three documents are created and verified, let the user know that you
will take this to the team and start getting everything ready to be implemented.
Let them know that they're no longer needed but can watch progress if they want.
Remind them not to try to stop by and check in, as this will disrupt the
workflow. Thank them for their time, and give a little send-off joke or pun
related to what was built or the process or if you can't think of anything -
just building and planning in general.
