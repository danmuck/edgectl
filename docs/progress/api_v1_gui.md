# Northbound api/v1 for a web GUI (2026-11-24)

Status: contract approved 2026-11-24 and implemented (commits `3c3ad89`, `aed7341`). Operator parity
amendment approved 2026-11-25 and implemented, uncommitted. The tracker_suite module exists for v1
and is being extended for parity.

## Why

The user wants tracker_suite's frontend to be the GUI for edgectl, local-only first. edgectl has no
browser-reachable surface: the only control interface is JSON lines over mutual TLS on Mirage's
admin port. The docs already place a frontend behind `api/v1` on Mirage
(`architecture/future_api_v1.md`, `mvp_p9.md`), so the first step is to turn that proposal into a
contract and implement it.

## User decisions (in thread)

- Build `api/v1` in Mirage rather than a bridge inside the GUI.
- There is one operator; the GUI is the frontend for that Mirage operator.
- One Mirage is the point of truth; other Mirages are backups for it.
- Stay true to the canonical message types.
- Record the contract in a new definitions file; leave `future_api_v1.md` unchanged.

## Contract

- `docs/architecture/definitions/api_v1.toml`
- `docs/architecture/models/api_v1_boundary.mmd`

## Decisions made by the builder (review these)

- **Two endpoints beyond the proposal's twelve.** `GET /api/v1/seeds` (`seed_catalog`) and
  `GET /api/v1/executions/{command_id}`. Without the first a GUI cannot build an `issue`; without
  the second it cannot show `seed.result` output, because a frontend may not call a Ghost.
- **Mirage reads an execution record from the Ghost for the client.** New Mirage behavior, using the
  existing Ghost action `execution_by_command_id` as the pinned mirage principal.
- **Correlation in headers, not bodies.** The proposal says writes carry `request_id`. The contract
  puts it in `X-Request-Id` so the `POST /api/v1/intents` body is exactly an `issue`.
- **Same auth as the admin endpoint.** Operator principal from the client certificate; no tokens,
  no browser-facing auth, no cross-origin headers. The GUI's server is the client.
- **Separate listener**, `api_v1_addr`, disabled when empty.
- **New snake_case response shapes** defined for `api/v1` only; the admin wire shapes do not change.
- **HTTP status mapping** for the canonical wire codes (`[envelope.http_status]`).
- **Left out:** `deploy_ghost`, dependency proposals, metrics. Dependency approval requires two
  operators (`admin_security.toml [install_approval]`), which does not fit a single operator and
  needs its own decision.

## Open point for the user

`docs/index.md` is canonical and was not edited, so the new contract and diagram are not linked from
it yet. Linking them needs approval.

## Outcome (2026-11-24)

Implemented in `internal/mirage/api_v1.go` (server, routes, envelope, error mapping),
`api_v1_shapes.go` (contract shapes) and `api_v1_test.go`.

- [x] `api/v1` HTTP server in Mirage with all 14 contract endpoints.
- [x] `api_v1_addr` config key in `miragectl` (off by default), documented in `ex.config.toml`.
- [x] Mirage execution read: `GhostControlClient.ExecutionByCommandID`.
- [x] Contract tests: operator flow, denials and request errors, execution read, plaintext rule.
- [x] Lab mode serves `api/v1` on 7021 and prints the operator certificate paths.
- [ ] tracker_suite `edgectl` module (separate repository).

Verified: `go test -race ./...` passes; no change under `internal/protocol`. Against a real lab over
mutual TLS with `curl`: status, ghosts, seeds, submit a two-command `seed.kv` issue, reconcile twice,
snapshot, execution output (`world`), and refusals for no certificate and for a Ghost certificate.

### How it differs from the plan

The plan named a new package `internal/apiv1`. It is in package `mirage` instead. Each endpoint
builds an `adminControlRequest` and calls the existing `authorizeAdminRequest` and
`handleAdminControlRequest`, so an endpoint cannot behave differently from its admin action. A
separate package would have needed those exported.

### Changes to existing code

- `adminControlResponse` keeps its underlying error in an unexported field (not on the wire) so the
  HTTP status can be chosen with `errors.Is`.
