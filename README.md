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

Prerequisites: Go `1.25.6` or newer (see `go.mod`). The certificate lab below also needs `openssl`
and `nc`. Everything runs from the repo root.

### Run the tests

```bash
make test             # interactive picker, by package
make test-override    # every package, no UI
go test -race -count=1 ./...
```

### Two ways to run the stack

| | Plaintext quick look | Certificate lab (full stack) |
|---|---|---|
| Start with | `make run-mirage`, `make run-ghost` | `testutil/adminlab/setup.sh`, then `up.sh` |
| Needs | `cmd/miragectl/config.toml` and `cmd/ghostctl/config.toml` (copy the `ex.config.toml` files) | a checkout with no `certs/` of its own |
| Admin transport | plaintext, loopback only | mutual TLS 1.3 |
| Read status, ghosts, seed catalogs | yes | yes |
| Submit an intent | yes | yes |
| Execute a command on a Ghost | **no** — refused with `error_code` 1000 | yes |
| Dependency approvals, deploy, spawn | **no** | yes (installs stay disabled by default) |

Command execution is a privileged action and always requires mutual TLS
(`docs/architecture/definitions/admin_security.toml`). The plaintext stack is useful for checking that
the runtimes start and for reading state; an intent submitted there ends `failed` with
`authenticated peer required: execute_envelope`. Use the lab to see the system actually work.

### Plaintext quick look

```bash
make run-mirage    # Mirage admin 127.0.0.1:7001, session :9000, managed ghost.local admin 127.0.0.1:7010
make run-ghost     # standalone Ghost admin 127.0.0.1:7011 (second terminal)
make run-client    # terminal client; answer m (Mirage) or g (Ghost)
```

Addresses come from the two `config.toml` files, which are gitignored. A fresh clone has neither:

```bash
cp cmd/miragectl/ex.config.toml cmd/miragectl/config.toml
cp cmd/ghostctl/ex.config.toml cmd/ghostctl/config.toml
```

In the Ghost copy, change `id` (the example uses `ghost.local`, the same id as Mirage's managed
Ghost) and set `project_fetch_on_boot = false` unless you want a `git fetch` at startup. Mirage dials
every `[[preload_ghost_admins]]` entry at startup, including remote hosts.

To make this stack execute commands without the lab, issue certificates for the example ids and
uncomment the `admin_tls_*` and `local_ghost_admin_tls_*` lines in `cmd/miragectl/config.toml`:

```bash
make certs-root        # only in a checkout with no certs/ yet: it replaces certs/ca.crt
make certs-mirage ID=mirage.local IP=127.0.0.1
make certs-ghost ID=ghost.local IP=127.0.0.1
make certs-operator NAME=<you>
```

Admin requests then need the operator certificate (`operator_*` in the client-tm configs).

### Certificate lab (full stack on loopback)

The lab creates a throwaway root CA and certificates in `certs/` and refuses to run if `certs/`
already holds files it did not create. If this checkout has a real `certs/`, use a second worktree:

```bash
git worktree add --detach ../edgectl-lab HEAD
cd ../edgectl-lab
```

Then:

```bash
sh testutil/adminlab/setup.sh    # lab CA and certs in certs/, binaries in local/adminlab/
sh testutil/adminlab/up.sh       # prints "lab up"
sh testutil/adminlab/down.sh     # stop everything
```

| Process | Principal | Address |
|---|---|---|
| Mirage admin | `edgectl://mirage/mirage.lab` | `127.0.0.1:7020` |
| Mirage session | — | `127.0.0.1:9000` |
| Managed local Ghost admin | `edgectl://ghost/ghost.local` | `127.0.0.1:7010` |
| Standalone Ghost admin | `edgectl://ghost/ghost.lab` | `127.0.0.1:7011` |

Operator identities `ops` and `lead` are created in `certs/`. Logs are in `local/adminlab/mirage.log`
(Mirage and the managed `ghost.local`) and `local/adminlab/ghost.log`.

### What you can do in the lab

`testutil/adminlab/admin.sh <addr> <cert-name|none> '<json>'` sends one admin request and prints the
one-line JSON response. An empty response means the server dropped the connection.

```bash
A=testutil/adminlab/admin.sh; M=127.0.0.1:7020

# Read state
sh $A $M ops '{"action":"status"}'
sh $A $M ops '{"action":"registered_ghosts"}'
sh $A $M ops '{"action":"routing_table"}'
sh $A $M ops '{"action":"available_services"}'
sh $A $M ops '{"action":"seed_catalog"}'

# Run one command end to end: issue -> command -> seed -> event -> report
sh $A $M ops '{"action":"submit_issue","issue":{"intent_id":"intent.demo","target_scope":"ghost:ghost.local","objective":"flow status","stages":[{"id":"s1","commands":[{"ghost_id":"ghost.local","seed_selector":"seed.flow","operation":"status"}]}]}}'
sh $A $M ops '{"action":"reconcile_intent","intent_id":"intent.demo"}'    # "CompletionState":"satisfied"
sh $A $M ops '{"action":"snapshot_intent","intent_id":"intent.demo"}'
sh $A $M ops '{"action":"list_intents"}'
sh $A $M ops '{"action":"recent_reports","limit":5}'

# Bring the standalone Ghost under this Mirage
sh $A $M ops '{"action":"attach_ghost_admin","ghost_admin_addr":"127.0.0.1:7011"}'
```

The caller's certificate is the actor: leave `actor` out of the issue. An intent can be re-submitted
only by the operator who submitted it.

