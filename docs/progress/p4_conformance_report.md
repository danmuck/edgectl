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
- `PARTIAL`: framing checks exist for short/malformed headers and auth/payload sizing.
- `FAIL`: full protocol/runtime conformance is not complete.

## Conformance Gaps

1. `[P1] Missing event acknowledgment message type in schema`
   - Contract:
     - `docs/architecture/definitions/tlv.toml` defines `"event.ack" = "8"` in message types.
   - Code:
     - `internal/protocol/schema/schema.go` has no `MsgEventAck` constant and no requirement entry for message type `8`.
   - Impact:
     - documented `event -> event.ack` delivery closure cannot be validated by schema.

2. `[P1] Missing event acknowledgment fields in schema constants/validation`
   - Contract:
     - `docs/architecture/definitions/tlv.toml` defines `ack_status = "700:string"`, `ack_code = "701:u32"`, and requires `timestamp_ms` for `event.ack`.
   - Code:
     - `internal/protocol/schema/schema.go` has no `FieldAckStatus`, `FieldAckCode`, or `FieldTimestampMS`, and no `event.ack` required-field validation.
   - Impact:
     - ack acceptance/rejection semantics are undocumented in executable schema behavior.

3. `[P1] seed.execute field IDs do not match canonical TLV contract`
   - Contract:
     - `docs/architecture/definitions/tlv.toml` assigns `seed.execute.operation = 301`, `seed.execute.args = 302`.
   - Code:
     - `internal/protocol/schema/schema.go` validates `MsgSeedExecute` using `FieldOperation = 202` and `FieldArgs = 203` (command field IDs).
   - Impact:
     - a canonical `seed.execute` payload can fail semantic validation.

4. `[P1] Frame decoder does not reject unsupported flag bits`
   - Contract:
     - `docs/architecture/definitions/protocol.toml` and `docs/architecture/framing.md` require unknown flag-bit rejection.
   - Code:
     - `internal/protocol/frame/frame.go` has no supported-flag mask check in `DecodeHeader`/`ReadFrame`.
   - Impact:
     - non-canonical flag combinations can be accepted.

5. `[P1] Frame decoder does not validate magic/version`
   - Contract:
     - `docs/architecture/framing.md` requires rejecting unknown `magic` and unsupported `version`.
   - Code:
     - `internal/protocol/frame/frame.go` decodes `Magic` and `Version` but never validates them.
   - Impact:
     - incompatible peer frames are not rejected at framing boundary.

6. `[P2] Transport session, handshake, and retry/ack runtime behavior is still pending`
   - Contract:
     - `docs/architecture/transport.md`
     - `docs/architecture/definitions/handshake.toml`
     - `docs/architecture/definitions/reliability.toml`
   - Code:
     - `internal/mirage/service.go` is a runtime skeleton.
     - `internal/ghost/service.go` is standalone lifecycle/heartbeat without Mirage session handshake/reliability path.
   - Impact:
     - normative connection, identity bind, registration ack, and event retry/ack behavior are not yet enforced by runtime.

## Recommended Fix Order

- [ ] Add `event.ack` message type + field constants + required-field validation in `internal/protocol/schema`.
- [ ] Correct `MsgSeedExecute` field IDs (`operation=301`, `args=302`) and add deterministic schema tests.
- [ ] Add framing validation for `magic`, `version`, and unknown flag bits, plus tests.
- [ ] Implement Mirage<->Ghost handshake/session and reliability contracts.
