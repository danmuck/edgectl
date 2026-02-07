# Phase break: Client Basic Implementation

**Status:** `In Progress`  
**Scope:** `pbreak_client_terminal`  
**Prior phase:** `p4 complete`

### Tasks

- [ ] Define `cmd/client-tm` scope and module boundaries (UI shell, command router, transport client, formatting layer).
- [ ] Define contracts-first interfaces for control actions: `start/stop/status/health/version/config`, RPC command execution, and monitoring streams.
- [ ] Define identity/addressing model in client state: `ghost_id`, `seed_id`, `mirage_id`, endpoint, and active target context.
- [x] Implement interactive TUI shell in `cmd/client-tm` with navigation for:
  - [x] Target selection (`Ghost Admin Console`, `Seed Operations`)
  - [x] Command execution views
  - [x] Monitoring views
  - [ ] `Mirage Control` (placeholder only; not wired)
- [ ] Integrate `smplog` for formatted output (operator-friendly tables, status panels, structured event lines).
- [x] Implement Ghost admin console workflows (connect, inspect, run control commands, monitor server/seed activity).
- [x] Implement seed workflows via Ghost (issue commands to seeds, stream status/events, verify responses).
- [x] Implement protocol/message verification view (request/response IDs, component, peer, trace/request IDs, result/error).
- [x] Add support for managing many Ghost targets in one session (switching context safely and explicitly).
- [x] Add single-Mirage control path placeholder + abstraction for future multi-Mirage support (no hardcoded singleton assumptions).
- [ ] Define failure behavior for CLI operations: timeouts, retries/backoff, idempotent command handling, reconnect/resume.
- [ ] Add architecture + message-flow diagrams for this phase (CLI-to-Ghost now, CLI-to-Mirage extension path later).

### Acceptance Checks

- [x] `cmd/client-tm` builds and launches interactive mode with stable navigation and no panic on normal input paths.
- [ ] Operator can connect to at least one Ghost and run `start/stop/status/health/version/config` successfully.
- [x] Operator can switch between multiple Ghost targets without context confusion; active target is always visibly shown.
- [x] Operator can issue seed commands through Ghost and observe deterministic success/error outputs.
- [ ] Monitoring screen shows structured live events with required observability fields: `component`, `message`, `peer`, `request_id/trace_id`.
- [ ] Protocol verification output allows matching request->response and clearly surfaces timeout/retry/error cases.
- [ ] `smplog` formatting is applied consistently across command output, monitoring, and error paths.
- [x] Single-Mirage path is represented in routing/config and works as a placeholder without blocking Ghost workflows.
- [ ] Client architecture explicitly supports future multi-Mirage (typed target model and non-singleton interfaces).
- [ ] Failure semantics are explicit and testable: timeout defaults, retry policy, backoff behavior, and idempotency expectations.
- [ ] Diagrams are committed and current:
  - [ ] System architecture diagram for client/ghost/mirage topology
  - [ ] Message-flow diagrams for key RPCs used in this phase
- [x] Basic verification run demonstrates end-to-end protocol/message correctness for Ghost + seed control flows.
