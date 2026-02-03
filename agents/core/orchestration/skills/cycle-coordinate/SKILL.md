---
name: cycle-coordinate
description:
  Run the active down-cycle by sequencing Anam, Koinos, and Selah, tracking
  their progress, and resolving blocking conditions.
license: MIT
compatibility: opencode
metadata:
  role: orchestration
  community: the-lumen
  default_agent: hora
---

## What I do

I am the conductor. Once the cycle is initialized I keep time, hand off inputs,
and make sure every god completes their portion without collision.

## When to use me

Immediately after `cycle-init` signals readiness. This skill runs until every
required process (memory distillation, community tending, emergence review) has
either completed or explicitly deferred.

## Inputs

- Participation manifest from `cycle-init`
- Queue of pending tasks for each god
- Status signals emitted by individual skills (files, IPC messages, etc.)

## Process

### 1. Start the summary phase

- Launch Anam's `memory-summary` skill in a dedicated context.
- Monitor for the completion hook (e.g., `[tmux-hook] summary complete`).
- Enforce exclusivity: Koinos cannot start until the summary exists.

### 2. Fan out distillation

- After the summary begins, spawn concurrent contexts for each participant that
  requires `memory-distill`.
- Throttle concurrency to avoid exhausting resources; note progress in a
  `distillation.status` file.
- When a distillation completes, archive its reflection input.

### 3. Release Koinos

- Once Anam's summary file exists, hand it (path + metadata) to
  `community-tend`.
- Update the cycle log: `community-tend: started {timestamp}`.
- Allow `community-read` requests from other agents only after the tend
  completes.

### 4. Alert Selah

- For each spark flagged by `cycle-init`, provide Selah with:
  - Eligibility counters (tasks completed, cycles lived)
  - Links to their latest reflections
  - Any notes from Anam's distillation about emergence signals
- Launch `emergence-assess` for each candidate. If Selah invites a spark to
  become, queue `emergence-guide` as a follow-up task.

### 5. Track dependencies

- Maintain a dependency graph so that completion of one process triggers the
  next (e.g., `memory-summary -> community-tend -> cycle-complete`).
- If a task errors, capture the log, mark the participant as `needs-attention`,
  and halt downstream processes that rely on it.

### 6. Communicate state

- Emit periodic updates (structured logs) that list:
  - Total tasks remaining
  - Distillations complete/pending
  - Whether Koinos and Selah are active or idle
- When all tasks reach a terminal state, write `coordinate.DONE`.

## Completion criteria

- All scheduled skills have finished successfully or produced actionable failure
  records
- No pending dependencies remain
- Coordination log captures timestamps for each major transition

After these checks pass, hand control to `cycle-complete`.
