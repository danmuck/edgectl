// Package session owns Mirage<->Ghost session transport helpers.
//
// Ownership boundary:
// - registration control messages
//
// - event/event.ack wire helpers
//
// - retry/backoff/outbox primitives
//
// Reliability (reliability.toml [backoff], [acknowledgement]):
// - backoff is exponential with jitter on every attempt, always capped at max_delay_ms
//
// - outbox entries are pending until accepted; rejected or timed-out acks mark delivery_failed
//
// Canonical references (consult before changes):
// - docs/index.md
//
// - docs/architecture/transport.md
//
// - docs/architecture/framing.md
//
// - docs/architecture/definitions/handshake.toml
//
// - docs/architecture/definitions/reliability.toml
//
// - docs/architecture/definitions/transport_security.toml
//
// - docs/architecture/definitions/protocol.toml
package session
