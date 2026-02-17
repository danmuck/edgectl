# client-tm

`client-tm` is the interactive operator terminal for Ghost and Mirage admin control paths.

## Status Legend

- `Implemented`: option is wired in `client-tm` and executes a concrete backend action.
- `Partial`: option works but has known scope limits, or broader phase requirements are still open.
- `Planned`: placeholder surface exists but end-to-end behavior is not yet complete.

## Launch Modes

- `mirage` mode (default, primary): `go run ./cmd/client-tm -mode mirage`
- `ghost` mode (headless-ghost operations): `go run ./cmd/client-tm -mode ghost`

## Main Menu (Ghost Mode, Headless Operations)

| Option | Functionality | Status | End-to-End Notes |
| --- | --- | --- | --- |
| `1) List ghost targets` | Shows configured Ghost targets, active marker, live status lookup. | Implemented | Works if target admin endpoint is reachable. |
| `2) Add/provision ghost target (persist)` | Uses active root Ghost `spawn_ghost`, persists target into config. | Implemented | Provisioning path is wired Ghost->admin boundary. |
| `3) Select active ghost target` | Switches active target context for all Ghost actions. | Implemented | Multi-target switching is stable and explicit. |
| `4) Show active target summary` | Displays Ghost lifecycle and seed inventory summary. | Implemented | Depends on target responding to `status` and catalog queries. |
| `5) Ghost admin console` | Opens Ghost operation menu. | Implemented | Full Ghost admin path is wired. |
| `6) Remove ghost target` | Removes runtime/config target and closes connection. | Implemented | Local config/runtime operation only. |
| `7) Config menu` | Runtime toggles and config save/reset controls. | Implemented | Applies to local client config files. |
| `8) Exit` | Saves config and closes all admin connections. | Implemented | Deterministic shutdown path. |

## Ghost Admin Console

| Option | Functionality | Status | End-to-End Notes |
| --- | --- | --- | --- |
| `1) Show status` | Reads Ghost lifecycle status. | Implemented | E2E when Ghost admin is reachable. |
| `2) List seeds and operations` | Reads Ghost seed catalog + idempotency flags per operation. | Implemented | Catalog is Ghost-owned and runtime-discovered. |
| `3) Execute seed command` | Command-template wizard -> command envelope -> `execute_envelope`. | Implemented | Core execute path is envelope-only and validated in Phase 6. |
| `4) Lookup execution by command_id` | Resolves stored execution state for one command. | Implemented | Depends on Ghost execution state retention. |
| `5) Show recent events` | Reads recent event stream summaries from Ghost. | Partial | Event listing works; richer observability contract completion remains open in phase tracker. |
| `6) Protocol/message verification view` | Displays request/trace IDs and command->execution->event linkage. | Partial | View works; timeout/retry/error semantics are still open in phase-break checklist. |
| `7) Back` | Returns to previous menu. | Implemented | Navigation control only. |

## Main Menu (Mirage Mode, Primary)

| Option | Functionality | Status | End-to-End Notes |
| --- | --- | --- | --- |
| `1) Show mirage control-plane config` | Displays active mirage target and local Ghost linkage. | Implemented | Local config view. |
| `2) Show mirage status` | Reads mirage lifecycle counters and control-plane summary. | Implemented | Requires Mirage admin endpoint reachability. |
| `3) Mirage admin console` | Opens Mirage orchestration/admin menu surfaces. | Partial | Menu notes `(*) not yet fully implemented`; major flows are wired but phase-break remains open. |
| `4) Show connected ghosts` | Lists Mirage-registered Ghosts and connection state. | Implemented | Discovery/read-model surface is wired. |
| `5) Open local ghost admin console` | Jumps from Mirage mode into configured local Ghost console. | Implemented | Depends on `local_ghost_admin_addr` correctness. |
| `6) Config menu` | Shared config/runtime toggle menu. | Implemented | Same as Ghost mode config menu. |
| `7) Exit` | Saves config and closes all connections. | Implemented | Deterministic shutdown path. |

## Mirage Admin Console

| Option | Functionality | Status | End-to-End Notes |
| --- | --- | --- | --- |
| `1) Show status` | Reads active Mirage status. | Implemented | Requires Mirage admin connectivity. |
| `2) Show available services` | Lists available seed services across connected ghosts. | Implemented | Uses Mirage read-model (`available_services`). |
| `3) Issue intent` | Intent template wizard, orchestration stage build, submit + reconcile loop. | Partial | Functional for wired templates; broader failure semantics/retry policy are still phase-open. |
| `4) Reconcile intent` | Opens reconcile sub-menu (`list`, `reconcile one`, `reconcile all`). | Implemented | Reconcile commands are wired to Mirage admin. |
| `5) Reports` | Opens reports sub-menu (`snapshot`, `recent reports`). | Implemented | Snapshot/report surfaces are wired. |
| `6) Ghost routing` | Opens routing sub-menu (`spawn local`, `attach`, `table`, `deploy remote`). | Partial | Core actions exist; marked `(*)` in UI for ongoing hardening. |
| `7) Back` | Returns to previous menu. | Implemented | Navigation control only. |

## Reconcile Sub-Menu

| Option | Functionality | Status | End-to-End Notes |
| --- | --- | --- | --- |
| `1) List intents` | Lists known intent IDs. | Implemented | Read-model operation. |
| `2) Reconcile one intent` | Runs one reconcile pass for selected intent. | Implemented | Works against active Mirage target. |
| `3) Reconcile all intents` | Runs reconcile across all known intents. | Implemented | Returns report list per pass. |
| `4) Back` | Navigation. | Implemented | Menu control only. |

