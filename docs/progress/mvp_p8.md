# MVP Progress — Phase 8 (Hardening + Cluster Validation)

Status: `In Progress (Plan Refined)`

## Phase Goal

Harden Mirage/Ghost reliability behavior to contract level and validate the control loop on a multi-ghost cluster before MVP closeout.

## Contract References

- `docs/architecture/definitions/reliability.toml`
- `docs/architecture/definitions/observability.toml`
- `docs/architecture/definitions/protocol.toml`
- `docs/architecture/definitions/errors.toml`
- `docs/architecture/definitions/handshake.toml`

## Design Artifacts (Phase 8)

- Cluster topology diagram: `docs/progress/mvp_p8_cluster_topology.mmd`
- Reliability message-flow diagram: `docs/progress/mvp_p8_reliability_message_flow.mmd`

## Phase 9 Preconditions Delivered by Phase 8

- [ ] Reliability baseline is complete and proven on multi-ghost cluster paths.
- [ ] Observability fields/correlation are complete enough for public `api/v1` consumers.
- [ ] Dependency governance guardrails are enforced before broad seed expansion in Phase 9.
- [ ] Cluster validation evidence exists for the first frontend-safe MVP release.

## Implementation Slices

### Slice A — Idempotency and Replay Safety

- [ ] Add Ghost command idempotency ledger keyed by `command_id` with duplicate retention window `>=15m`.
- [ ] Add Ghost seed execution idempotency keyed by `execution_id` so replay returns prior terminal `seed.result`.
- [ ] Add Mirage event ingest dedupe keyed by `event_id`; repeated delivery must not re-mutate observed state.
- [ ] Return idempotent `event.ack` by `event_id` (accepted for already-accepted event).
- [ ] Add conformance tests for duplicate `command`, duplicate `seed.result`, duplicate `event`, and duplicate `event.ack`.

### Slice B — Timeout, Retry, Backoff, Terminal States

- [ ] Normalize runtime defaults to reliability contract values (`connect`, `handshake`, `read`, `write`, `ack_timeout`).
- [ ] Implement bounded exponential backoff with jitter (`initial=250ms`, `multiplier=2.0`, `max=5000ms`).
- [ ] Retry `command` only when no terminal event is observed; retries must preserve idempotency keys.
- [ ] Implement explicit terminal states for command/report lifecycle (`complete`, `failed`, `timed_out`, `delivery_failed`, `retry_exhausted`).
- [ ] Add deterministic timeout/retry tests with bounded completion and explicit assertions on terminal state.

### Slice C — Protocol Compatibility and Error Mapping Hardening

- [ ] Enforce protocol version compatibility gates at session boundary and admin boundary.
- [ ] Verify unknown TLV field preservation remains inert (never executable semantics).
- [ ] Verify unknown frame flag bits are rejected with canonical framing error behavior.
- [ ] Ensure wire error codes map to canonical taxonomy (`1000-1500`) for failure responses.
- [ ] Add compatibility tests for version mismatch and unknown-field/flag handling.

### Slice D — Structured Observability Completion

- [ ] Enforce required log fields: `component, peer, direction, trace_id, request_id, message_id, message_type, status, timestamp_ms`.
- [ ] Propagate correlation IDs across full custody chain: `issue -> command -> seed.execute -> seed.result -> event -> event.ack -> report`.
- [ ] Add deterministic structured event lines in Mirage and Ghost for timeout/retry/idempotency transitions.
- [ ] Add verification tests/assertions for required fields and correlation continuity in success and failure flows.

### Slice E — Proxmox Cluster Validation Pass

- [ ] Stand up test topology:
- [ ] `VM1`: `miragectl` control plane.
- [ ] `VM2..N`: `ghostctl` nodes with builtin seeds.
- [ ] Deploy Linux amd64 binaries and signed config for each node.
- [ ] Configure Mirage `preload_ghost_admins` with all ghost admin endpoints.
- [ ] Configure each Ghost with stable `ghost_id` and `mirage_address` for session path.
- [ ] Validate core scenarios on real network:
- [ ] Single-ghost intent success (`seed.fs` store file).
- [ ] Multi-ghost fanout success (`seed.fs` distribute file).
- [ ] Multi-stage success (`seed.fs + seed.kv` store and index).
- [ ] Host and Docker baseline intent success (`seed.host` + `seed.docker`) for Phase 9 API readiness.
- [ ] Induced duplicate/replay event delivery (assert dedupe and idempotent ack).
- [ ] Induced command timeout/retry exhaustion (assert deterministic terminal state).
- [ ] Network partition and reconnect (assert no duplicate mutation and eventual report closure).

### Slice F — Dependency Governance Baseline for Phase 9

- [ ] Define one Mirage-owned dependency catalog source file used for install-policy decisions.
- [ ] Enforce allowlisted install methods (`github`, `curl`, `brew`, `stream`) at runtime boundaries.
- [ ] Enforce transfer direction policy: streaming payloads may only flow `mirage -> ghost`.
- [ ] Enforce Mirage approval requirement before Ghost executes dependency install plans.
- [ ] Enforce project-managed install roots and disallow unmanaged host-global dependency paths.
- [ ] Emit audit records for proposal, approval, execution start, execution result, and rollback state.

## Failure Modes and Behavioral Expectations

- Duplicate `command_id` replay: no second mutation; Ghost returns stable execution outcome.
- Duplicate `event_id` replay: Mirage observed state is unchanged; returns idempotent `event.ack`.
- `event.ack` timeout: Ghost outbox retries with bounded backoff until accepted or `ack_timeout` exceeded.
- `event.ack` reject: mark delivery failed; no implicit mutation replay.
- No terminal event for command: Mirage may retry according to policy; terminal state must be explicit and bounded.
- Session disconnect during in-flight intent: reconnect resumes without replaying already-accepted events.

## Acceptance Checks

- [ ] Duplicate/replay inputs do not produce duplicate seed mutations.
- [ ] Retry/backoff behavior is deterministic, bounded, and observable.
- [ ] Terminal failure states are explicit in report output and logs.
- [ ] Structured logs include required observability fields and correlation IDs.
- [ ] Protocol compatibility checks enforce unknown flag rejection and safe unknown TLV handling.
- [ ] Proxmox cluster test matrix passes for success and failure scenarios.
- [ ] Dependency governance controls are active and tested (catalog, approval, transfer direction, install roots).
- [ ] `go test ./...` passes after each hardening slice.

## MVP Exit Dependency

- [ ] Phase 8 acceptance checks completed and linked to concrete verification evidence.
- [ ] Phase 9 finalization scope can be defined from validated Phase 8 outcomes.
