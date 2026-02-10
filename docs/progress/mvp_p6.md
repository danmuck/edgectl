# MVP Progress — Phase 6 (Boundary Transport Integration)

Status: `Completed`

### Tasks

- [x] Bind Mirage command dispatch link to Ghost admin boundary using protocol command/event envelopes (`execute_envelope`)
- [x] Replace any direct action-style HTTP shortcuts between Mirage and Ghost
- [x] Wire optional auth block handling and validation hooks
- [x] Add contract tests for all boundaries

### Multi-Stage Intent Stop-Gaps (client-tm)

The following issues block full E2E multi-seed intent execution from client-tm through Mirage orchestration. The orchestrator infrastructure (`normalizeIssueToStages`, `flattenStagesToPlannedCommands`, `IssueStage`, `Orchestrator` field on `MirageIntentTemplate`) exists and is tested at the orchestration layer, but the client-tm submission path bypasses it entirely due to legacy single-command scoping.

**Known Bugs (resolved):**

1. ~~**Template filtering drops orchestrator intents**~~ — Fixed: `mirageIntentTemplatesForServices()` now matches orchestrator templates by `SeedDependencies` instead of `Command.SeedSelector`.
2. ~~**Submission path ignores Orchestrator function**~~ — Fixed: `submitMirageIssue()` branches on `template.Orchestrator != nil` and calls `submitOrchestratorIssue()` which invokes the orchestrator and submits `Stages`.
3. ~~**Single-ghost selection forced for all intents**~~ — Fixed: orchestrator path skips ghost selection; the orchestrator resolves targets from service discovery.

**Tasks:**

- [x] Fix `mirageIntentTemplatesForServices()` to include orchestrator-only templates that lack `Command.SeedSelector`
- [x] Fix `submitMirageIssue()` to call `template.Orchestrator` when present, instead of always building single-command plan
- [x] Fix ghost selection in `submitMirageIssue()` to skip single-ghost prompt when orchestrator handles targeting
- [x] Populate `Stages` field (not just `CommandPlan`) in `MirageIssueRequest` for orchestrator-produced multi-stage plans
- [x] Add E2E test for multi-seed orchestrator intent template (`store_and_index` through full mirage orchestration)
- [x] Verify `normalizeIssueToStages()` correctly processes `Stages` submitted from client-tm orchestrators

### Acceptance Checks

- [x] All boundary interactions are envelope-driven
- [x] Protocol encode/decode is used end-to-end for Mirage command dispatch to Ghost admin execute path
- [x] Multi-seed intent template (e.g. `distribute_file`) is selectable, submits multi-stage issue, reconciles to completion, and produces correct reports

### Phase 6 Closeout Notes (2026-02-09 EST)

- Envelope-only admin execution is now enforced at Ghost boundary: legacy action `execute` was removed; `execute_envelope` is the only execute path.
- `client-tm` Ghost command wizard now sends `command_frame` via `execute_envelope` and resolves execution state by `command_id`.
- Optional auth block hooks are wired on the execute boundary:
  - Ghost supports `AdminFrameAuthToken` for `command_frame` auth validation.
  - Mirage Ghost admin client supports optional command-frame auth bytes via `GhostAdminFrameAuthToken`.
- Contract tests added for boundary behavior:
  - legacy execute action rejection
  - auth-required rejection and auth-accepted execution
  - command-frame auth encode/decode and client propagation

### Boundary Flow Artifact

- Message flow diagram: `docs/progress/mvp_p6_execute_envelope_flow.mmd`

### Failure Modes / Retry / Idempotency Expectations

- Auth failure (`command_frame` missing/invalid auth when required): Ghost rejects execute request; Mirage marks command/report as failed; no mutation replay.
- Transport/session failure before terminal `event`: Mirage command remains eligible for retry per orchestration reconciliation policy.
- Duplicate replay with same `command_id`: execution lookup remains stable via `execution_by_command_id`; no direct mutation path outside envelope boundary.
