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
