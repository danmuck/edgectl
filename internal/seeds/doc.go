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
// Install policy (docs/progress/mvp_p8.md Slice F; ValidateSpec, enforced on Mirage and Ghost):
// - methods allowlisted: github (pinned ref), curl (https + version + sha256), brew, stream
//
// - stream transfers flow mirage->ghost only; stream byte transfer is deferred to Phase 9
//
// - installs land under project-managed roots (local/); brew is the single host-global exception,
// exposed to Ghost only via a symlink in the managed bin root
//
// - Uninstall rolls back github/curl installs under the managed roots
//
// - git/brew argument values must match a safe charset and cannot start with '-'; bootstrap
// command fields are never decoded from JSON
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
