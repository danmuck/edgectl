# MVP Progress — Phase 5 (Mirage Orchestration Layer)

Status: `In Progress`

## Phase Goal

- [ ] Establish Mirage as an orchestration boundary that ingests user-facing `issue` state, reconciles desired vs observed state, drives Ghost command envelopes, and emits user-facing reports.

## Tasks (Buildplan-Aligned)

- [x] Define the Mirage server controller, similar to ghosts
- [x] Define `issue` ingestion contract and desired-state persistence model
- [x] Implement `issue` ingestion path and desired-state store (in-memory first)
- [x] Implement reconcile loop (single-ghost first)
- [x] Dispatch protocol command envelopes and ingest protocol event envelopes into observed state
- [x] Emit `report` to user boundary with explicit desired vs observed transitions
- [x] Mirage must be able to spin up local Ghost servers
- [x] Introduce Mirage server boundary (`internal/mirage/server.go`) for lifecycle + orchestration command boundary ownership
- [x] Update architecture/message-flow diagrams in `docs/architecture/models` for Phase 5 behavior

### Acceptance Checks

- [x] One intent drives command execution and produces report updates
- [x] Desired vs observed state transitions are explicit and testable
- [x] Phase 5 contracts are reflected in docs and corresponding test coverage

## Coexisting Phase-Break Flow

- [ ] `pbreak_client_terminal.md` remains a coexisting workstream and does not block Mirage Phase 5 orchestration execution.
