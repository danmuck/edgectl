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

## Phase 2 — Protocol Core (No Runtime Coupling)

Status: `Done`

### Tasks

- [x] Implement fixed header + optional auth + TLV encoder/decoder
- [x] Implement semantic schema validation layer
- [x] Define schemas for all boundary envelopes
- [x] Add tests: round-trip, malformed input, unknown fields, missing required fields

### Acceptance Checks

- [x] Protocol tests pass independently of Mirage/Ghost runtime
- [x] Unknown fields preserved/ignored per contract
- [x] Invalid wire data fails deterministically

## Baseline Contracts (Locked)

- [x] Lock canonical vocabulary: `issue`, `command`, `seed.execute`, `seed.result`, `event`, `report`
- [x] Lock authority model: Mirage = desired/observed, Ghost = execution, Seed = service interface
- [x] Lock required envelope fields and message type IDs
- [x] Lock supporting contracts: transport security, handshake, reliability, errors, observability

## Phase 3 — Seed Interface Layer

**Status:** `Done`

### Tasks

- [x] Define seed metadata contract: `id`, `name`, `description`
- [x] Implement seed registry API
- [x] Implement one deterministic seed (`flow`) returning stable results
- [x] Add tests for registry and seed action behavior

### Acceptance Checks

- [x] Ghost can invoke seed actions locally with deterministic output
- [x] Seed metadata is available and validated

1.

- [x] Lock seed interfaces in code (minimal, explicit)
- [x] Define `SeedMetadata` with `id`, `name`, `description`
- [x] Define `Seed` interface with:
- [x] `Metadata() SeedMetadata`
- [x] `Execute(action string, args map[string]string) (SeedResult, error)`
- [x] Define `SeedResult` with deterministic fields: `status`, `stdout`, `stderr`, `exit_code`

2.

- [x] Implement registry as pure in-memory contract
- [x] `Register(seed Seed) error` (reject duplicate `id`)
- [x] `Resolve(id string) (Seed, bool)`
- [x] `ListMetadata() []SeedMetadata` (stable sort by `id` for deterministic tests)
- [x] `ValidateMetadata(meta SeedMetadata) error` (non-empty fields + id format guard)

3.

- [x] Implement deterministic `flow` seed
- [x] Seed id: `seed.flow`
- [x] Supported action set:
- [x] `status` -> always success, stable stdout payload
- [x] `echo` -> returns deterministic render from `args` (canonical key ordering)
- [x] `step` -> deterministic pseudo-step output from fixed mapping
- [x] Unknown action -> deterministic error result (`status=error`, stable message, non-zero exit)

4.

- [x] Add unit tests (no runtime coupling)

  Registry:

- [x] register success
- [x] duplicate register failure
- [x] resolve/list behavior
- [x] metadata validation failures

  Flow seed:

- [x] metadata correctness
- [x] supported actions return byte-for-byte stable output
- [x] unknown action deterministic failure
- [x] arg-order independence test for `echo`

5.

- [x] Update progress tracking
- [x] Add Phase 3 section to `docs/progress.md` (mirror your checklist)
- [x] Mark items as they pass
- [x] Run `go test ./...` and close acceptance checks only when green

## Why this shape is best

- [x] Keeps Phase 3 independent from transport/runtime orchestration.
- [x] Produces a stable local execution API for Phase 4 Ghost execution layer.
- [x] Gives deterministic fixtures you can reuse for protocol/e2e tests later.
- [x] Minimizes redesign risk by locking seed contracts before distributed wiring.

## Suggested concrete file targets

- [x] `internal/seeds/types.go` (metadata/result/contracts)
- [x] `internal/seeds/registry.go` (registry + validation)
- [x] `internal/seeds/flow.go` (deterministic seed)
- [x] `internal/seeds/registry_test.go`
- [x] `internal/seeds/flow_test.go`
- [x] `docs/progress.md` (Phase 3 mirror + status)

### Verify smplog interface is clean, and is able to run zerolog naked

- [ ] No bugs in smplog
- [x] Add smplog output throughout all tests
- [x] Add smplog output to describe actions and state change for all functions
- [x] Everything happening should be logged via smplog
- [x] Use them like colors, rather than heuristical titles for nice output

## Phase 4 — Ghost Execution Layer

Status: `Done`

### Tasks

- [x] Lock Ghost execution flow in docs (`command -> seed.execute -> seed.result -> event`) and add/update a message-flow diagram in `docs/architecture/models/`
- [x] Define minimal Ghost contracts in `internal/ghost` for executor, event emitter, and execution state with required correlation fields (`message_id`, `command_id`, `execution_id`, `trace_id`)
- [x] Implement Ghost command input boundary handler for `command` envelopes with Ghost-level semantic guards
- [x] Implement deterministic execution pipeline: resolve seed, execute action, normalize `seed.result`, emit terminal `event` (`success` or `error`)
- [x] Implement in-memory execution store keyed by `execution_id` and indexed by `command_id`
- [x] Add query methods for execution correlation (`GetExecution`, `GetByCommandID`)
- [x] Add tests for success path, unknown seed, unknown action, seed error path, and correlation/state query checks
- [x] Verify acceptance: every valid command yields one valid terminal event
- [x] Verify acceptance: execution state is queryable and correlated by command/execution IDs
- [x] Update `docs/progress.md` as each Phase 4 task/check passes

### Acceptance Checks

- [x] Every valid command yields a valid event (success or error)
- [x] Execution state is queryable and correlated

## Post-Phase-4 MVP Steps

- [ ] Add Mirage↔Ghost session wiring (connect/register/ready) while preserving protocol/runtime boundaries
- [ ] Implement single-intent loop end-to-end (`issue -> command -> seed.execute -> seed.result -> event -> report`)
- [ ] Add failure-path tests (disconnect, timeout, duplicate IDs, validation failures) before MVP tag

---

# Slice Status

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

- [x] Create `internal/protocol/frame` package skeleton with tests
- [x] Create `internal/protocol/tlv` package skeleton with tests
- [x] Create `internal/protocol/semantic` package skeleton with tests
- [x] Define fixture vectors for one `issue` and one `command` frame

## Notes

- Canonical contract tables and IDs live in `../architecture/definitions/*.toml`.
- Glossary files provide copy/paste Go definitions and small implementation scaffolds.
- `design.toml` is not modified without explicit user request.
