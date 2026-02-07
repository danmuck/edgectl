// Package session owns Mirage<->Ghost session transport helpers.
//
// Ownership boundary:
// - registration control messages
// - event/event.ack wire helpers
// - retry/backoff/outbox primitives
//
// Canonical references (consult before changes):
// - docs/index.md
// - docs/architecture/transport.md
// - docs/architecture/framing.md
// - docs/architecture/definitions/handshake.toml
// - docs/architecture/definitions/reliability.toml
// - docs/architecture/definitions/transport_security.toml
// - docs/architecture/definitions/protocol.toml
package session