- `ghostControlRequest` gained `command_id`.
- `ServiceConfig.APIv1ListenAddr`; `Service.Run` starts the listener when it is set.

### Found while testing

- **A GUI cannot add a Ghost on its own.** `spawn-local` fails unless a certificate for the child
  already exists, because only operator tooling issues certificates
  (`admin_security.toml [trust] leaf_issuance`). The lab menu issues one before it spawns. A GUI
  "Add a Ghost" needs a decision: issue the certificate out of band first, or define who may issue.
- **Request validation runs before authorization**, as on the admin endpoint: a non-operator with a
  malformed body gets 400 rather than 403. No state is reachable either way.
- **`retryable`** is true only when Mirage could not reach a Ghost at all. A failure a Ghost reports
  is never marked retryable.
- **Server certificate names.** Lab certificates carry `127.0.0.1` and `localhost`, so a standard
  HTTPS client verifies them. Certificates from `make certs-mirage` should be checked for the same
  before a non-lab GUI deployment.
- Error messages can contain filesystem paths from the Mirage host. Fine for one local operator;
  review before any public exposure.

## Hardening pass (2026-11-24, after an automated security review of the commit)

Both findings were valid and are fixed in `internal/mirage/api_v1.go`, with tests.

- **A web page could drive the development plaintext listener.** With no certificate configured the
  API is unauthenticated on loopback, so a page in the operator's browser could have submitted an
  `issue` by a cross-site request, or read state by DNS rebinding. Every request now passes a guard
  first: any `Origin` header or a `Sec-Fetch-Site` other than `none` is refused (403, code 1000); a
  body must be declared `application/json`, which a page cannot send cross-site without a preflight
  this server never answers (400, code 1300); and a plaintext caller must address the listener by a
  loopback literal or `localhost` (403). This enforces `api_v1.toml [transport]` `cross_origin` and
  `content_type`. With mutual TLS the first two checks still apply; the handshake already excludes
  a page.
- **`spawn-local` relied on the Ghost to refuse plaintext callers.** The contract marks it
  privileged. Mirage now refuses it for an unauthenticated peer itself, before anything is sent to a
  Ghost. `adminauth`'s privileged set is unchanged, since that list is fixed by
  `admin_security.toml`.

Client impact: send `Content-Type: application/json` with every body and no `Origin` header. A
server-side HTTPS client does both by default.

## Next

- [ ] tracker_suite `edgectl` module against the lab.
- [ ] Decide certificate issuance for "Add a Ghost" from a GUI.
- [ ] Decide whether to link the contract and diagram from `docs/index.md`.

## Operator parity amendment (2026-11-25) — approved and implemented

The user wants the tracker_suite module to have full admin control, matching the terminal client.
The amendment is in `api_v1.toml` (tables marked `amendment_2026_11_25`). The user approved it in
thread on 2026-11-25; the outcome is at the end of this note.

### What the terminal can do that the GUI cannot

| Terminal | Today | After the amendment |
|---|---|---|
| Routing table, available services, reconcile all | in `api/v1`, no GUI page | GUI only |
| Add a Ghost (issues a certificate, then spawns) | spawn fails without a certificate | GUI server issues the leaf as operator tooling |
| Deploy remote Ghost (SSH) | not in `api/v1` | `POST /ghosts/deploy`, `GET /ghosts/deploy-targets` |
| Dependencies: list, propose, approve, execute | not in `api/v1` | four routes under `/dependencies/proposals` |
| Ghost console: status, recent events, verification view | terminal calls the Ghost directly | read through Mirage, `/ghosts/{ghost_id}/...` |
| Recent activity: audit and custody lines | read from the Mirage log file | `GET /activity`, from an in-memory record |
| Ghost console: execute a seed command, look up an execution | already covered (`issue`, `/executions`) | no change |
| Target lists and config menus | client-side only | not applicable to a GUI |

### User decisions (in thread)

- Add a Ghost: the GUI server issues the certificate; Mirage never holds an issuing key.
- Dependency approvals: keep the two-operator rule; one person holds two operator keys.
- Contract amendment: draft first, review before code.

