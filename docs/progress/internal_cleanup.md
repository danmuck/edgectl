# Internal Cleanup Tracker

**Status:** `In Progress`
**Scope:** `internal_cleanup`

## 2026-02-09 Pass

- [x] Consolidate repeated "recent N events" slice-copy logic in `internal/ghost/admin_control.go`.
- [x] Remove duplicate stage/command validation in Mirage submit path by making `IssueEnv.Validate()` the single validation gate in `internal/mirage/orchestration.go`.
- [x] Simplify Mirage admin issue mapping via a dedicated command mapper helper in `internal/mirage/admin_control.go`.
- [x] Deduplicate TLV frame encoding flow in `internal/protocol/session` with one shared helper (`wire_encode.go`).
- [x] Validate compatibility with protocol contracts in `docs/architecture/definitions/protocol.toml` and `docs/architecture/definitions/tlv.toml`.
- [x] Run full repository tests (`go test ./...`).

## Guardrails

- No changes to canonical architecture contracts or models.
- No wire field IDs, message types, or required-field semantics changed.
- Refactors are structural/readability-only.
