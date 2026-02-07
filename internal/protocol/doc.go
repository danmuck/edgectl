// Package protocol owns wire contract and parsing primitives.
//
// Ownership boundary:
// - frame/header primitives
//
// - tlv payload primitives
//
// - semantic validation entry points
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