Follow one intent through both runtimes by its `trace_id`, and see every admin decision:

```bash
grep 'trace.intent.demo' local/adminlab/*.log    # custody chain
grep 'admin.audit' local/adminlab/*.log          # allow/deny per request
```

**See the security boundary refuse things:**

```bash
sh $A $M none '{"action":"status"}'                 # no certificate: connection dropped
sh $A $M ghost.lab '{"action":"status"}'            # wrong role: 1000
sh $A 127.0.0.1:7011 ops '{"action":"status"}'      # operator access is off on this Ghost: 1000
sh $A $M lead '{"action":"submit_issue","issue":{"intent_id":"intent.demo","target_scope":"ghost:ghost.local","objective":"x","stages":[{"id":"s1","commands":[{"ghost_id":"ghost.local","seed_selector":"seed.flow","operation":"status"}]}]}}'    # another operator's intent: 1000
```

**Two-operator dependency approval** (`ops` proposes, `lead` approves). With the lab default
`seed_install_enabled = false` the last step ends `failed` with Ghost code 1400, which is the
expected result; nothing is installed. Attach `ghost.lab` first (above). The proposal id is the `id`
field in `prop.json`.

```bash
S=local/adminlab/sign
$S propose -as ops -ghost ghost.lab -seeds seed.docker | sh $A $M ops - > prop.json
$S approve -as lead -proposal prop.json | sh $A $M lead -
sh $A $M ops '{"action":"list_dependency_proposals"}'
sh $A $M ops '{"action":"execute_dependencies","dependency":{"proposal_id":"depprop.ghost.lab.1"}}'
```

**More seeds.** The lab starts with `seed.flow` only. Edit `local_ghost_seeds` in
`testutil/adminlab/mirage.toml` and restart the lab (the file is tracked; do not commit the edit):

```toml
local_ghost_seeds = ["seed.flow", "seed.kv", "seed.fs", "seed.host"]
```

| Seed | Operations | Arguments |
|---|---|---|
| `seed.flow` | `status`, `echo`, `step` | none for `status` |
| `seed.kv` | `put`, `get`, `delete`, `list` | `key`, `value`; `prefix` for `list` (in memory) |
| `seed.fs` | `write`, `read`, `delete`, `list` | `path`, `content`; files land in `local/dir/<ghost_id>/` |
| `seed.host` | `status`, `ports`, `interfaces`, `brew_version` | none; reads this machine |

Pass arguments as `"args":{"key":"greeting","value":"hello"}` inside the command. `seed.mongod` and
`seed.docker` act on real services on this machine (`systemctl`, `docker`); leave them out unless you
mean it.

**Read a command's output.** Reports carry the outcome, not stdout. Read the execution record from
the Ghost that ran it; this needs operator access on that Ghost
(`local_ghost_admin_operator_access = true` in `mirage.toml`, `admin_operator_access = true` in
`ghost.toml`):

```bash
sh $A 127.0.0.1:7010 ops '{"action":"execution_by_command_id","command_id":"cmd.intent.demo.1"}'
sh $A 127.0.0.1:7010 ops '{"action":"recent_events","limit":5}'
```

**Terminal client against the lab.** `client-tm` reads `cmd/client-tm/a.mirage.toml` and
`a.ghost.toml` and rewrites both on exit (tracked files; do not commit lab values). Set:

```toml
# cmd/client-tm/a.mirage.toml
operator_cert_file = "certs/ops.crt"
operator_key_file = "certs/ops.key"
ca_file = "certs/ca.crt"
local_ghost_id = "ghost.local"
local_ghost_admin_addr = "127.0.0.1:7010"

[[targets]]
  name = "lab-mirage"
  addr = "127.0.0.1:7020"
  mirage_id = "mirage.lab"
```

```toml
# cmd/client-tm/a.ghost.toml (Ghost mode needs admin_operator_access = true on the Ghost)
operator_cert_file = "certs/ops.crt"
operator_key_file = "certs/ops.key"
ca_file = "certs/ca.crt"

[[targets]]
  name = "lab-ghost"
  addr = "127.0.0.1:7011"
  ghost_id = "ghost.lab"
```

Then `go run ./cmd/client-tm -mode mirage` (or `-mode ghost`). Each target's id must equal the id in
that server's certificate. Menus are described in `cmd/client-tm/README.md`; the intent templates
there need `seed.fs`, `seed.kv` or `seed.host` on a connected Ghost.

The full list of admin actions and which role may call each is in
`docs/architecture/definitions/admin_security.toml`; more attack-style probes are in
`testutil/adminlab/README.md`.

### Safety

- Keep `seed_install_enabled = false` outside a disposable container or VM: an approved plan runs
  `brew`/`curl` installs on the Ghost's host.
- Do not run `deploy_ghost` or `make update-pi` while testing locally; both reach other hosts.
- An admin connection idle for five minutes is closed by the server; the client's next request
  fails once and the one after reconnects.

### Certificates for real hosts

```bash
make certs-root                                  # once; writes certs/root and REPLACES certs/ca.crt
make certs-mirage ID=mirage.mac DNS=mirage.mac IP=192.168.50.94
make certs-ghost ID=ghost.pi                     # ID must equal the ghost_id
make certs-operator NAME=<you>                   # client-tm operator identity
```

Principals are `edgectl://<role>/<id>`. Keep `certs/root` offline: a runtime needs only
`certs/ca.crt` plus its own certificate and key. Configure `admin_tls_*` on ghostctl and miragectl and
`operator_*` in client-tm (see `cmd/ghostctl/ex.config.toml`).

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
