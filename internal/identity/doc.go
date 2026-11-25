// Package identity owns certificate principals and key/revocation checks for TLS boundaries.
//
// Ownership boundary:
// - principal parsing: edgectl://<role>/<id> from exactly one URI SAN; common name ignored
//
// - roles: operator, mirage, ghost
//
// - private key file-mode enforcement (0600) before keys are loaded
//
// - denied-serial revocation checked on every handshake
//
// Canonical references (consult before changes):
// - docs/index.md
//
// - docs/architecture/definitions/admin_security.toml
//
// - docs/architecture/definitions/transport_security.toml
//
// - docs/progress/admin_security.md
package identity
