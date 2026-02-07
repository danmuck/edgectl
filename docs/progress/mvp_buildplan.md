# MVP Build Plan and Slices (Canonical)

Status: `Phase 4 complete; Phase 5 in planning; Phases 6-9 templated`

This is the canonical high-level schedule for `mvp_p0.md` through `mvp_p9.md`.
Use phase files for detailed task execution and acceptance check tracking.

## MVP Phase Schedule

- [x] Phase 0: `mvp_p0.md`
- [x] Phase 1: `mvp_p1.md`
- [x] Phase 2: `mvp_p2.md`
- [x] Phase 3: `mvp_p3.md`
- [x] Phase 4: `mvp_p4.md`
- [ ] Phase 5: `mvp_p5.md`
- [ ] Phase 6: `mvp_p6.md`
- [ ] Phase 7: `mvp_p7.md`
- [ ] Phase 8: `mvp_p8.md`
- [ ] Phase 9: `mvp_p9.md` (placeholder; scope to be restored)

## Slice Status

- [x] Slice docs created: `transport`, `frame`, `tlv_codec`, `semantic_validation`, `mirage_reconcile`, `ghost_dispatch`, `observability`
- [x] Code implementation started under `internal/*`

## Implementation Slices (Code)

- [x] 1. Transport/session baseline (`dial`, `listen`, session lifecycle, heartbeat hooks) via `internal/protocol/session` + Ghost/Mirage runtime wiring
- [x] 2. Protocol frame codec (`header`, `read_frame`, `write_frame`)
- [x] 3. TLV codec (`encode_field`, `decode_fields`, typed getters)
- [x] 4. Semantic validation (`validate_by_message_type`)
- [x] 5. Mirage reconcile interfaces + in-memory stores
- [x] 6. Ghost command dispatch + seed registry execution path
- [ ] 7. Structured logging/event schema (`component`, `peer`, `trace_id`, correlation IDs)

## MVP Build Plan

- [x] Milestone 1: Protocol core compiles
- [x] Implement `internal/protocol/frame` (header + frame reader/writer)
- [x] Implement `internal/protocol/tlv` (field reader/writer + typed accessors)
- [x] Implement `internal/protocol/semantic` (required field validation by `message_type`)
- [x] Add unit tests for malformed header, malformed tlv length, missing required fields

- [x] Milestone 2: Session path works
- [x] Implement Ghost outbound session manager (`dial`, handshake, heartbeat, reconnect)
- [x] Implement Mirage inbound acceptor + identity binding to `ghost_id`
- [x] Implement registration handshake (`seed` payload + ack)
- [x] Add integration test for connect -> register -> ready

- [ ] Milestone 3: Mirage single-intent orchestration loop works
- [x] Implement Mirage in-memory intent and observed stores
- [x] Implement Mirage minimal reconcile loop (single command target)
- [x] Implement Ghost command routing to seed registry and execution adapter
- [x] Implement event emission and Mirage report generation
- [ ] Add Mirage e2e test: `issue -> command -> seed.execute -> seed.result -> event -> report`

- [ ] Milestone 4: MVP hardening gate
- [x] Add retry/backoff + idempotency behavior per `reliability.toml`
- [ ] Add structured logs per `observability.toml`
- [ ] Add error mapping per `errors.toml`
- [x] Add failure-path tests (disconnect, duplicate IDs, timeout, validation failures)

- [ ] Milestone 5: Mirage orchestration loop baseline (`mvp_p5.md`)
- [x] Implement `issue` ingestion + desired-state persistence
- [x] Implement reconcile loop (single-ghost first)
- [x] Dispatch commands and ingest events into observed state
- [x] Emit `report` to user boundary
- [x] Enable Mirage local Ghost spin-up path
- [x] Add Mirage admin control boundary and runtime reconcile actions
- [x] Wire `mirage.toml` + `ghost.toml` coupling for local Ghost admin controller boot
- [x] Add temporary persistence seeds (`seed.kv` and `seed.fs`) and route buildlog persistence through seed execution

- [ ] Milestone 6: Boundary transport integration (`mvp_p6.md`)
- [ ] Bind control-plane links to protocol envelopes (not ad-hoc calls)
- [ ] Replace direct action-style HTTP shortcuts between Mirage and Ghost
- [ ] Wire optional auth block handling and validation hooks
- [ ] Add contract tests for all boundaries

- [ ] Milestone 7: End-to-end control loop validation (`mvp_p7.md`)
- [ ] Add E2E scenario: intent -> command -> seed execution -> event -> report
- [ ] Add deterministic logs for ownership transitions
- [ ] Add E2E failure scenario with corrective behavior

- [ ] Milestone 8: Hardening completion (`mvp_p8.md`)
- [ ] Add idempotency strategy for repeated commands/events
- [ ] Add duplicate/replay event handling
- [ ] Add protocol version-compatibility behavior checks
- [ ] Add timeout/retry policies and terminal error states

- [ ] Milestone 9: MVP finalization (`mvp_p9.md`)
- [ ] Recover original Phase 9 definition
- [ ] Define MVP exit criteria and final release validation matrix

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

- [x] Add `event.ack` message type and required field validation in `internal/protocol/schema`.
- [x] Add schema tests for valid/invalid `event.ack`.
- [x] Add frame unknown-flag rejection behavior and tests in `internal/protocol/frame`.
- [x] Close remaining runtime conformance gaps against transport/handshake/reliability contracts.

Current state:
- P4 conformance transport/handshake/reliability baseline is closed (`docs/progress/p4_conformance_report.md`).
- Remaining MVP gaps are Mirage Phase 5 orchestration concerns (`issue`/desired-state/reconcile/report).
