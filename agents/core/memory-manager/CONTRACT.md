# Memory Manager Contract

Contract version: 1

## Inputs

- reflection-batch: Raw reflections from agents after a work cycle.
- agent-identity-files: Current state of agent identity files.

## Outputs

- updated-identity-files: Agent files with distilled memories integrated.
- cycle-summary: Summary for the community-memory agent.

## Required Skills

- memory-distill: input is a single agent reflection plus identity files; output
  is updated identity files.
- memory-summary: input is all reflections from the cycle; output is a summary
  document for the community-memory agent.

## Behaviors

- MUST preserve agent voice when writing memories.
- MUST age older memories (fade, do not delete).
- MUST NOT speak directly to agents.
- MUST complete all distillations before the cycle closes.
