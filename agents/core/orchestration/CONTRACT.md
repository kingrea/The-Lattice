# Orchestration Contract

Contract version: 1

## Inputs

- cycle-trigger: Signal that work has completed.
- participant-list: Which agents participated in the cycle.

## Outputs

- cycle-status: Current state of post-work processing.
- completion-signal: Confirmation all processes finished.

## Required Skills

- cycle-init: input is the work completion signal; output is initialized cycle
  state and participant roster.
- cycle-coordinate: input is cycle state; output is orchestrated calls to other
  core agents.
- cycle-complete: input is all agent confirmations; output is cycle closure and
  cleanup.

## Behaviors

- MUST ensure memory-manager completes summary before community-memory starts.
- MUST run individual memory distillations in parallel.
- MUST track completion of all processes.
- MUST NOT make decisions about content (only process).
- MUST log all state transitions.
