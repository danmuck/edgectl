// Package ghost owns execution concerns.
//
// Ownership boundary:
// - command routing
// - seed dispatch
// - event emission
//
// Lifecycle order:
// - appear -> seed -> radiate
// - radiate may run with an empty seeded registry.
//
// Ghost does not own desired state.
//
// Relevant docs:
// - docs/architecture/transport.md
// - docs/architecture/models/discovery.mmd
// - docs/architecture/definitions/protocol.toml
// - docs/glossary/ghost_dispatch.md
// - docs/glossary/definitions.md
package ghost
