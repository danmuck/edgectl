# Docs Cleanup Progress

Status: `In Progress`

- [x] [P1 | Contract Parseability] Convert canonical definition files to valid TOML syntax
- [x] [P1 | Compatibility Semantics] Resolve unknown-field/unknown-flag behavior mismatch (drop vs preserve)
- [x] [P1 | Reliability Protocol Gap] Define event acknowledgment mechanism or revise retry-until-ack contract
- [x] [P2 | Security Policy Consistency] Align production mTLS requirement language (`MUST` vs `SHOULD`)
- [x] [P2 | Doc Path Correctness] Fix canonical definitions path reference in progress tracker
- [x] [P2 | Authority Boundary Clarity] Reconcile control-loop example with seed ownership model
- [x] [P3 | Naming Consistency] Align diagram envelope names with canonical vocabulary (`seed.execute`, etc.)
- [x] [P3 | Progress Doc Hygiene] Remove/merge duplicate Phase 3 checklist blocks for unambiguous status tracking
- [x] [P4 | Buildlog Discipline] Require TOML build logs in `docs/progress/buildlog` with EST naming and follow-up prompts appended to the initial prompt log
- [x] [P4 | Final Pass] Verify all docs can be assumed canonical, update [`../../AGENTS.md`](../../AGENTS.md) to require explicit approval for canonical doc changes, and require canonical references in package `doc.go` stubs.
- [ ] [P4 | Final Verify] Verify all current code aligns with the documentation.  
  Current status: see [`p4_conformance_report.md`](p4_conformance_report.md).

## P1 Completion Notes (2026-02-07)

- [x] Canonical contract files under `docs/architecture/definitions/*.toml` were normalized to valid TOML while preserving existing diagram/comment content.
- [x] Unknown handling policy was unified: unknown TLV fields are preserved as inert raw data and ignored semantically; unsupported flag bits are rejected.
- [x] Reliability delivery closure was defined via `event.ack` with required fields, retry ownership, and idempotency keys (`event_id`).

## P2 Completion Notes (2026-02-07)

- [x] Security language aligned in transport contract: production now requires TLS + mTLS and strict pre-flow identity binding.
- [x] Progress note path fixed to `architecture/definitions/*.toml`.
- [x] Control-loop authority clarified: Mirage is orchestration-only; local critical seeds are hosted by a co-located local Ghost with shared network identity/locality metadata.

## P3 Completion Notes (2026-02-07)

- [x] Model diagrams now use canonical envelope naming (`issue`, `command`, `seed.execute`, `seed.result`, `event`, `event.ack`, `report`).
- [x] Duplicate Phase 3 checklist content was removed from progress tracking to keep status unambiguous.

## P4 Progress Notes (2026-02-07)

- [x] Repository-level governance file added at `AGENTS.md` with canonical-source and change-control requirements.
- [x] Package `doc.go` stubs updated to reference canonical contracts before implementation changes.
- [x] Buildlog policy, naming scheme, and TOML template added under `docs/progress/buildlog/` with scope-based log rollover and follow-up append rules.
- [ ] `2026-02-07` verification + implementation passes completed; `go test ./...` passes, P1 is closed, and P2 runtime now includes session primitives, minimal Mirage endpoint, Ghost client, config/policy wiring, reconnect baseline tests, and step-3 resilience scenarios (ack-timeout + replay-across-reconnect coverage). Remaining gap: TLS/mTLS enforcement.  
  Source: `p4_conformance_report.md`.
