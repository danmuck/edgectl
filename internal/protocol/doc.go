// Package protocol owns wire contract and parsing primitives.
//
// Ownership boundary:
// - frame/header primitives
// - tlv payload primitives
// - semantic validation entry points
//
// Relevant docs:
// - docs/architecture/framing.md
// - docs/architecture/definitions/protocol.toml
// - docs/architecture/definitions/tlv.toml
// - docs/glossary/frame.md
// - docs/glossary/tlv.md
// - docs/glossary/semantic_validation.md
package protocol
