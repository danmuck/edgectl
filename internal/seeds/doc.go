// Package seeds owns seed service interfaces exposed by Ghost.
//
// Ownership boundary:
// - seed metadata shape
//
// - seed execution interface
//
// - local seed registry primitives
//
// - whitelist-gated seed dependency installation primitives
//
// Canonical references (consult before changes):
// - docs/index.md
//
// - docs/architecture/control-loop.md
//
// - docs/architecture/models/proto_interface_boundary.mmd
//
// - docs/architecture/definitions/protocol.toml
//
// - docs/architecture/definitions/tlv.toml
//
// - docs/architecture/definitions/reliability.toml
//
// - docs/glossary/shapes.md
//
// - docs/glossary/definitions.md
package seeds
