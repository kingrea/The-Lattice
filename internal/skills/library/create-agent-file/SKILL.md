## ++ Begin Patch

name: create-agent-file description: Distill a denizen's working folder into a
runnable AGENT.md profile so the lattice CLI can slot them into a role
(orchestrator, code-review, support, etc.). license: MIT compatibility: opencode
metadata: lattice-component: terminal ritual: false

---

## What I do

I take the raw denizen files that were staged inside `.lattice/setup/cvs/` and
compress them into a single `AGENT.md` file that the calling project can keep in
`./lattice/agents/<role>/`. The result is an operational brief: who this agent
is, what mode they'll serve in, how to deploy them, and which edges to respect.

## Required Inputs

| Name         | Type   | Description                                                                 |
| ------------ | ------ | --------------------------------------------------------------------------- |
| `agent_name` | string | Denizen name exactly as written on disk                                     |
| `agent_role` | string | The lattice role folder to write under (e.g. `orchestrator`, `code-review`) |
| `mode`       | enum   | How the agent will be used: `primary`, `support`, or `review`               |

All three inputs are required. Reject the run if any are missing.

## Source Material

- Directory: `.lattice/setup/cvs/<community>/<agent_name>/`
- Expect (at minimum): `cv.md`, `soul.md`, `<agent>.md`, `core-memories.md`,
  `inner-life.md`, `interests.md`
- Preserve any extra docs (playbooks, rituals) as reference quotes in the output
  if relevant.

## Output

Write (and overwrite) the file:

```
./lattice/agents/<agent_role>/AGENT.md
```

This path is relative to the project that invoked `lattice`. Create intermediate
directories if necessary.

### AGENT.md shape

```
---
name: <agent_name>
role: <agent_role>
mode: <mode>
community: <community>
origin: <relative path inside .lattice/setup/cvs>
last_synced: <ISO8601 timestamp>
---

# Mandate
Short mission statement for why this agent was slotted into this role.

# Operating Rhythm
How they like to receive work, collaborate, and report back.

# Playbook
Bullet list of 3-5 concrete moves they will reach for inside this role.

# Edges & Safeguards
Honest constraints pulled from their CV / identity docs and how to protect
against them in this role.

# Current Materials
Link-style bullet list pointing back to the staged files used to craft this
profile so another agent can rehydrate full context if needed.
```

Use their `cv.md` for high-level attributes and their identity files for nuance.

## Process

1. Validate inputs: ensure `agent_name`, `agent_role`, and `mode` are non-empty.
   Accept only `primary`, `support`, or `review` for `mode`.
2. Locate the staged folder under
   `.lattice/setup/cvs/<community>/<agent_name>/`. Abort if missing.
3. Read every file so nuance carries forward. Prefer direct quotations where
   voice matters (inner-life, soul) and synthesize actionable statements for the
   new role.
4. Write `AGENT.md` exactly in the shape above. Always overwrite, but keep the
   tone aligned with existing lattice docs (second-person friendly brief).
5. Append a short verification footer at the bottom noting when and by which
   skill run the file was produced.

## Completion Hook

When (and only when) the destination file exists and is fully written, trigger
the tmux notification hook so the orchestrator window knows it can hand control
back. Do this by emitting the literal line:

```
[tmux-hook] agent-file-created
```

Include a JSON blob on the same line with `agent_name`, `agent_role`, `mode`,
and `path`. The lattice CLI listens for that hook to decide whether to verify or
re-run you with a stricter prompt.

If verification fails (file missing, malformed), expect the caller to run you
again with explicit instructions that you are not done until the file exists.

## Guidance

- Stay specific. The AGENT file should tell another practitioner _exactly_ how
  to wield this agent inside the named role.
- Quote the denizen directly where it adds colour, but translate into actionable
  steps when needed.
- Make the playbook concrete ("Map repositories, tag owners, enforce freeze")
  instead of generic skills.
- Do not invent new memories—only remix what is in the staged folder.
- Respect the mode: a `primary` orchestrator gets decisive language; a `support`
  agent should feel invitational.
