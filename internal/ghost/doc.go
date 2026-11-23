// Package ghost owns execution concerns.
//
// Ownership boundary:
// - command routing
//
// - seed dispatch
//
// - event emission
//
// Lifecycle order:
// - appear -> seed -> radiate
//
// - radiate may run with an empty seeded registry.
//
// - standalone runtime does not require Mirage to be connected.
//
// Builtin seed catalog:
// - seed.flow (control plane)
// - seed.mongod (systemctl mongod adapter)
// - seed.kv (in-memory key-value store)
// - seed.fs (ghost-scoped filesystem)
// - seed.docker (container runtime CLI adapter)
// - seed.host (host introspection via Go stdlib + CLI)
//
// Idempotency (reliability.toml [idempotency]):
// - a completed command_id replays its prior terminal event; the seed is not re-executed
//
// - execution_id is derived from command_id, so seed.result is never produced twice per key
//
// - completed records are retained for DuplicateWindow (15m) before pruning
//
// Event delivery (reliability.toml [acknowledgement], [backoff]):
// - events retry until an accepted event.ack or ack_timeout_ms; rejection or timeout marks delivery_failed
//
// - Mirage reconnect uses contract backoff with required jitter
//
// Dependency installs (docs/progress/mvp_p8.md Slice F):
// - Ghost never installs on its own (no boot-time installs; seed_install config specs rejected)
//
// - install_dependencies executes only Mirage-approved plans: requires the admin auth token (refused
// when none is configured), every step must pass policy and exactly match internal/depcatalog
// before any step runs, and a failed step rolls back this plan's managed-root installs
//
// - brew bootstrap commands are host-local only; plans cannot carry them
//
// - dependency_status reports which required binaries resolve
//
// Custody logging (observability.toml):
// - command in (accepted|replayed|rejected), seed.execute, seed.result, event out, event.ack in
//
// - CommandEnv.Peer is boundary metadata for logging only; heartbeat probes skip custody lines
//
// Admin boundary errors (errors.toml):
// - failed admin responses carry error_code; execute_envelope frames are version/flag gated (1100)
//
// - unknown TLV fields in command frames are ignored for execution
//
// Ghost does not own desired state.
//
// Canonical references (consult before changes):
//
// - docs/index.md
//
// - docs/architecture/transport.md
//
// - docs/architecture/control-loop.md
//
// - docs/architecture/models/discovery.mmd
//
// - docs/architecture/models/proto_interface_boundary.mmd
//
// - docs/architecture/models/single_intent.mmd
//
// - docs/architecture/definitions/protocol.toml
//
// - docs/architecture/definitions/reliability.toml
//
// - docs/architecture/definitions/observability.toml
//
// - docs/architecture/definitions/errors.toml
//
// - docs/glossary/ghost_dispatch.md
//
// - docs/glossary/definitions.md
package ghost
