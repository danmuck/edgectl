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
// - docs/glossary/ghost_dispatch.md
//
// - docs/glossary/definitions.md
package ghost
