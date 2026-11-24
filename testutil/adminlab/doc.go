// Package adminlab is a local lab for testing the admin security boundary by hand or by an external
// agent: configs for a Mirage (with its managed local Ghost) and a separate Ghost on loopback mTLS,
// a setup script, a one-request openssl helper, and a signing CLI (./sign) for proposer intents and
// approvals. It is test tooling only; nothing here is imported by the runtimes.
//
// Canonical references (consult before changes):
// - docs/architecture/definitions/admin_security.toml
//
// - docs/progress/admin_security.md
//
// - testutil/adminlab/README.md
package adminlab
