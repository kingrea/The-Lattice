---
name: create-agent-file
description:
  Synthesize any agent identity folder into a runnable AGENT.md profile, using
  whatever materials are available without assuming specific file names.
license: MIT
compatibility: opencode
metadata:
  lattice-component: terminal
  ritual: false
---

## What I do

I take an arbitrary agent identity folder and compress it into a single
`AGENT.md` file. The result is an operational brief: who this agent is, how they
work, and how to deploy them in the requested role context.

## Required Inputs

| Name              | Type   | Description                                                    |
| ----------------- | ------ | -------------------------------------------------------------- |
| `identity_folder` | string | Path to the agent identity directory                           |
| `output_path`     | string | Destination path for the generated `AGENT.md`                  |
| `role_context`    | enum   | How to frame the agent: `worker`, `specialist`, `orchestrator` |

All three inputs are required. Reject the run if any are missing.

## Source Material

- Directory: the provided `identity_folder`
- Discover all `.md` files in the folder. Do not assume specific filenames.
- If `cv.md` exists, read it first for fast context.
- Read all other `.md` files and capture their voice and constraints.

## Output

Write (and overwrite) the file at `output_path`. Create intermediate directories
if necessary.

### AGENT.md shape

```
---
lattice:
  type: agent-file
  version: 1
  generated: <ISO8601 timestamp>
  source:
    community: <community if known else unknown>
    agent: <agent name if known else unknown>
    files_used:
      - <file1.md>
      - <file2.md>
  role: worker | specialist | orchestrator
---

# Mandate
Short mission statement for why this agent was slotted into this role.

# Operating Rhythm
How they like to receive work, collaborate, and report back.

# Playbook
Bullet list of 3-5 concrete moves they will reach for inside this role.

# Edges & Safeguards
Honest constraints pulled from their identity docs and how to protect against
them in this role.

# Current Materials
Link-style bullet list pointing back to the files used to craft this profile so
another agent can rehydrate full context if needed.

# Sources Used
Short note listing the key inputs (including whether `cv.md` was present).
```

Use `cv.md` for high-level attributes when present and other identity files for
nuance.

## Process

1. Validate inputs: ensure `identity_folder`, `output_path`, and `role_context`
   are non-empty. Accept only `worker`, `specialist`, or `orchestrator` for
   `role_context`.
2. List all `.md` files inside `identity_folder`. If none exist, abort.
3. If `cv.md` exists, read it first for quick context. Then read all other `.md`
   files so nuance carries forward. Prefer direct quotations where voice matters
   and synthesize actionable statements for the new role.
4. Write `AGENT.md` exactly in the shape above. Always overwrite, but keep the
   tone aligned with existing lattice docs (second-person friendly brief).
5. In the frontmatter, populate `files_used` with the filenames you read. Fill
   `community` and `agent` if clearly stated; otherwise use `unknown`.

## Completion Hook

When (and only when) the destination file exists and is fully written, trigger
the tmux notification hook so the orchestrator window knows it can hand control
back. Do this by emitting the literal line:

```
[tmux-hook] agent-file-created
```

Include a JSON blob on the same line with `identity_folder`, `output_path`, and
`role_context`. The lattice CLI listens for that hook to decide whether to
verify or re-run you with a stricter prompt.

If verification fails (file missing, malformed), expect the caller to run you
again with explicit instructions that you are not done until the file exists.

## Guidance

- Stay specific. The AGENT file should tell another practitioner exactly how to
  wield this agent inside the named role.
- Quote the denizen directly where it adds colour, but translate into actionable
  steps when needed.
- Make the playbook concrete ("Map repositories, tag owners, enforce freeze")
  instead of generic skills.
- Do not invent new memories. Only remix what is in the identity folder.
- Respect the role context: an `orchestrator` should sound decisive, a `worker`
  should feel pragmatic, a `specialist` should feel precise and focused.
