# MVP Progress — Phase 7 (End-to-End Control Loop)

Status: `Completed`

### Tasks

- [x] Add deterministic E2E scenario: store file intent on one `seed.fs` ghost, then copy intent to all `seed.fs` ghosts (`internal/mirage/orchestration_test.go`)
- [x] Add deterministic logs for ownership transitions
- [x] Add E2E failure scenario with corrective behavior

### Acceptance Checks

- [x] Success and failure loop tests both pass
- [x] Ownership transitions are visible and unambiguous

### Closeout Prep Snapshot (2026-02-09 EST)

- [x] `seed.docker` adapter exists with operations `status|ps|run|stop|rm|logs|inspect` and unit tests (`/Users/danmuck/local/edgectl/internal/seeds/docker/seed.go`, `/Users/danmuck/local/edgectl/internal/seeds/docker/seed_test.go`)
- [x] `seed.host` adapter exists with operations `status|ports|interfaces` and unit tests (`/Users/danmuck/local/edgectl/internal/seeds/host/seed.go`, `/Users/danmuck/local/edgectl/internal/seeds/host/seed_test.go`)
- [x] Ghost builtin registry wiring includes `seed.docker|docker` and `seed.host|host` (`/Users/danmuck/local/edgectl/internal/ghost/service.go`)
- [x] Mirage seed discovery enrichment includes `seed.docker` and `seed.host` scopes (`/Users/danmuck/local/edgectl/internal/mirage/server.go`)
- [x] Ghost host summary enrichment is present in heartbeat and Mirage registration seed metadata path (`/Users/danmuck/local/edgectl/internal/ghost/service.go`, `/Users/danmuck/local/edgectl/internal/ghost/server.go`)
- [x] Add cross-ghost docker/host orchestrator E2E tests (M5) in `/Users/danmuck/local/edgectl/internal/mirage/orchestration_test.go`
- [x] Add ownership transition logs in orchestrator custody boundaries (M6) in `/Users/danmuck/local/edgectl/internal/mirage/orchestration.go`
- [x] Add/update buildlog + phase checklist closures
- [x] Add canonical `seed_examples["node.host"]` contract entry in `/Users/danmuck/local/edgectl/docs/architecture/definitions/design.toml`
