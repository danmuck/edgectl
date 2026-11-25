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
- Example configs: `cmd/ghostctl/ex.config.toml`, `cmd/miragectl/ex.config.toml`

Key Ghost config capabilities already wired:

- `project_fetch_on_boot` for repo refresh on startup
- Mirage session policy (`headless|auto|required`) and transport security fields
- seed-install enablement, managed roots, and seed whitelist; Ghost installs only Mirage-approved plans (`propose_dependencies` -> `approve_dependencies` -> `execute_dependencies`) built from Mirage's dependency catalog (`internal/depcatalog`); an operator signs each approval (client-tm Mirage console > Dependencies) and Ghost verifies the signature itself before installing only the signed catalog steps
- install methods allowlisted: `github` (pinned ref), `curl` (https + sha256), `brew`, `stream` (Mirage -> Ghost, transfer deferred)

## Local Development

Prerequisites: Go `1.25.6` or newer (see `go.mod`). Run everything from the repo root.

```bash
make test    # interactive test picker; `make test-override` runs every package
make lab     # start a local Mirage and Ghost and open the lab menu
```

`make lab` is the way to try the system by hand. It builds Mirage from this checkout, creates
throwaway certificates, starts a Mirage and a Ghost on loopback with mutual TLS, and opens a menu
that shows what is running and lets you add Ghosts, run seed commands and see their output, and
review recent activity. Everything it creates lives in `local/lab/` (gitignored) and stops when you
exit. It never touches `certs/`.

Other entry points:

- `make run-mirage`, `make run-ghost`, `make run-client` run the runtimes from your own
  `cmd/*/config.toml` (copy the `ex.config.toml` files). Without `admin_tls_*` certificates they
  serve plaintext on loopback and refuse to execute commands
  (`docs/architecture/definitions/admin_security.toml`).
- `make certs-root`, `certs-mirage`, `certs-ghost`, `certs-operator` issue certificates for real
  hosts (`make certs` lists usage). `certs-root` replaces `certs/ca.crt`; keep `certs/root` offline.
- `testutil/adminlab/README.md` is the brief for attack-style testing of the admin boundary.

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
