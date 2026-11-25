# Northbound api/v1 for a web GUI (2026-11-24)

Status: contract approved by the user 2026-11-24. Mirage side implemented and verified (commit `3c3ad89`); the hardening pass below is uncommitted.
The tracker_suite module is not started.

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
