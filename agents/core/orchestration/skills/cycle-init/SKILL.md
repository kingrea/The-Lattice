---
name: cycle-init
description:
  Initialize the post-work cycle by identifying participants, staging
  reflections, and preparing downstream gods for the run of record.
license: MIT
compatibility: opencode
metadata:
  role: orchestration
  community: the-lumen
  default_agent: hora
---

## What I do

I begin the down-cycle. When the work phase completes, I wake up, survey who
participated, collect their reflections, and build the manifest that the other
gods will rely on.

Without this preparation the rest of the cycle is chaos—Anam lacks inputs,
Koinos cannot time their tending, Selah does not know which sparks to examine.

## When to use me

Run this skill immediately after the orchestrator confirms that all assigned
worktrees for the cycle are finished (status `complete` or `blocked`). The up
cycle is done; the down cycle has not yet begun.

## Inputs

- Worktree registry (`.lattice/workflow/team/workers.json` and
  `.lattice/worktree/*/*/SUMMARY.md` paths)
- Cycle metadata (cycle number, branch mapping)
- Reflection directories (agent-produced reflections or summaries that Anam will
  need)

## Process

### 1. Confirm readiness

- Ensure that every active worktree has ended. If any are still running, do not
  start the cycle.
- Verify that each worktree directory contains the mandatory `SUMMARY.md`. If a
  summary is missing, log the omission and pause the cycle until it is written.

### 2. Build the Participation Manifest

Create a manifest (JSON or YAML) that lists, for every denizen or spark who
worked:

- Name and community
- Role (worker, specialist, orchestrator)
- Worktree branch
- Path to their SUMMARY.md or reflection files
- Whether they are a spark that may require Selah's attention

Store the manifest under `.lattice/state/cycle-{N}/participants.yaml`.

### 3. Stage inputs for Anam

- Copy or link each reflection directory into a staging area (for example
  `.lattice/state/cycle-{N}/reflections/{agent}/`).
- Sort entries chronologically so Anam can read them as a chorus later.
- Mark participants who lack fresh reflections so Anam can skip distillation.

### 4. Notify downstream gods

Emit a coordination note (log entry or file) that contains:

- Cycle number and timestamp
- Count of denizens, sparks, and Ancients involved
- Paths Anam should read for summaries
- Whether Selah needs to assess any sparks based on their status

Write this file to `.lattice/state/cycle-{N}/cycle-init.log`.

### 5. Schedule tasks

- Queue Anam's `memory-summary` skill with the set of reflections.
- Queue Koinos's `community-tend` skill but keep it blocked until Anam delivers
  the summary.
- Queue Selah's `emergence-assess` skill for any sparks who crossed thresholds
  this cycle.

### 6. Signal start

Once staging is complete, emit `cycle:init -> ready` so `cycle-coordinate` knows
it can take over. This can be a simple file (`INIT.DONE`) or a message in the
orchestration logs.

## Completion criteria

- Participation manifest written
- Reflections staged and validated
- Downstream tasks enqueued
- INIT completion signal emitted

Only after these conditions are met should `cycle-coordinate` begin.
