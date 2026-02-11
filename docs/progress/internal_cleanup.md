# Internal Cleanup Tracker

**Status:** `In Progress`
**Scope:** `internal_cleanup`

## 2026-02-09 Pass

- [x] Consolidate repeated "recent N events" slice-copy logic in `internal/ghost/admin_control.go`.
- [x] Remove duplicate stage/command validation in Mirage submit path by making `IssueEnv.Validate()` the single validation gate in `internal/mirage/orchestration.go`.
- [x] Simplify Mirage admin issue mapping via a dedicated command mapper helper in `internal/mirage/admin_control.go`.
- [x] Deduplicate TLV frame encoding flow in `internal/protocol/session` with one shared helper (`wire_encode.go`).
- [x] Validate compatibility with protocol contracts in `docs/architecture/definitions/protocol.toml` and `docs/architecture/definitions/tlv.toml`.
- [x] Run full repository tests (`go test ./...`).
- [x] Align new test debug output to repository logging convention (`smplog`) for protocol/session/frame and orchestration test additions.
- [x] Add demonstration-focused high-level test module at `testutil/demo` for protocol message flow, orchestrator control flow, and live Mirage↔Ghost e2e flow; use `logs.Debug` for demo output.
- [x] Update `cmd/testctl` interactive UX so `q` is universal back/exit control, remove explicit Back/Exit menu options, and default pacing to paused mode.
- [x] Add visible progress bars in `cmd/testctl` during background pre-run discovery (test collection/inventory build) so users can track setup progress before execution.
- [x] Restore log-level coloring in `cmd/testctl` output rendering for rewritten lines (`DEBUG` green, `DEV` magenta) so display matches expected smplog semantics.
- [x] Fix progress-bar redraw behavior in `cmd/testctl` to fully clear trailing characters when successive status text shrinks.
- [x] Expand `cmd/testctl` protocol-output readability by mapping `message_type` ids to descriptive envelope labels and add unit tests for the formatter.

## 2026-02-10 Pass

- [x] Trace seed capability ownership path (`internal/seeds` -> `internal/ghost` -> `internal/mirage` -> `cmd/client-tm`) and document command-template catalog coupling for refactor planning.
- [x] Add initial implementation tracker doc `docs/progress/seed-coupling.md` with architecture/message-flow artifacts, staged checklist, and acceptance gates.
- [x] Complete Seed Coupling Stage 1 contract pass: add seed-owned command catalog types, extend `seeds.Seed` interface, and publish command catalogs for all built-in seeds.
- [x] Complete Seed Coupling Stage 2 Ghost publication pass: expose full seed capability catalog (`metadata + operations + command_catalog`) via Ghost server/admin, preserving legacy `list_seeds`.
- [x] Complete Seed Coupling Stage 3 Mirage aggregation pass: aggregate Ghost `seed_catalog` snapshots in Mirage, expose via Mirage admin action, and add mixed-connectivity/partial-availability tests.
- [x] Complete Seed Coupling Stage 4 client migration pass: drive Ghost command wizard and operation listing from remote `list_seed_catalog` payloads, remove static client catalog/inference helpers, and keep the existing `CommandTemplate` UI shape.
- [x] Set `cmd/testctl` run-mode default pacing to `free` so `make test-override` executes the full suite non-interactively while preserving paused pacing in interactive mode.
- [x] Make Ghost Mirage connect/register retries switch to resolved `bind_mirage` session addresses in-flight using a concurrency-safe client dial target update, and cover with client/service reroute tests.

## Guardrails

- No changes to canonical architecture contracts or models.
- No wire field IDs, message types, or required-field semantics changed.
- Refactors are structural/readability-only.