### Decisions made by the builder (review these)

- **Signing and certificate issuance should be done by edgectl tooling, not reimplemented in the
  GUI.** The signature covers Go's canonical JSON of the intent and payload; a second
  implementation in TypeScript could drift and produce signatures every Ghost rejects. Proposed:
  the GUI server shells out to `edgecert` (it already issues certificates) and to new
  non-interactive `edgecert` subcommands that sign an intent and an approval. This adds no new
  role: `edgecert` is the operator tooling the trust rules already name.
- **The payload digest comes from the signing tool, not Mirage.** The draft had Mirage return a
  `payload_digest` on each proposal. That cannot work: the payload includes the approver's nonce
  and expiry, which exist only when the approver builds it. The field is removed from the
  contract; `edgecert approval` prints the digest for the GUI to show, and `edgecert sign-approval`
  refuses to sign unless it is handed that same digest.
- **Approve must come from the second identity's connection.** Mirage checks approver == caller, so
  the GUI server needs a second client certificate and key (the lab's `lead`) and makes that one
  call as `lead`.
- **Two new Mirage reads with no admin action behind them:** deploy targets (from configured
  manifests, no credentials) and activity (a 1000-record in-memory buffer of audit and custody
  records). Everything else maps to an existing action.
- **Adding a Ghost stays local-only.** The spawned Ghost reads its certificate from Mirage's
  certificate directory, so the GUI server must be able to write there. That holds when both run on
  one machine and not otherwise.
- **Metrics remain excluded**; Mirage has no metrics read model.

### Outcome (2026-11-25)

- [x] Routes in `internal/mirage/api_v1_parity.go`: deploy, deploy targets, dependency proposals
  (list, get, propose, approve, execute), Ghost status, events and verification, activity.
- [x] `internal/activity`: in-memory record (1000 entries) fed by `adminauth.Audit` and
  `observability.Emit`.
- [x] `internal/opsign`: the terminal client's intent and approval-payload checks, moved so
  `client-tm` and `edgecert` share one implementation.
- [x] `edgecert intent | sign-intent | approval | sign-approval`.
- [x] Tests for every new route and command; `go vet ./...` and `go test -race ./...` pass; no
  change under `internal/protocol`.
- [ ] tracker_suite module extended for parity (separate repository, in progress).

Verified against a throwaway lab over mutual TLS: Ghost status, events and verification after a
`seed.kv` issue; activity (audit and custody); deploy targets; a full propose (as `ops`) and approve
(as `lead`) using the `edgecert` commands; and adding a Ghost by issuing its certificate with
`edgecert issue` and calling `spawn-local`. Refusals seen: self-approval, a wrong confirmed digest,
and `lead`'s approval sent on `ops`' connection.

Not exercised live: `execute` (it would run a real `brew install`) and a remote deploy (no SSH
target). Both are covered by tests with stubs only.

### Found while implementing

- **A spawned child's id is `<local ghost id>.<target_name>`.** The certificate must be issued for
  that full id (`ghost.local.web1` for target `web1`), or the spawn fails naming the missing file.
- **The activity feed includes the local Ghost's audit lines** when Mirage runs it in-process, as
  the lab does. Each record carries `component` so a client can tell them apart.
- **Activity is per process and in memory.** It is empty after a Mirage restart and does not include
  a remote Ghost's own records.
- `sign-approval` signs the payload it is given; the proposal checks happen in `approval`. A caller
  that skips `approval` gains nothing: Mirage and every Ghost verify the approval again.
- Still open from v1: Mirage sets `registered_at_ms` anew on every Ghost read (reported by the GUI
  session, not yet examined), and `docs/index.md` does not link the contract.

### Make targets (2026-11-25)

- `make build-edgecert` builds `local/bin/edgecert` (also part of `make build-all`).
- `make gui-env` builds it and prints the ten settings a front end's server needs for the lab, as
  `KEY=value` lines with absolute paths, reading the api/v1 address from `local/lab/mirage.toml`.
  It prints only; it does not write into another repository. `LAB_DIR=` selects another lab.
