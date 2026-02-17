# MVP Progress — Phase 9 (MVP v1 Scope + Finalization)

Status: `Defined (Implementation Pending)`

## Phase Goal

Define and complete MVP v1 as a Mirage-first product with:

- strict northbound API ownership (`api/v1` only through Mirage)
- end-to-end seed orchestration for the v1 seed set
- controlled dependency deployment from Mirage to Ghosts
- an implementation-safe contract for the first real frontend

## Contract References

- `docs/architecture/definitions/protocol.toml`
- `docs/architecture/definitions/reliability.toml`
- `docs/architecture/definitions/observability.toml`
- `docs/architecture/definitions/errors.toml`
- `docs/architecture/future_api_v1.md`

## Design Artifacts (Phase 9)

- MVP v1 system boundary: `docs/progress/mvp_p9_v1_system_boundary.mmd`
- API-intent orchestration flow: `docs/progress/mvp_p9_v1_api_intent_flow.mmd`
- Dependency approval/install flow: `docs/progress/mvp_p9_dependency_approval_flow.mmd`

## MVP v1 Seed Scope

### Required End-to-End Seeds (must pass cluster validation)

- [ ] `seed.host`
- [ ] `seed.docker`
- [ ] `seed.traefik`
- [ ] `seed.kubernetes`
- [ ] `seed.frontend`
- [ ] `seed.redis`
- [ ] Existing builtin baseline seeds remain supported: `seed.flow`, `seed.fs`, `seed.kv`, `seed.mongod`

### Planned Stub Seeds (defined contracts, non-E2E acceptable in v1)

- [ ] `seed.raft-node` (stub contract + intent surface)
- [ ] `seed.kdht` (stub contract + intent surface)
- [ ] `seed.fileserver` (stub contract + intent surface)

## MVP v1 Northbound API Surface (`api/v1` via Mirage only)

All frontend calls must terminate at Mirage; no frontend-to-Ghost direct control path is allowed.

### Core Endpoint Families

- [ ] `POST /api/v1/intents` (submit intent)
- [ ] `POST /api/v1/intents/{intent_id}/reconcile` (force reconcile pass)
- [ ] `GET /api/v1/intents/{intent_id}` (intent snapshot/read model)
- [ ] `GET /api/v1/reports` (report history/query)
- [ ] `GET /api/v1/services` (cluster capability catalog by seed + ghost)
- [ ] `GET /api/v1/ghosts` (connected ghost inventory + health summary)
- [ ] `GET /api/v1/metrics/cluster` (Mirage-aggregated host/cluster metrics view)
- [ ] `POST /api/v1/dependencies/proposals` (create dependency install proposal)
- [ ] `POST /api/v1/dependencies/proposals/{proposal_id}/approve` (manual approval)
- [ ] `POST /api/v1/dependencies/proposals/{proposal_id}/execute` (Mirage-driven install execution)
- [ ] `GET /api/v1/dependencies/proposals` (audit/query approval/install state)

### Intent Families Expected for MVP v1

- [ ] Host intents: inventory, health, ports/interfaces, metrics snapshot
- [ ] Docker intents: run/stop/remove/logs/inspect fleet operations
- [ ] Traefik intents: router/service sync, reload/restart, status
- [ ] Kubernetes intents: apply/rollout/scale/status
- [ ] Frontend intents: build/deploy/restart/health/version
- [ ] Redit intents: lifecycle + health + data-plane checks
- [ ] Utility intents: file distribution/indexing (`seed.fs` + `seed.kv`) and control checks (`seed.flow`)

### Intent-to-Endpoint Catalog (v1 target)

