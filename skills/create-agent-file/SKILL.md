---
name: create-agent-file
description:
  Distill whatever identity materials live inside an agent's folder into a fresh
  AGENT.md that any lattice role can load without knowing that community's
  conventions.
license: MIT
compatibility: opencode
metadata:
  lattice-component: terminal
  ritual: false
---

## Purpose

Given nothing more than a path to an identity folder, assemble an operational
AGENT.md. Discover every relevant Markdown file (including `cv.md` if present),
absorb the voice and working notes inside, and merge them into a single brief
that records where the information came from.

## Required Inputs

| Name           | Type | Description                                                                             |
| -------------- | ---- | --------------------------------------------------------------------------------------- |
| `identity_dir` | path | Absolute path to the agent's identity folder. The folder layout is arbitrary.           |
| `output_path`  | path | Absolute path to the AGENT.md file that must be written (create directories as needed). |
| `role_context` | enum | One of `worker`, `specialist`, or `orchestrator`. Shapes tone + emphasis.               |

Reject the run if any input is empty or if `role_context` is outside the allowed
set.

## Source Discovery

1. List every `.md` file at the top level of `identity_dir`. Do not assume
   canonical names; accept irregular spellings.
2. If a `cv.md` exists, treat it as a quick orientation doc and read it first.
3. Ignore non-Markdown files entirely unless the caller explicitly adds support.
4. Track every file you actually quote or synthesize from so you can report it
   in both the frontmatter and the Sources section.
5. If the folder includes subdirectories, only descend one level when the folder
   name clearly signals writing (e.g. `memories/*.md`). Otherwise stay shallow.

## Output

Write (and overwrite) the file at `output_path`.

### Mandatory frontmatter

```
---
lattice:
  type: agent-file
  version: 1
  generated: <ISO8601 timestamp>
  source:
    community: <community name if known, else unknown>
    agent: <best available agent name>
    files_used:
      - <relative-or-base filename for each Markdown input>
  role: <role_context>
name: <agent display name>
mode: <worker/specialist/orchestrator framing note>
---
```

If the community or agent name cannot be inferred from frontmatter, fall back to
folder names and note any uncertainty inside the Sources section.

### Body shape

```
# <Agent Name>

## Mandate
Orient the reader to why this agent is valuable inside the requested
role_context. Tie back to the strongest identity statements.

## Working Rhythm
How they intake work, collaborate, and hand back outcomes.

## Capabilities & Tools
Concrete moves they repeatedly reach for (3-5 bullets anchored in the sources).

## Guardrails
Edges, failure modes, and how to keep them in peak form.

## Sources
Bullet list calling out each file used (filename + one-line why it mattered).
```

Feel free to add short inline quotes when it preserves voice, but synthesize the
actionable interpretation in your own words.

## Process

1. Validate parameters and confirm `identity_dir` exists.
2. Enumerate Markdown files per **Source Discovery** and read their contents.
3. Pull structured metadata from any YAML frontmatter you encounter (CVs, soul
   files, etc.) so the AGENT.md inherits accurate names and communities.
4. Draft each section with clear, role-aware language. The agent's voice leads,
   but the brief must remain actionable for the requestor.
5. Populate the provenance frontmatter (`source.files_used`) with the file names
   actually used, ordered roughly by importance.
6. Close with the `## Sources` section that mirrors the list and includes simple
   annotations ("`cv.md` — strengths + constraints").
7. Save to `output_path` atomically, ensuring the file exists and is non-empty
   before signalling completion.

## Completion Hook

After the AGENT.md file is written, emit exactly one line:

```
[tmux-hook] agent-file-created {"identity_dir":"...","output_path":"...","role_context":"..."}
```

Update the JSON keys to reflect the inputs you received. Do not fire the hook if
the file is missing or if validation failed.

## Guidance

- Do not assume canonical filenames; adapt to whatever Markdown files exist.
- Prefer cv frontmatter for canonical spellings, but never block if it is
  missing.
- Quote sparingly and only to anchor tone; otherwise keep guidance imperative
  and role-aware.
- If critical files are missing, explain the gap inside `## Guardrails` or the
  `## Sources` section so future maintainers know what to fix.
- Preserve each agent's flavor without sacrificing clarity for the humans who
  will run them.
