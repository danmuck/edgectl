# Future API v1 Proposal Tracker

Status: `Proposed`

## Goal

- Define a concrete future `api/v1` boundary that maps to canonical Ghost/Mirage contracts without changing wire-level protocol ownership.

## Tasks

- [x] Add proposal document for future API v1 northbound scope (`docs/architecture/future_api_v1.md`)
- [x] Add architecture boundary model for future API v1 (`docs/architecture/models/future_api_v1_boundary.mmd`)
- [x] Add issue/reconcile message-flow model for future API v1 (`docs/architecture/models/future_api_v1_issue_reconcile_flow.mmd`)
- [x] Link proposal artifacts from docs navigation index (`docs/index.md`)

## Acceptance Checks

- [x] Proposal clearly separates public API boundary from internal Mirage<->Ghost protocol
- [x] Scope includes endpoint surface, idempotency, retry/timeout behavior, and error taxonomy mapping
- [x] Models are text-native and version-control friendly (`.mmd`)
