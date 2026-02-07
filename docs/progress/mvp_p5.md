# MVP Progress — Phase 5 (Mirage Orchestration Layer)

Status: `Not Started`

## Phase Goal

- [ ] Establish Mirage as an orchestration boundary that ingests user-facing `issue` state, reconciles desired vs observed state, drives Ghost command envelopes, and emits user-facing reports.

## Tasks (Buildplan-Aligned)

- [ ] Define `issue` ingestion contract and desired-state persistence model
- [ ] Implement `issue` ingestion path and desired-state store (in-memory first)
- [ ] Implement reconcile loop (single-ghost first)
- [ ] Dispatch protocol command envelopes and ingest protocol event envelopes into observed state
- [ ] Emit `report` to user boundary with explicit desired vs observed transitions
- [ ] Mirage must be able to spin up local Ghost servers
- [ ] Update architecture/message-flow diagrams in `docs/architecture/models` for Phase 5 behavior

### Acceptance Checks

- [ ] One intent drives command execution and produces report updates
- [ ] Desired vs observed state transitions are explicit and testable
- [ ] Phase 5 contracts are reflected in docs and corresponding test coverage

## Coexisting Phase-Break Flow

- [ ] `pbreak_client_terminal.md` remains a coexisting workstream and does not block Mirage Phase 5 orchestration execution.