| Endpoint                                  | Intent Family                     | Primary Seeds                | E2E Requirement |
| ----------------------------------------- | --------------------------------- | ---------------------------- | --------------- |
| `POST /api/v1/intents/host/inventory`     | Host inventory                    | `seed.host`                  | Required        |
| `POST /api/v1/intents/host/metrics`       | Host and cluster metrics snapshot | `seed.host`                  | Required        |
| `POST /api/v1/intents/docker/deploy`      | Docker deploy/update              | `seed.docker`, `seed.host`   | Required        |
| `POST /api/v1/intents/docker/operate`     | Docker lifecycle ops              | `seed.docker`                | Required        |
| `POST /api/v1/intents/traefik/reconcile`  | Traefik route/service sync        | `seed.traefik`, `seed.host`  | Required        |
| `POST /api/v1/intents/kubernetes/apply`   | Kubernetes manifest apply         | `seed.kubernetes`            | Required        |
| `POST /api/v1/intents/kubernetes/rollout` | Kubernetes rollout and scale      | `seed.kubernetes`            | Required        |
| `POST /api/v1/intents/frontend/deploy`    | Frontend build and deploy         | `seed.frontend`, `seed.host` | Required        |
| `POST /api/v1/intents/redit/reconcile`    | Redit lifecycle and health        | `seed.redit`                 | Required        |
| `POST /api/v1/intents/files/distribute`   | File distribution/indexing        | `seed.fs`, `seed.kv`         | Required        |
| `POST /api/v1/intents/raft/join`          | Raft node join and status         | `seed.raft-node`             | Stub            |
| `POST /api/v1/intents/kdht/route`         | DHT routing/replica actions       | `seed.kdht`                  | Stub            |
| `POST /api/v1/intents/fileserver/sync`    | Fileserver replication            | `seed.fileserver`            | Stub            |

## Dependency Deployment and Transfer Model (Required for MVP v1)

### Single Source-of-Truth Install Catalog

- [ ] One hardcoded catalog file defines all external dependency install paths and methods.
- [ ] Catalog contains verified source metadata (URL/repo, version/ref, checksum/signature policy, destination path).
- [ ] Catalog is Mirage-owned and versioned with the codebase.
- [ ] Ghosts do not accept ad-hoc install locations outside catalog policy.

### Install and Approval Rules

- [ ] Ghost binary ships seed interfaces only; external dependencies are not pre-bundled.
- [ ] Any dependency install requires explicit Mirage approval before execution.
- [ ] Install execution is initiated and tracked by Mirage only.
- [ ] Runtime dependencies must be managed under project-controlled roots, not host-global system installs.

### Transfer Channel Rules

- [ ] Allowed methods: `github`, `curl`, `brew`, `stream`.
- [ ] `stream` direction is strictly `mirage -> ghost` only.
- [ ] Verified-source download paths are preferred over streaming where possible.
- [ ] Transfer and install events must be auditable via report/read-model surfaces.

## Observability and Metrics Requirements for MVP v1

- [ ] `seed.host` exposes expanded node stats suitable for Prometheus ingestion and cluster aggregation.
- [ ] Mirage provides aggregated cluster metrics endpoint(s) for frontend dashboards.
- [ ] Intent/report telemetry includes correlation IDs across `issue -> command -> seed.execute -> seed.result -> event -> event.ack -> report`.

## Phase 9 Implementation Slices

### Slice A — API/Intent Contract Finalization

- [ ] Freeze v1 `api/v1` endpoint contracts and request/response shapes.
- [ ] Map each endpoint to Mirage orchestration actions and report models.
- [ ] Add contract tests for all endpoint families.

### Slice B — Seed Completion (E2E Set)

- [ ] Implement/validate required operations and templates for v1 E2E seeds.
- [ ] Ensure each seed has at least one production-useful intent family exposed via `api/v1`.
- [ ] Add cluster E2E tests for each required seed family.

### Slice C — Stub Seed Contracts (Non-E2E)

- [ ] Add placeholder seed interfaces, metadata, and intent stubs for `raft-node`, `kdht`, `fileserver`.
- [ ] Ensure stubs are non-breaking and clearly marked non-production/E2E-pending.

### Slice D — Dependency Governance and Install Control

- [ ] Land single-file dependency install catalog and loader.
- [ ] Add Mirage approval workflow and execution ledger for installs.
- [ ] Enforce transfer-direction and install-root policy at runtime boundaries.

### Slice E — Frontend Readiness Pack

- [ ] Finalize API error taxonomy mapping for frontend-safe responses.
- [ ] Finalize dashboard-focused read models (services, ghosts, reports, metrics).
- [ ] Produce frontend integration runbook using Mirage-only API paths.

## Acceptance Checks

- [ ] Phase 9 scope is approved and traced to implementation slices.
- [ ] All required E2E seeds pass intent->report cluster tests through Mirage API.
- [ ] Stub seeds are present with explicit non-E2E status and forward contracts.
- [ ] Dependency installs are Mirage-approved, audited, and policy-enforced.
- [ ] No frontend/consumer path invokes Ghost endpoints directly.
- [ ] API contracts are stable enough to begin safe public API and real frontend work.
- [ ] Release validation matrix and operator runbook/rollback checklist are complete.
