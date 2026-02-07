// Package mirage owns orchestration concerns.
//
// Ownership boundary:
// - desired-state ingestion
// - reconciliation planning
// - report production
//
// Mirage does not execute seed operations directly.
//
// Relevant docs:
// - docs/architecture/control-loop.md
// - docs/architecture/models/state_authority.mmd
// - docs/architecture/definitions/protocol.toml
// - docs/glossary/mirage_reconcile.md
// - docs/glossary/definitions.md
package mirage
