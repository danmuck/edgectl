// Package mirage owns orchestration concerns.
//
// Ownership boundary:
// - desired-state ingestion
// - reconciliation planning
// - report production
//
// Mirage does not execute seed operations directly.
//
// Canonical references (consult before changes):
// - docs/index.md
// - docs/architecture/control-loop.md
// - docs/architecture/models/state_authority.mmd
// - docs/architecture/models/control_loop_locality.mmd
// - docs/architecture/definitions/protocol.toml
// - docs/architecture/definitions/reliability.toml
// - docs/architecture/definitions/observability.toml
// - docs/glossary/mirage_reconcile.md
// - docs/glossary/definitions.md
package mirage
