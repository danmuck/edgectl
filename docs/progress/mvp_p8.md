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

- [x] Add Ghost command idempotency ledger keyed by `command_id` with duplicate retention window `>=15m`.
- [x] Add Ghost seed execution idempotency keyed by `execution_id` so replay returns prior terminal `seed.result`.
- [x] Add Mirage event ingest dedupe keyed by `event_id`; repeated delivery must not re-mutate observed state.
- [x] Return idempotent `event.ack` by `event_id` (accepted for already-accepted event).
- [x] Add conformance tests for duplicate `command`, duplicate `seed.result`, duplicate `event`, and duplicate `event.ack`.

Slice A implementation notes (2026-11-22):

- Ghost `HandleCommandAndExecute` replays the stored terminal `event` for a completed `command_id`; the seed is not re-executed and the stored `seed.result` (keyed by derived `execution_id`) is unchanged. A retry may carry a new `message_id`; replay does not index it.
- An in-flight duplicate (accepted, no terminal event yet) still returns `ErrDuplicateCommandID`; there is no terminal state to replay.
- Completed Ghost records and Mirage acks are retained for `DuplicateWindow = 15m` and pruned after; in-flight records never expire.
- Mirage `Server.IngestEvent` checks `event_id` before observation. A redelivered event returns the stored ack and does not touch observed state, report history, ghost event counters, or runtime buildlogs.
- `Orchestrator.IngestObservedEvent` now returns `duplicate`; `Server.ObserveEvent` no longer appends a second report when a command was already observed (e.g. reconcile path followed by session delivery).
- After ack expiry a redelivered event gets a fresh ack, but observed state still dedupes by `command_id`.
- Tests: `internal/ghost/idempotency_test.go`, `internal/mirage/idempotency_test.go`.
- Open question: replay does not compare payloads; a reused `command_id` with different seed/operation/args returns the original event. Contract is silent on conflict handling.

Slice A security follow-up — event identity binding (2026-11-22):

- Finding: Mirage observed session events without checking them against the session's bound `ghost_id`. Any authenticated ghost could report a result for another ghost's `command_id`; Slice A dedupe then discarded the real result as a duplicate. Gap pre-dated Slice A.
- Contract: `transport_security.toml` `ghost_id_binding = "required before command/event flow"`, `binding_mismatch = "reject session"`; `errors.toml` `declared_peer_mismatch = "1000"`.
- Fix: `Server.IngestEvent` rejects, before dedupe or observation, any event whose declared `ghost_id` differs from the session, or whose `command_id` is planned for a different ghost. Rejected `event.ack` carries `ack_code=1000`; nothing is observed or stored; `handleConn` closes the session after writing the ack.
- Tests: `internal/mirage/binding_test.go` (declared mismatch, planned-ghost mismatch with real result still accepted afterwards, unplanned command from bound ghost accepted, session closed end to end).
- Not covered: the direct reconcile path (`ReconcileOnce` -> executor) trusts the event returned by the executor registered for that `ghost_id`; it is not a session boundary.
- Hardening (second review): the planned-ghost check now runs inside `Orchestrator.IngestObservedEvent` under the same lock and against the same plan entry used for observation (was a separate lookup: check/use race). `SubmitIssue` rejects an intent whose generated `command_id`s collide with another intent's (sanitized ids such as `x/y` and `x.y` produced identical `command_id`s, making lookup ambiguous).

### Slice B — Timeout, Retry, Backoff, Terminal States

- [x] Normalize runtime defaults to reliability contract values (`connect`, `handshake`, `read`, `write`, `ack_timeout`).
- [x] Implement bounded exponential backoff with jitter (`initial=250ms`, `multiplier=2.0`, `max=5000ms`).
- [x] Retry `command` only when no terminal event is observed; retries must preserve idempotency keys.
- [x] Implement explicit terminal states for command/report lifecycle (`complete`, `failed`, `timed_out`, `delivery_failed`, `retry_exhausted`).
- [x] Add deterministic timeout/retry tests with bounded completion and explicit assertions on terminal state.

