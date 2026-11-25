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
// Compatibility and errors (protocol.toml, tlv.toml, errors.toml):
// - envelopes carry UnknownFields, filled on decode and re-emitted on encode; known IDs are filtered out
//
// - ErrorCode classifies framing, tlv_decode, semantic, and transport errors into wire codes
//
// - decoders apply the encoders' required-field checks; a blank required field is a semantic failure (1300)
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
//
// - docs/architecture/definitions/tlv.toml
//
// - docs/architecture/definitions/errors.toml
package session
