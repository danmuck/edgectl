# Mirage Control Loop

Example deployment used in this flow:

- Mirage remains orchestration-only and does not host seeds directly.
- Mirage may pair with one co-located local Ghost that shares network identity and locality context.
- Any Ghost can host any seed type; locality is a scheduling preference, not a placement restriction.
- The local Ghost can host critical/locality-sensitive seeds (for example `raft-node`) and local services (for example `mongodb`).
- External Ghosts can simultaneously host the same seed types with their own network URIs.
- In a distributed `raft-node` deployment, multiple Ghosts may each expose their own `raft-node` entrypoint.
- In a distributed `kdht-node` deployment, multiple Ghosts may expose `kdht-node` entrypoints for routing, key lookup, and replication.
- Mirage may schedule storage movement across the network between local and remote database seed URIs.
- Locality-aware control-loop diagram: [`models/control_loop_locality.mmd`](models/control_loop_locality.mmd)

## Phase 5 Boundary and Message Flow

- Mirage runtime is split into transport/session handling (`Service`) and orchestration/lifecycle authority (`Server` + orchestrator loop).
- Command dispatch and event/report handling are envelope-first (`command`, `event`, `event.ack`, `report`) and flow through protocol framing/TLV validation.
- Complex intents are modeled as multiple single-command loops in one ordered command plan.
- Blocking command steps acquire a seed lock scoped to `ghost_id::seed_selector`; while lock is held by another intent, Mirage emits `report(phase=in_progress, completion_state=in_progress)` and does not dispatch the blocked command.
- Ghost event ingest is idempotent by `event_id`; duplicates do not produce duplicate desired/observed transitions.
- Report emission is an explicit Mirage user boundary with bounded history for `intent` progress and terminal outcomes.
- Mirage local Ghost spin-up is exposed through a decoupled boundary adapter to root Ghost admin (`spawn_ghost`), not by direct package coupling.

Phase 5 models:

- Architecture boundary: [`models/phase5_orchestration_boundary.mmd`](models/phase5_orchestration_boundary.mmd)
- Event-to-report custody flow: [`models/phase5_event_report_flow.mmd`](models/phase5_event_report_flow.mmd)
- Updated single-intent flow: [`models/single_intent.mmd`](models/single_intent.mmd)