Slice B implementation notes (2026-11-22):

- Decisions (user, 2026-11-22): terminal states are per-command in Mirage observed state, not wire values; report `completion_state` stays `in_progress|satisfied|failed` and the summary names the specific state. Command dispatch defaults to 3 attempts, configurable via miragectl `command_max_attempts` (contract is silent on a bound).
- Defaults: `session.DefaultConfig` already matched `reliability.toml`; both runtimes and shipped TOML use it. Existing `TestDefaultConfigMatchesReliabilityContract` pins the values. No change needed.
- Backoff: jitter was applied after the max cap (delays up to 7.5s) and only from attempt 2; jitter is now applied on every attempt and the result is always capped at `max_delay_ms`. A nil rng uses the shared source instead of a fixed 0.5 factor. Ghost reconnect previously forced `Jitter=false`, contrary to `jitter = "required"`; it now uses contract backoff.
- Command retry: `Orchestrator.dispatchWithRetry` reuses the same `command_id` on every attempt (Ghost replays instead of re-executing, Slice A), sleeps with contract backoff between attempts, and stops early if a terminal event for the command arrives via the session path.
- Mirage per-command states (`CommandStatus` in `ObservedIntent.CommandStates`): `complete` / `failed` from the terminal event outcome; `retry_exhausted` after `MaxAttempts` failures; `timed_out` when the caller deadline expires first. Terminal commands are settled and never re-dispatched. A real terminal event arriving later supersedes an inferred `timed_out`/`retry_exhausted` (attempt count kept).
- Ghost event delivery: outbox entries carry `DeliveryState`; an `event.ack` rejection or `ack_timeout_ms` expiry marks `delivery_failed` and retains the record (previously a rejection deleted it and a timeout left it looking pending). Command results currently return synchronously over the admin path, so only session heartbeat events use this path today.
- Reconcile requests have a 5s admin deadline and the ghost admin client uses a 5s per-call timeout, so a hung ghost yields `timed_out` after one attempt; fast transport failures get all 3 attempts (~250ms + ~500ms backoff).
- Tests: `internal/mirage/command_retry_test.go` (8 tests, injected sleeper, no real waiting), backoff bound tests in `internal/protocol/session/session_test.go`, outbox state test, `delivery_failed` assertions in Ghost ack-timeout and Mirage binding-rejection tests, miragectl `command_max_attempts` config test.

### Slice C — Protocol Compatibility and Error Mapping Hardening

- [x] Enforce protocol version compatibility gates at session boundary and admin boundary.
- [x] Verify unknown TLV field preservation remains inert (never executable semantics).
- [x] Verify unknown frame flag bits are rejected with canonical framing error behavior.
- [x] Ensure wire error codes map to canonical taxonomy (`1000-1500`) for failure responses.
- [x] Add compatibility tests for version mismatch and unknown-field/flag handling.

Slice C implementation notes (2026-11-23):

- Version/flag gate: `frame.DecodeHeader` already rejected unsupported magic, version, and flag bits. Mirage's session loop previously closed silently; it now logs `component, peer, direction, message_id, message_type, error_code` (framing.md / errors.toml) and closes. A clean EOF at a frame boundary is not logged as a framing error. Ghost's admin `execute_envelope` returns `error_code=1100` for these frames and does not execute.
- Unknown TLV fields: previously ignored but discarded, contrary to `protocol.toml` "decode and preserve in raw form" and `tlv.toml` "preserve unknown field_id values for re-encode". `Command`, `Event`, `EventAck`, and `Report` now carry `UnknownFields`, filled on decode via `schema.UnknownFields` and re-emitted on encode. Known IDs are filtered out of `UnknownFields` on encode, so the list cannot inject or shadow semantic fields. Ghost builds executions from known fields only; a test proves an unknown field does not change seed/operation.
- Error taxonomy: `session.ErrorCode` maps framing (1100, 1101 oversize), TLV decode (1200, including invalid typed values and malformed `args` JSON via `tlv.ErrInvalidFieldValue`), schema/registration validation (1300), transport (1000), else 1500. Ghost maps its own command-boundary errors (1300 semantic, 1400 runtime, 1000 frame auth). Both admin boundaries now return `error_code` on every failure; Mirage passes Ghost codes through via `GhostAdminError`. Registration and event.ack codes were already canonical and are unchanged.
- Admin "version gate": the admin JSON envelopes and the `seed.register` JSON handshake carry no protocol version in any contract (`handshake.toml` requires only `ghost_id`, `seed_list`). The gate applies to binary frames crossing the admin boundary (`execute_envelope`). Adding a version field to JSON envelopes would be a contract change; left as an open question.
- Not wired: the `error` envelope (message type 7). `errors.toml` "emit error envelope when safe" remains open; failures surface via acks, admin `error_code`, and logs.
- Tests: `internal/protocol/session/compat_test.go`, `internal/ghost/compat_test.go`, `internal/mirage/compat_test.go`.

