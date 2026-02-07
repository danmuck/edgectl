# P4 Conformance Report

Date: `2026-02-07`
Pass: `docs cleanup conformance verify`

Scope:
- verify current code behavior against canonical protocol/design docs
- identify implementation gaps that must be closed before claiming full conformance

## Verification Inputs

- Commands:
  - `go test ./...` (`PASS`)
  - repo-wide contract/code scans with `rg` and file-level review
- Canonical contracts checked:
  - `docs/architecture/definitions/tlv.toml`
  - `docs/architecture/definitions/protocol.toml`
  - `docs/architecture/framing.md`
  - `docs/architecture/transport.md`
  - `docs/architecture/definitions/handshake.toml`
  - `docs/architecture/definitions/reliability.toml`

## Verification Summary

- `PASS`: canonical docs are parseable and navigable.
- `PASS`: package `doc.go` stubs include canonical references.
- `PASS`: unknown TLV fields are decoded and preserved as inert data in the protocol path.
- `PASS`: schema now includes `event.ack` message type and required-field validation.
- `PASS`: `seed.execute` schema field IDs now match canonical TLV IDs (`301`, `302`).
- `PASS`: frame decode now rejects unsupported `magic`, `version`, and unknown flag bits.
- `PARTIAL`: full protocol/runtime conformance is not complete due to pending transport/session runtime behavior.

## Conformance Gaps

1. `[P2] Transport session, handshake, and retry/ack runtime behavior is still pending`
   - Contract:
     - `docs/architecture/transport.md`
     - `docs/architecture/definitions/handshake.toml`
     - `docs/architecture/definitions/reliability.toml`
   - Code:
     - `internal/mirage/service.go` is a runtime skeleton.
     - `internal/ghost/service.go` is standalone lifecycle/heartbeat without Mirage session handshake/reliability path.
   - Impact:
     - normative connection, identity bind, registration ack, and event retry/ack behavior are not yet enforced by runtime.

## Closed In This Pass

- [x] Add `event.ack` message type + required fields in schema (`internal/protocol/schema/schema.go`).
- [x] Add schema tests for valid/invalid `event.ack` and canonical `seed.execute` field IDs (`internal/protocol/schema/schema_test.go`).
- [x] Correct `MsgSeedExecute` field IDs (`operation=301`, `args=302`) in schema requirements.
- [x] Add frame validation for `magic`, `version`, and unknown flag bits (`internal/protocol/frame/frame.go`).
- [x] Add deterministic frame tests for magic/version/flag rejection (`internal/protocol/frame/frame_test.go`).

## Recommended Fix Order

- [ ] Implement Mirage<->Ghost handshake/session and reliability contracts.
