// Package observability owns the structured custody-chain log record.
//
// Ownership boundary:
// - LogEvent with observability.toml required fields and validation
//
// - trace_id/request_id derivation from wire correlation keys (no wire fields added)
//
// - pluggable Logger for runtime output and test capture
//
// Correlation (observability.toml [correlation_rules]):
// - trace_id = "trace." + intent_id, shared by every custody step of an intent on both runtimes
//
// - request_id = intent_id (issue, report), command_id (command, seed.execute, seed.result, event),
// event_id (event.ack)
//
// Canonical references (consult before changes):
// - docs/index.md
//
// - docs/architecture/definitions/observability.toml
//
// - docs/architecture/definitions/protocol.toml
//
// - docs/architecture/definitions/errors.toml
//
// - docs/glossary/observability.md
package observability