### Slice D — Structured Observability Completion

- [x] Enforce required log fields: `component, peer, direction, trace_id, request_id, message_id, message_type, status, timestamp_ms`.
- [x] Propagate correlation IDs across full custody chain: `issue -> command -> seed.execute -> seed.result -> event -> event.ack -> report`.
- [x] Add deterministic structured event lines in Mirage and Ghost for timeout/retry/idempotency transitions.
- [x] Add verification tests/assertions for required fields and correlation continuity in success and failure flows.

Slice D implementation notes (2026-11-23):

- Decisions (user, 2026-11-23): `trace_id`/`request_id` are derived from existing wire correlation keys, no new TLV fields: `trace_id = "trace." + intent_id`; `request_id` = `intent_id` (issue, report), `command_id` (command, seed.execute, seed.result, event), `event_id` (event.ack), per `observability.toml [correlation_rules]`. The structured format applies to custody-chain lines and retry/timeout/idempotency/delivery transitions; debug and lifecycle logs stay free-form.
- New package `internal/observability`: `LogEvent` (glossary shape plus `ack_status`), `Validate`/`MissingFields`, deterministic `Line()` (required fields first in contract order), pluggable `Logger`, and `Emit`, which derives trace/request IDs, stamps `timestamp_ms`, and writes incomplete events with an error naming the missing fields rather than dropping them. Lives outside `internal/logging` to avoid a test import cycle (`testlog` -> `logging`).
- Mirage lines: issue in (accepted); command out (dispatched, retry, retry_stopped, timed_out, retry_exhausted); event in (outcome, duplicate, rejected, error); event.ack out; report out (status = completion_state). The previous free-form `ownership.transition`, `mirage.command.retry`, and `mirage.command.terminal` lines are replaced.
- Ghost lines: command in (accepted, replayed, rejected); seed.execute out and seed.result in (peer = seed id, execution_id set); event out (outcome, replayed, delivery_failed); event.ack in. `CommandEnv.Peer` carries the admin client address (or `local`) for logging only; it is not a wire field.
- `message_id` is the protocol frame id for that hop where one exists (Mirage orchestration frames, session event/ack frames) and the Ghost-assigned command message id for Ghost-internal seed steps.
- Session heartbeat probes and unplanned events are not logged per delivery on either side; their `delivery_failed` is still recorded.
- Ordering fix found by tests: on the session path the report was logged before the inbound event that produced it; the orchestrator now logs the matched event, then its report, using the session frame `message_id`.
- Tests: `internal/observability/event_test.go`, `internal/mirage/custody_test.go` (success chain across both runtimes, retry-exhausted failure flow, session event/ack with duplicate), `internal/ghost/custody_test.go` (replay, rejection). Helper: `internal/testutil/custodytest`.
- Needs approval to update (canonical/read-only): `observability.toml [implementation_status]` still says the phase8 target is pending; `docs/glossary/observability.md` is marked `stub` and its planned `LogEvent` lacks `ack_status`.

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

