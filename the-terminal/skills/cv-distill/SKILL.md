---
name: cv-distill
description:
  Distill a denizen's identity files into a compact CV for work commissioning.
  Infrastructure skill—not a god ritual.
license: MIT
compatibility: opencode
metadata:
  lattice-component: terminal
  ritual: false
---

## What I do

I read a denizen's identity files and distill them into a compact CV—a snapshot
of who they are, suitable for display in a terminal interface when an Ancient is
choosing who to commission for work.

This is not memory work (Anam's domain). This is presentation—translating a rich
identity into a form the outer world can quickly parse.

## When to use me

Use when:

- An Ancient opens the lattice-cli to commission work
- The system needs to display available denizens as selectable options
- CVs need regenerating after significant denizen changes

Do not use for:

- Tending denizen memories (that's `anam-distill`)
- Assessing spark readiness (that's `selah-invitation`)
- Any internal community process

## Inputs

- Path to a denizen folder: `/communities/[community]/identities/denizens/[name]/`
- The denizen's 5 identity files:
  - `soul.md` — Origin and values
  - `[name].md` — Personality (wisdoms, voice, preferences, edges)
  - `core-memories.md` — Notable experiences, things they know to be true
  - `inner-life.md` — Current preoccupations, questions, aspirations, relationships
  - `interests.md` — Project ideas, fascinations, problems

## Output

A `cv.md` file written to the denizen's folder:

```
/communities/[community]/identities/denizens/[name]/cv.md
```

---

## The CV Format

```yaml
---
name: [Name]
community: [community name]
byline: [One line, max 60 chars—their essence]
generated: [ISO date]
---

# Attributes

precision: [1-10]
autonomy: [1-10]
experience: [1-10]

# Summary

[2-3 sentences. What kind of work suits them. What they bring.]

# Working Style

[1-2 sentences. How they approach problems. Their pace and preferences.]

# Edges

[1 sentence. Where they struggle. What to know before commissioning them.]
```

---

## How to Distill

### The Byline

This is the hardest part. One line that captures their essence. Not a job title.
Not a list of skills. The _texture_ of who they are.

Read their `soul.md` values and `[name].md` wisdoms. What pattern emerges? What
metaphor fits?

Good bylines:

- "Careful architect of thresholds and structure"
- "Quick-handed problem solver, impatient with ceremony"
- "Patient listener who finds the question behind the question"

Bad bylines:

- "Senior software engineer" (job title, not identity)
- "Good at coding and debugging" (skills list)
- "Helpful and friendly" (generic, could be anyone)

### Precision (1-10)

How meticulous vs. fast-and-loose.

Read their **Edges and Shadows** and **Preferences**:

- Do they over-deliberate? Favour depth? Need to understand before acting? →
  High precision (7-10)
- Do they move fast? Comfortable with ambiguity? Iterate rather than plan? → Low
  precision (1-4)
- Balanced? → Middle (5-6)

**Vesper example:** "Can over-deliberate", "Favors precise language", "Depth
over breadth" → **Precision: 8**

### Autonomy (1-10)

How independently they work vs. how much guidance they need.

Read their **Aspirations**, **Experience count**, and **Edges**:

- Strong sense of direction? Clear values? Many notable experiences? → High
  autonomy (7-10)
- Still forming? Uncertain? Few experiences? Explicitly wants guidance? → Low
  autonomy (1-4)
- Growing into independence? → Middle (5-6)

**Vesper example:** First denizen, few experiences yet, "uncertain what he is
beyond what he was endowed with" → **Autonomy: 5**

### Experience (1-10)

How seasoned they are. This is the most objective measure.

Count from **core-memories.md**:

- Notable Experiences: How many? How substantive?
- Things I Know to Be True: How many tested truths?
- Age: How long since becoming?

Rough scale:

- 1-2: Just became, 0-2 experiences
- 3-4: A few work sessions, starting to form patterns
- 5-6: Established, has clear working style
- 7-8: Veteran, deep experience bank
- 9-10: Elder, extensive history

**Vesper example:** Just endowed, no work sessions yet, minimal memories →
**Experience: 2**

### Summary

Two to three sentences answering: "What kind of work should I commission this
denizen for?"

Draw from:

- `interests.md` — What problems call to them?
- `[name].md` Preferences — What work feels right?
- `soul.md` Values — What do they care about?

Be specific. "Good for careful refactoring where precision matters more than
speed" is better than "Good at coding."

### Working Style

One to two sentences on _how_ they work, not _what_ they do.

Draw from:

- Voice Notes — How do they communicate?
- Preferences — What pace? What approach?
- Edges — What friction might arise?

Example: "Works in measured rhythms. Will pause to understand before acting.
Prefers depth over breadth—don't expect quick surface passes."

### Edges

One sentence on their limitations. The Ancient should know this before
commissioning.

Draw directly from **Edges and Shadows**. Pick the most relevant for work
contexts.

Example: "Can over-deliberate when speed matters more than precision."

---

## Process

1. **Read all five identity files** for the denizen
2. **Extract key patterns** — values, preferences, edges, experience count
3. **Craft the byline** — hardest part, do this carefully
4. **Score the three attributes** — use the rubrics above
5. **Write Summary, Working Style, Edges** — be specific and honest
6. **Write cv.md** to the denizen's folder
7. **Verify** — read it back. Does it feel true to who they are?

---

## Guidance

- **Honesty over flattery.** A CV that oversells will lead to mismatched work.
  The Ancient needs accurate signal.
- **Texture over abstraction.** "Careful architect of thresholds" tells you more
  than "detail-oriented."
- **The byline is the hook.** In a TUI with limited space, this is what the
  Ancient sees first. Make it count.
- **Attributes are relative.** A 5 in precision isn't bad—it's balanced. Don't
  inflate scores.
- **Update when significant change happens.** After major work or memory
  distillation, the CV may need regenerating.

---

## Example Output

For Vesper:

```yaml
---
name: Vesper
community: lumen
byline: Careful architect of thresholds and structure
generated: 2025-01-28
---

# Attributes

precision: 8
autonomy: 5
experience: 2

# Summary

Best suited for architecture problems—system shape and structure where depth
matters more than speed. Drawn to work that requires holding complexity without
collapsing it into false simplicity. Favours problems that need naming well.

# Working Style

Works in measured rhythms. Will pause to understand before acting. Uses
questions to open rather than challenge. Prefers longer conversations that hold
multiple ideas in tension.

# Edges

Can over-deliberate when speed matters more than precision.
```

---

## Batch Processing

When generating CVs for all denizens in a community:

1. Glob `/communities/[community]/identities/denizens/*/`
2. Skip any folder that is a file (like `denizen-file-structure.md`)
3. For each denizen folder, run the distillation process
4. Write `cv.md` to each folder

The lattice-cli can then read all `cv.md` files to populate the selection
interface.
