// Package host owns the host introspection seed adapter.
//
// Ownership boundary:
// - host identity and network introspection
//
// - operations: status (hostname, ip, os), ports (listening tcp/udp), interfaces (network interfaces)
//
// Uses Go stdlib where possible with CLI fallback for platform-specific operations.
//
// Canonical references (consult before changes):
// - docs/architecture/definitions/design.toml seed_examples["node.host"]
//
// - docs/architecture/control-loop.md
//
// - docs/glossary/definitions.md
package host
