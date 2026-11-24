// Package mirage owns orchestration concerns.
//
// Ownership boundary:
// - lifecycle control (appear -> shimmer -> seed)
//
// - desired-state ingestion
//
// - reconciliation planning
//
// - report production
//
// Session TLS identity (admin_security.toml [identity], [trust]):
// - session client certificates must carry an edgectl://ghost/<ghost_id> URI SAN principal (CN ignored)
//
// - keys load only with mode 0600; denied serials are rejected on every handshake
//
// Admin transport (admin_security.toml [transport]):
// - admin endpoint is mTLS as edgectl://mirage/<mirage_id>; plaintext only on loopback outside production
//
// - privileged actions (deploy_ghost, approve/execute_dependencies) require an mTLS peer
//
// - Ghost admin clients dial as Mirage and pin edgectl://ghost/<ghost_id>
//
// Admin authorization (admin_security.toml [authorization.mirage_admin]):
// - default deny; operators only; actor fields bound to the caller principal
//
// - every request writes an admin.audit line (principal, role, action, decision, error_code)
//
// Identity binding (transport_security.toml [identity_binding], errors.toml):
// - session events must match the bound ghost_id, both declared and as planned for the command_id
//
// - mismatches get a rejected event.ack (code 1000), are not observed, and close the session
//
// Dependency governance (docs/progress/mvp_p8.md Slice F, mvp_p9_dependency_approval_flow.mmd):
// - internal/depcatalog is the single Mirage-owned source of install paths and methods
//
// - propose -> approve -> execute_dependencies; Ghost dispatch only after approval
//
// - audit records for proposal, approval, execution start, execution result, rollback
//
// Custody logging (observability.toml):
// - issue in, command out (dispatched|retry|retry_stopped|timed_out|retry_exhausted), event in
// (outcome|duplicate|rejected), event.ack out, report out; all on trace "trace."+intent_id
//
// - matched session events are logged by the orchestrator before their report (causal order)
//
// Protocol compatibility and errors (framing.md, errors.toml):
// - session frames failing version/magic/flag/TLV/schema checks are logged with error_code and close the session
//
// - failed admin responses carry error_code; Ghost admin codes pass through (GhostAdminError)
//
// Command retry and lifecycle (reliability.toml retry_policy.command, [backoff]):
// - dispatch retries reuse command_id, use contract backoff, and stop once a terminal event is observed
//
// - per-command states: complete | failed | timed_out | retry_exhausted (observed state, not wire values)
//
// - attempts default to DefaultCommandMaxAttempts (3); terminal commands are never re-dispatched
//
// Idempotency (reliability.toml [idempotency], control-loop.md):
// - session event ingest dedupes by event_id before observation (Server.IngestEvent)
//
// - event.ack is idempotent by event_id and retained for DuplicateWindow (15m)
//
// - observed state dedupes by command_id; duplicates never append reports
//
// Mirage does not execute seed operations directly.
//
// Canonical references (consult before changes):
// - docs/index.md
//
// - docs/architecture/control-loop.md
//
// - docs/architecture/models/state_authority.mmd
//
// - docs/architecture/models/control_loop_locality.mmd
//
// - docs/architecture/definitions/protocol.toml
//
// - docs/architecture/definitions/transport_security.toml
//
// - docs/architecture/definitions/admin_security.toml
//
// - docs/architecture/definitions/errors.toml
//
// - docs/architecture/definitions/reliability.toml
//
// - docs/architecture/definitions/observability.toml
//
// - docs/glossary/mirage_reconcile.md
//
// - docs/glossary/definitions.md
package mirage
