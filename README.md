# EdgeCTL

EdgeCTL is a control-plane system with:
- `Mirage`: orchestration authority (desired state + reconciliation)
- `Ghost`: execution authority (command routing + seed dispatch)
- `Seed`: service interface surface exposed by Ghosts

Current repo state is documentation-first while implementation is rebuilt from scratch.

## Documentation

Start here:
- [`docs/index.md`](docs/index.md)

## Servers and Packaging Targets

- Mirage runtime entrypoint: `cmd/miragectl`
- Ghost runtime entrypoint: `cmd/ghostctl`
- Protocol package: `internal/protocol`
- Mirage package: `internal/mirage`
- Ghost package: `internal/ghost`
- Seeds package: `internal/seeds`

## Current Gaps (Docs Backlog)

- Normative wire-level behavior:
  - encode/decode error matrix and malformed-frame handling table
  - stream framing and maximum payload policy
- Runtime contracts:
  - Mirage/Ghost startup lifecycle and registration handshake sequence
  - seed capability advertisement and version compatibility policy
- State model details:
  - command lifecycle states and retry/idempotency rules
  - event ordering, deduplication, and correlation guarantees
- Operations:
  - configuration schema (`miragectl` and `ghostctl`)
  - local development bootstrap and integration test plan

## Status

Phase 0 is focused on terminology and contract freeze before implementation.
