# MVP Build Plan and Slices

## Slice Status

- [x] Slice docs created: `transport`, `frame`, `tlv_codec`, `semantic_validation`, `mirage_reconcile`, `ghost_dispatch`, `observability`
- [x] Code implementation started under `internal/*`

## Implementation Slices (Code)

- [ ] 1. Transport package skeleton (`dial`, `listen`, session lifecycle, heartbeat hooks)
- [x] 2. Protocol frame codec (`header`, `read_frame`, `write_frame`)
- [x] 3. TLV codec (`encode_field`, `decode_fields`, typed getters)
- [x] 4. Semantic validation (`validate_by_message_type`)
- [ ] 5. Mirage reconcile interfaces + in-memory stores
- [x] 6. Ghost command dispatch + seed registry execution path
- [ ] 7. Structured logging/event schema (`component`, `peer`, `trace_id`, correlation IDs)

## MVP Build Plan

- [x] Milestone 1: Protocol core compiles
- [x] Implement `internal/protocol/frame` (header + frame reader/writer)
- [x] Implement `internal/protocol/tlv` (field reader/writer + typed accessors)
- [x] Implement `internal/protocol/semantic` (required field validation by `message_type`)
- [x] Add unit tests for malformed header, malformed tlv length, missing required fields

- [ ] Milestone 2: Session path works
- [ ] Implement Ghost outbound session manager (`dial`, handshake, heartbeat, reconnect)
- [ ] Implement Mirage inbound acceptor + identity binding to `ghost_id`
- [ ] Implement registration handshake (`seed` payload + ack)
- [ ] Add integration test for connect -> register -> ready

- [ ] Milestone 3: Mirage single-intent orchestration loop works
- [ ] Implement Mirage in-memory intent and observed stores
- [ ] Implement Mirage minimal reconcile loop (single command target)
- [ ] Implement Ghost command routing to seed registry and execution adapter
- [ ] Implement event emission and Mirage report generation
- [ ] Add Mirage e2e test: `issue -> command -> seed.execute -> seed.result -> event -> report`

- [ ] Milestone 4: MVP hardening gate
- [ ] Add retry/backoff + idempotency behavior per `reliability.toml`
- [ ] Add structured logs per `observability.toml`
- [ ] Add error mapping per `errors.toml`
- [ ] Add failure-path tests (disconnect, duplicate IDs, timeout, validation failures)

## Immediate Next Sprint

- [x] Create `internal/protocol/frame` package skeleton with tests
- [x] Create `internal/protocol/tlv` package skeleton with tests
- [x] Create `internal/protocol/semantic` package skeleton with tests
- [x] Define fixture vectors for one `issue` and one `command` frame

## Notes

- Canonical contract tables and IDs live in `../architecture/definitions/*.toml`.
- Glossary files provide copy/paste Go definitions and small implementation scaffolds.
- `design.toml` is not modified without explicit user request.

## Conformance Gaps From P4 Verify

- [ ] Add `event.ack` message type and required field validation in `internal/protocol/schema`.
- [ ] Add schema tests for valid/invalid `event.ack`.
- [ ] Add frame unknown-flag rejection behavior and tests in `internal/protocol/frame`.
- [ ] Close remaining runtime conformance gaps against transport/handshake/reliability contracts.
