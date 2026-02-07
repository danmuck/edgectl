# EdgeCTL

EdgeCTL is a control plane for distributed service execution across heterogeneous nodes.

- `Mirage` owns orchestration (desired state, reconciliation, report emission).
- `Ghost` owns execution (command routing, seed dispatch, event emission).
- `Seed` packages expose service operations Ghost can execute.

Ghost is Mirage-agnostic at runtime: Ghost can run headless and execute commands without Mirage.

## Current Project State

- Protocol/frame/TLV/schema packages are implemented and tested.
- Mirage<->Ghost session path is implemented (`register`, `event`, `event.ack`, reconnect/backoff, transport security policy hooks).
- Mirage Phase 5 orchestration baseline is in progress and mostly implemented in code (`issue` ingest, in-memory desired/observed state, reconcile loop, report history, local Ghost spawn boundary).
- Phase templates for 6-9 are defined in progress docs; later phases are not started.
- Full repository tests currently pass with `go test ./...`.

## Architecture and Source of Truth

- Docs root: [`docs/index.md`](docs/index.md)
- Normative contracts: `docs/architecture/definitions/*.toml`
- Canonical diagrams: `docs/architecture/models/*.mmd`
- Progress tracker index: [`docs/progress/index.md`](docs/progress/index.md)
- Canonical MVP schedule: [`docs/progress/mvp_buildplan.md`](docs/progress/mvp_buildplan.md)

## Repository Layout

- `cmd/ghostctl`: Ghost runtime entrypoint
- `cmd/miragectl`: Mirage runtime entrypoint
- `cmd/client-tm`: terminal client for Ghost admin control/testing
- `cmd/testctl`: test inventory + interactive test runner
- `internal/protocol`: frame/TLV/schema/session transport primitives
- `internal/ghost`: Ghost lifecycle, command loop, admin control boundary
- `internal/mirage`: Mirage lifecycle server + orchestration boundary
- `internal/seeds`: seed interfaces, registry, install policies, built-in seeds

## Runtime and Control Boundaries

- Mirage handles `issue`/reconcile/report boundaries.
- Ghost handles `command`/execution/event boundaries.
- Mirage does not execute seeds directly.
- Any Ghost may host any seed type; locality is a scheduling preference.

## Configuration

Both runtimes load TOML config.

- Ghost default config path: `cmd/ghostctl/config.toml`
- Mirage default config path: `cmd/miragectl/config.toml`
- Ghost example config: `cmd/ghostctl/ex.config.toml`

Key Ghost config capabilities already wired:

- `project_fetch_on_boot` for repo refresh on startup
- Mirage session policy (`headless|auto|required`) and transport security fields
- seed-install allowlist + methods (`github`, `workspace_copy`, `brew`)
- optional Homebrew bootstrap command when missing

## Local Development

Prerequisites:

- Go `1.25.6` (see `go.mod`)

Common commands:

```bash
# interactive test picker (by module/package)
make test

# run all tests without UI
make test-override

# start runtimes
make run-mirage
make run-ghost

# terminal client for Ghost admin/testing
make run-client
```

## Testing

- Unit/integration coverage exists for protocol, session, Ghost, Mirage, and seed installers.
- `cmd/testctl` provides grouped inventory and interactive package selection.
- Baseline full sweep command:

```bash
go test ./...
```

## Notes

- This repository can be in a multi-agent, in-progress state during development.
- Treat `docs/architecture/definitions/*.toml` and `docs/architecture/models/*.mmd` as canonical when reconciling behavior.
