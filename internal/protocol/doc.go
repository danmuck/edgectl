// Package protocol owns wire contract and parsing primitives.
//
// Ownership boundary:
// - frame/header primitives
//
// - tlv payload primitives
//
// - semantic validation entry points
//
// Compatibility (protocol.toml [protocol.unknown_handling], tlv.toml decoder rules):
// - unsupported version, magic, and flag bits are rejected at header decode
//
// - unknown TLV field IDs are preserved raw (schema.UnknownFields) and never mapped to semantics
//
// - session.ErrorCode maps protocol errors to errors.toml wire codes (1100/1101/1200/1300/1000/1500)
//
// Canonical references (consult before changes):
// - docs/index.md
//
// - docs/architecture/framing.md
//
// - docs/architecture/tlv.md
//
// - docs/architecture/definitions/protocol.toml
//
// - docs/architecture/definitions/tlv.toml
//
// - docs/architecture/definitions/errors.toml
//
// - docs/architecture/definitions/reliability.toml
//
// - docs/glossary/frame.md
//
// - docs/glossary/semantic_validation.md
package protocol
