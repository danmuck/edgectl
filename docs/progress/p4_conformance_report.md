# P4 Conformance Report

Date: `2026-02-07`

Scope:
- verify current code behavior against canonical protocol/design docs
- identify implementation gaps that must be closed before claiming full conformance

## Verification Summary

- `PASS`: canonical docs are parseable and navigable.
- `PASS`: package `doc.go` stubs include canonical references.
- `PASS`: unknown TLV fields are decoded and carried without semantic branching in protocol decode/validate path.
- `PARTIAL`: framing checks are present for malformed header/auth/payload sizing.
- `FAIL`: full protocol conformance is not yet complete in code.

## Conformance Gaps

1. `event.ack` message type is documented but not present in semantic schema constants/requirements.
   - Contract: `docs/architecture/definitions/tlv.toml`
   - Code: `internal/protocol/schema/schema.go`

2. `event.ack` fields (`ack_status`, `ack_code`) are documented but not present in schema field constants/requirements.
   - Contract: `docs/architecture/definitions/tlv.toml`
   - Code: `internal/protocol/schema/schema.go`

3. Unknown frame flag bits must be rejected per protocol/framing docs, but frame decode path does not enforce this yet.
   - Contract: `docs/architecture/definitions/protocol.toml`, `docs/architecture/framing.md`
   - Code: `internal/protocol/frame/frame.go`

4. Transport/session and handshake behavior is documented as normative, but corresponding runtime implementation is still pending.
   - Contract: `docs/architecture/transport.md`, `docs/architecture/definitions/handshake.toml`, `docs/architecture/definitions/reliability.toml`
   - Code status: pending in MVP Milestone 2

## Next Implementation Tasks (from this report)

- [ ] Add `event.ack` message type and required field validation in `internal/protocol/schema`.
- [ ] Add schema tests for valid/invalid `event.ack`.
- [ ] Add frame flag-mask validation and tests for unknown/unsupported flags.
- [ ] Implement handshake/session path and reliability behavior per contracts.
