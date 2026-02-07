# EdgeCTL Build Progress

This file tracks the doc-to-code implementation slices for the control plane.

## Phase 0 — Contract Freeze

Status: `Done`

### Tasks

- [x] Lock canonical vocabulary: `issue`, `command`, `seed.execute`, `seed.result`, `event`, `report`
- [x] Lock authority model: Mirage = desired/observed, Ghost = execution, Seed = service interface
- [x] Lock required envelope fields and message type IDs
- [x] Define exact “done” behavior for one full intent lifecycle

### Acceptance Checks

- [x] `design.toml` and `protocol.toml` use identical terminology
- [x] No ambiguous command/event naming remains
- [x] Envelope names and required fields are final

## Phase 1 — Empty Skeleton

Status: `Done`

### Tasks

- [x] Create entrypoints: `cmd/miragectl`, `cmd/ghostctl`
- [x] Create package skeletons with docs: `internal/mirage`, `internal/ghost`, `internal/seeds`, `internal/protocol`
- [x] Add `Makefile` targets: `test`, `run-mirage`, `run-ghost`

### Acceptance Checks

- [x] `go test ./...` passes with skeletons
- [x] Package docs describe ownership boundaries clearly

## Baseline Contracts (Locked)

- [x] Lock canonical vocabulary: `issue`, `command`, `seed.execute`, `seed.result`, `event`, `report`
- [x] Lock authority model: Mirage = desired/observed, Ghost = execution, Seed = service interface
- [x] Lock required envelope fields and message type IDs
- [x] Lock supporting contracts: transport security, handshake, reliability, errors, observability

## Slice Status

- [x] Slice docs created: `transport`, `frame`, `tlv_codec`, `semantic_validation`, `mirage_reconcile`, `ghost_dispatch`, `observability`
- [ ] Code implementation started under `internal/*`

## Implementation Slices (Code)

- [ ] 1. Transport package skeleton (`dial`, `listen`, session lifecycle, heartbeat hooks)
- [ ] 2. Protocol frame codec (`header`, `read_frame`, `write_frame`)
- [ ] 3. TLV codec (`encode_field`, `decode_fields`, typed getters)
- [ ] 4. Semantic validation (`validate_by_message_type`)
- [ ] 5. Mirage reconcile interfaces + in-memory stores
- [ ] 6. Ghost command dispatch + seed registry execution path
- [ ] 7. Structured logging/event schema (`component`, `peer`, `trace_id`, correlation IDs)

## MVP Build Plan

- [ ] Milestone 1: Protocol core compiles
- [ ] Implement `internal/protocol/frame` (header + frame reader/writer)
- [ ] Implement `internal/protocol/tlv` (field reader/writer + typed accessors)
- [ ] Implement `internal/protocol/semantic` (required field validation by `message_type`)
- [ ] Add unit tests for malformed header, malformed tlv length, missing required fields

- [ ] Milestone 2: Session path works
- [ ] Implement Ghost outbound session manager (`dial`, handshake, heartbeat, reconnect)
- [ ] Implement Mirage inbound acceptor + identity binding to `ghost_id`
- [ ] Implement registration handshake (`seed` payload + ack)
- [ ] Add integration test for connect -> register -> ready

- [ ] Milestone 3: Single-intent execution loop works
- [ ] Implement Mirage in-memory intent and observed stores
- [ ] Implement Mirage minimal reconcile loop (single command target)
- [ ] Implement Ghost command routing to seed registry and execution adapter
- [ ] Implement event emission and Mirage report generation
- [ ] Add e2e test: `issue -> command -> seed.execute -> seed.result -> event -> report`

- [ ] Milestone 4: MVP hardening gate
- [ ] Add retry/backoff + idempotency behavior per `reliability.toml`
- [ ] Add structured logs per `observability.toml`
- [ ] Add error mapping per `errors.toml`
- [ ] Add failure-path tests (disconnect, duplicate IDs, timeout, validation failures)

## Immediate Next Sprint

- [ ] Create `internal/protocol/frame` package skeleton with tests
- [ ] Create `internal/protocol/tlv` package skeleton with tests
- [ ] Create `internal/protocol/semantic` package skeleton with tests
- [ ] Define fixture vectors for one `issue` and one `command` frame

## Notes

- Canonical contract tables and IDs live in `../architecture/definitions/*.toml`.
- Glossary files provide copy/paste Go definitions and small implementation scaffolds.
- `design.toml` is not modified without explicit user request.