## Reports Sub-Menu

| Option | Functionality | Status | End-to-End Notes |
| --- | --- | --- | --- |
| `1) Snapshot intent` | Reads desired/observed snapshot and pending counts. | Implemented | Wired to `snapshot_intent`. |
| `2) Show recent reports` | Shows report history with command/event/outcome metadata. | Implemented | Wired to `recent_reports`. |
| `3) Back` | Navigation. | Implemented | Menu control only. |

## Ghost Routing Sub-Menu

| Option | Functionality | Status | End-to-End Notes |
| --- | --- | --- | --- |
| `1) Spawn local ghost` | Requests Mirage-managed local Ghost spawn. | Implemented | End-to-end requires local runtime/process prerequisites. |
| `2) Connect ghost service` | Attaches external Ghost admin endpoint into Mirage routing. | Implemented | Requires reachable admin endpoint. |
| `3) Show routing table` | Lists ghost routing entries and connectivity state. | Implemented | Read-model operation. |
| `4) Deploy remote ghost (SSH)` | Sends remote deployment manifest to Mirage. | Partial | Submission is wired; actual deployment success depends on SSH host/key/runtime environment. |
| `5) Back` | Navigation. | Implemented | Menu control only. |

## Config Menu

| Option | Functionality | Status | End-to-End Notes |
| --- | --- | --- | --- |
| `1) Toggle clear-screen` | Toggles terminal clear between actions. | Implemented | Local client behavior only. |
| `2) Save configs` | Persists Ghost/Mirage config files. | Implemented | Writes `cmd/client-tm/a.ghost.toml` and `cmd/client-tm/a.mirage.toml`. |
| `3) Reset configs to defaults` | Resets runtime/config to local defaults after confirmation. | Implemented | Destructive to client target config only. |
| `4) Back` | Navigation. | Implemented | Menu control only. |

## Intent Catalog (Mirage Issue Templates)

The templates below are statically defined in `cmd/client-tm/cmd_mirage.go` and filtered by discovered service availability.

| Intent ID | Label | Seed Dependencies | Targeting Model | Current Status | End-to-End Notes |
| --- | --- | --- | --- | --- | --- |
| `intent.seed.fs.store_file` | Store File (seed.fs) | `seed.fs` | Single ghost, explicit ghost selection | Implemented | Single-stage `seed.fs/write`; requires at least one connected ghost advertising `seed.fs`. |
| `intent.seed.fs.distribute_file` | Distribute File (seed.fs) | `seed.fs` | Multi-ghost fanout (all connected ghosts with `seed.fs`) | Implemented | Multi-stage orchestrator path is wired and phase-tracker validated. |
| `intent.multi.store_and_index` | Store and Index File (seed.fs + seed.kv) | `seed.fs`, `seed.kv` | Multi-stage (`fs` write then `kv` index fanout) | Implemented | Orchestrator/template path is covered by tests and phase closeout notes. |
| `intent.docker.deploy` | Deploy Docker Container | `seed.docker`, `seed.host` | Single ghost, explicit ghost selection, 4-stage flow | Partial | Template is wired; runtime success depends on host/docker availability and deployment conditions. |
| `intent.fleet.inventory` | Collect Fleet Inventory | `seed.host` (+ optional `seed.kv`) | Fleet fanout across connected `seed.host` ghosts | Partial | Template is wired; optional index stage runs only when `seed.kv` is available. |

## Seed Service Catalog

`client-tm` uses two seed-service surfaces:
- Ghost command wizard: dynamically from Ghost `list_seed_catalog`.
- Mirage intent templates: static seed dependencies listed above.

| Seed Service | Typical Operations | Where Used in client-tm | Current Status | End-to-End Notes |
| --- | --- | --- | --- | --- |
| `seed.fs` | `write`, `read`, `delete`, `list` | Ghost command wizard + Mirage intents | Implemented | Core file-control intent paths are wired and validated in orchestration flow. |
| `seed.kv` | `put`, `get`, `delete`, `list` | Ghost command wizard + Mirage intents | Implemented | Used in multi-stage indexing and optional inventory indexing. |
| `seed.docker` | `status`, `ps`, `run`, `stop`, `rm`, `logs`, `inspect` | Ghost command wizard + Mirage docker deploy intent | Partial | Adapter exists and orchestrator path exists; environment-dependent runtime success. |
| `seed.host` | `status`, `ports`, `interfaces` | Ghost command wizard + Mirage intents | Implemented | Used for host checks and fleet fanout inventory collection. |
| `seed.flow` | `status`, deterministic step/example actions | Ghost command wizard | Implemented | Ghost-side execution path available through discovered templates. |
| `seed.mongod` | `status`, `start`, `stop`, `restart` (seed-defined) | Ghost command wizard | Partial | Seed adapter exists; no dedicated Mirage intent template in `client-tm` catalog yet. |
| `custom seed.*` | Seed-defined | Ghost command wizard | Partial | Available when Ghost advertises operation + command template metadata. |

## Known Scope Limits

- Mirage target support is currently single-control-plane in client runtime (`only first target is supported` behavior).
- Mirage and verification menus are intentionally marked partial in UI while phase-break checklist items remain open (failure semantics, observability completeness, and hardening).
- Full hardening phase (timeouts/retries/idempotency policies and replay handling) is tracked separately in `docs/progress/mvp_p8.md`.
