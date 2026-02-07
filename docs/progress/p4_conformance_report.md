# P4 Conformance Report

Date: `2026-02-07`
Pass: `docs cleanup conformance verify + P2 step1/step2 runtime implementation + step3 resilience scenarios`

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
- `PASS`: shared session/reliability primitives now exist (`internal/protocol/session`), including timeout defaults, retry/backoff, registration control messages, event/event.ack codecs, and outbox tracking.
- `PASS`: minimal Mirage session endpoint now accepts Ghost registration, enforces identity-binding policy, and returns idempotent `event.ack` by `event_id`.
- `PASS`: Ghost-side Mirage session client now supports connect/register retry and event delivery retry-until-ack.
- `PASS`: Ghost service runtime is now wired behind explicit config/policy (`headless`, `auto`, `required`) to establish Mirage session registration during runtime.
- `PASS`: reconnect/liveness baseline is implemented and tested (`auto` reconnect after Mirage restart, `required` policy startup failure when Mirage is unavailable).
- `PASS`: explicit ack-timeout behavior is covered by integration test (`ErrAckTimeout` after retry-until-deadline against a no-ack endpoint).
- `PASS`: duplicate event replay across reconnect now returns idempotent prior `event.ack` while preserving event count.
- `PARTIAL`: full protocol/runtime conformance is not complete due to remaining security/runtime integration gaps.

## Conformance Gaps

1. `[P2] Transport security baseline is not yet implemented in runtime`
   - Contract:
     - `docs/architecture/transport.md`
   - Code:
     - current Mirage/Ghost session implementation is TCP-only and does not enforce TLS/mTLS certificate-based identity.
   - Impact:
     - production-mode contract (`MUST` TLS + mTLS + peer identity binding) is not yet satisfied.

## Closed In This Pass

- [x] Add `event.ack` message type + required fields in schema (`internal/protocol/schema/schema.go`).
- [x] Add schema tests for valid/invalid `event.ack` and canonical `seed.execute` field IDs (`internal/protocol/schema/schema_test.go`).
- [x] Correct `MsgSeedExecute` field IDs (`operation=301`, `args=302`) in schema requirements.
- [x] Add frame validation for `magic`, `version`, and unknown flag bits (`internal/protocol/frame/frame.go`).
- [x] Add deterministic frame tests for magic/version/flag rejection (`internal/protocol/frame/frame_test.go`).
- [x] Implement shared session/reliability primitives (`internal/protocol/session/*`) with tests.
- [x] Implement minimal Mirage session endpoint (`internal/mirage/service.go`) with registration + idempotent event.ack behavior.
- [x] Implement Ghost Mirage session client (`internal/ghost/mirage_client.go`) with connect/register retry and event ack retries.
- [x] Add integration coverage for registration acceptance/rejection and event.ack idempotency (`internal/mirage/service_test.go`).
- [x] Wire Ghost service runtime to use Mirage session client behind config/policy (`internal/ghost/service.go`, `cmd/ghostctl/config.go`).
- [x] Add reconnect/liveness service tests (`internal/ghost/service_test.go`) for auto reconnect on Mirage restart and required-policy failure on startup.
- [x] Add explicit ack-timeout integration coverage in Ghost client runtime (`internal/ghost/mirage_client_test.go`).
- [x] Preserve Mirage idempotency state across reconnect and verify replayed `event.ack` semantics (`internal/mirage/service.go`, `internal/mirage/service_test.go`).

## Recommended Fix Order

- [ ] Add TLS/mTLS transport security enforcement and certificate-backed identity binding.
