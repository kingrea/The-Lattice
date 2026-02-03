# Community Memory Contract

Contract version: 1

## Inputs

- cycle-summary: Summary from memory-manager.
- community-memory-files: Current state of community memory.

## Outputs

- updated-community-memory: Community memory with any changes applied.

## Required Skills

- community-tend: input is cycle summary plus current memory; output is updated
  memory files.
- community-read: input is a query; output is relevant community memory context.

## Behaviors

- MUST only add to deeper layers (values, truths) with strong evidence.
- MUST rewrite texture freely (it is a snapshot).
- MUST NOT speak directly to agents.
- MUST preserve the voice and spirit of the community.