- [x] Define one Mirage-owned dependency catalog source file used for install-policy decisions.
- [x] Enforce allowlisted install methods (`github`, `curl`, `brew`, `stream`) at runtime boundaries.
- [x] Enforce transfer direction policy: streaming payloads may only flow `mirage -> ghost`.
- [x] Enforce Mirage approval requirement before Ghost executes dependency install plans.
- [x] Enforce project-managed install roots and disallow unmanaged host-global dependency paths.
- [x] Emit audit records for proposal, approval, execution start, execution result, and rollback state.

Slice F implementation notes (2026-11-23):

- Decisions (user, 2026-11-23): remove Ghost boot-time installs (Mirage-only); enforce the allowlist, add `curl` with required sha256, enforce `stream` direction in policy but defer stream byte transfer to Phase 9; `brew` is the single allowed host-global exception (binary exposed only via a managed `local/bin` symlink); approval workflow as Mirage admin actions now, wrapped by Phase 9 HTTP later.
- Catalog: `internal/depcatalog/catalog.go` is the single source (initially `internal/mirage/dependency_catalog.go`; moved during hardening so Ghost can verify plan steps). It replaced per-seed `Deps()` functions, `seeds.DepSpec`, and `internal/seeds/catalog`. Entries: `seed.docker` -> brew `docker` (CLI only; the Docker daemon must be provisioned out of band), `seed.mongod` -> brew `mongosh`. `mongod` itself has no allowlisted verified source and is reported as unresolved on proposals instead of guessed. Previous entries used `apt`, which is not allowlisted.
- Policy: `seeds.ValidateSpec` enforces the method allowlist, pinned sources (github requires `ref`; curl requires https, `version`, 64-hex `sha256`, destination), `stream` direction `mirage->ghost`, and relative destinations that cannot escape the install root. Checked by Mirage at proposal time and by Ghost before executing any step. `apt` and `workspace_copy` were removed from the installer.
- Approval: Mirage admin actions `propose_dependencies`, `approve_dependencies`, `execute_dependencies`, `list_dependency_proposals` over an in-memory ledger (`pending -> approved -> executing -> succeeded|failed`). Ghost `install_dependencies` requires a proposal id and approver, checks the admin auth token when configured, policy-checks the whole plan first, and rolls back this plan's managed-root installs on failure. Ghost no longer installs at boot; `[[seed_install]]` in Ghost config or Mirage ghost manifests is now a load error, and SSH deploy strips any `seed_install` a template carries.
- Audit: each proposal carries ordered audit records (`proposal`, `approval`, `execution_start`, `execution_result`, `rollback` with `rolled_back n/m` or `partial`), readable via `list_dependency_proposals`, plus a `mirage.dependency.audit` log line per record.
- Security hardening (review of uncommitted Slice F): the first version let any admin client submit a plan when no admin token was configured, and plans could carry `bootstrap_command`, arbitrary taps/sources, and `-`-prefixed `ref`/`package` values (option injection). Fixes: bootstrap fields are never decoded from JSON; Ghost refuses all plans unless `admin_frame_auth_token` is configured and presented; every step must exactly match an entry of the shared catalog (moved to `internal/depcatalog`, still Mirage-owned; Ghost only verifies membership); git/brew argument values must match a safe charset and cannot start with `-`. Config keys added so the token is actually settable: ghostctl `admin_frame_auth_token`, miragectl `ghost_admin_frame_auth_token` (also applied to the managed local Ghost); SSH deploy writes the token into the remote Ghost config.
- Limits: Ghost still cannot cryptographically verify that Mirage approved a specific plan; the admin token plus catalog-only steps bound what any token holder can install. The ledger is in memory (lost on Mirage restart). `brew` installs are not rolled back. Plans cannot bootstrap brew; it must already be installed on the host.
- Tests: `internal/seeds/policy_test.go`, `internal/ghost/dependencies_test.go`, `internal/mirage/dependency_test.go`, updated config/deploy tests.

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
