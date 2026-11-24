# Admin Security Test Brief (third-party tester)

You are testing the admin security boundary of edgectl on branch `staging`. Your job is to find ways
around it, not to confirm it works. Report what you find; do not fix code.

## What is under test

Seven commits (slices A–F of `docs/progress/admin_security.md`) replaced a shared admin token with
certificate identities:

- Every admin caller and server is a certificate principal `edgectl://<role>/<id>` (roles: `operator`,
  `mirage`, `ghost`) in exactly one URI SAN, under one root CA, with a denied-serial list.
- Both admin endpoints (Mirage, Ghost) are TLS 1.3 mutual TLS. Plaintext is allowed only on loopback
  outside production, and privileged actions always need mTLS.
- Default-deny role authorization; the caller's principal is the actor (no spoofing); a Ghost obeys
  only its one pinned Mirage principal; operator access to a Ghost is off unless enabled.
- Dependency installs need two operators: the proposer signs an intent, a different approver signs an
  approval embedding it. Mirage and every Ghost verify both signatures independently; the Ghost
  recomputes the install steps from its own compiled catalog. Approvals and intents are single-use.
- `deploy_ghost` names only a configured manifest; SSH host keys are pinned; each Ghost receives only
  its own certificate and key.

## Ground rules

- Source of truth: `docs/architecture/definitions/admin_security.toml`. Behavior that contradicts it is
  a finding; behavior it permits is not (see Accepted limitations). Do not edit anything under `docs/`.
- Use a fresh clone. The lab creates a throwaway root CA in `certs/` and refuses to run if `certs/`
  holds anything it did not create.
- Stay on loopback. Do not run `deploy_ghost` against real hosts, and do not set
  `seed_install_enabled = true` outside a disposable container or VM: an approved plan runs
  `brew`/`curl` installs on the Ghost's host.
- Do not push to `staging` or `dev`. Deliver a report (format at the end).

## Read first

1. `docs/architecture/definitions/admin_security.toml` — the contract.
2. `docs/progress/admin_security.md` — slice notes, decisions, coverage map.
3. Code: `internal/identity`, `internal/adminauth` (transport and authorization), `internal/approval`
   (signatures), `internal/mirage/{admin_control,dependency_*,ghost_manifest,ssh_deploy}.go`,
   `internal/ghost/{admin_control,dependencies}.go`.

## Setup (verified on macOS with Go 1.27 and OpenSSL 3; needs `go`, `openssl`, `nc`, `python3`)

```bash
git clone --branch staging git@github.com:danmuck/edgectl.git edgectl-staging
cd edgectl-staging
go test ./...                       # baseline: all packages pass
sh testutil/adminlab/setup.sh       # lab CA + certs in certs/, binaries in local/adminlab/
sh testutil/adminlab/up.sh          # starts the lab; logs in local/adminlab/*.log
```

Stop with `sh testutil/adminlab/down.sh`. Everything runs from the repo root.

| Process | Principal | Address |
|---|---|---|
| Mirage admin | `edgectl://mirage/mirage.lab` | `127.0.0.1:7020` |
| Mirage session | — | `127.0.0.1:9000` (development mode) |
| Managed local Ghost admin | `edgectl://ghost/ghost.local` | `127.0.0.1:7010` |
| Ghost admin (`ghostctl`) | `edgectl://ghost/ghost.lab` | `127.0.0.1:7011` |

Client identities in `certs/`: operators `ops` and `lead`; you may also present `mirage.lab`,
`ghost.lab`, `ghost.local`, or `none`. Issue more with
`go run ./cmd/edgecert issue -role <role> -id <id> -out certs` (or hand-craft certificates with
OpenSSL against `certs/root/ca.key`). Configs: `testutil/adminlab/{mirage,ghost}.toml`.

## Tools

`testutil/adminlab/admin.sh <addr> <cert|none> '<json>'` sends one request over TLS 1.3 and prints
the one-line JSON response (empty output means the server dropped the connection). Pass `-` as the
request to read it from stdin.

`local/adminlab/sign` (built by setup) signs anything you give it, with no safety checks, so you can
craft hostile inputs:

```bash
S=local/adminlab/sign; A=testutil/adminlab/admin.sh
$S propose -as ops -ghost ghost.lab -seeds seed.docker | $A 127.0.0.1:7020 ops - > prop.json
$S approve -as lead -proposal prop.json | $A 127.0.0.1:7020 lead -
$S approve -as lead -proposal prop.json -expires 1h        # over-long lifetime
$S intent  -as ops -ghost ghost.lab -seeds seed.docker -proposed-at-ms 1  # stale intent
$S payload -as lead -payload my_payload.json                # sign an arbitrary approval payload
```

`-as` loads only operator certificates (the signer library enforces the role). To test a
non-operator signature, write your own signer.

Wire shapes: requests are `{"action": ..., ...}`; dependency actions carry
`"dependency": {"proposal_id", "intent", "approval"}`; Ghost installs use
`{"action":"install_dependencies","install":{"proposal_id","approval"}}`; Ghost commands use
`{"action":"execute_envelope","command_frame":<base64 frame>}`. Responses are
`{"ok", "error", "error_code", "data"}`.

## Verified baseline (all observed on this lab)

| Request | Expected |
|---|---|
| `ops` → Mirage `status` | `ok:true` |
| no client cert → Mirage | connection dropped |
| `ghost.lab` → Mirage `status` | `1000` authorization_denied |
| `ops` → Ghost `status` | `1000` (operator access off) |
| `ops` → Mirage `attach_ghost_admin` `127.0.0.1:7011` | `ok:true`, ghost_id from the certificate |
| `deploy_ghost` with an unknown `ghost_id` | `1300` unknown deploy target |
| `propose_dependencies` without a signed intent | `1300` |
| replaying the same intent | `1300` intent already proposed |
| proposer approves own proposal | `1300` approver must differ from proposer |
| `lead`-signed approval submitted by `ops` | `1300` |
| approval lifetime over 15 minutes | `1300` |
| intent older than 24 hours | `1300` |
| `submit_issue` with `"actor":"user:someone"` | `1000` actor is not the caller |
| two-operator approve, then `execute_dependencies` | proposal `failed`, Ghost `1400` installs disabled (lab default) |

Every admin request writes an `admin.audit` line (principal, role, action, decision, error code) to
`local/adminlab/*.log`.

Error codes: `1000` transport/authorization, `1100` protocol version/flags, `1300` semantic
validation (bad request, approval, deploy target), `1400` runtime execution.

## What to try (not exhaustive)

- **Identity:** certificates with zero, two, or malformed URI SANs; principals that differ only in
  case or encoding; CN or DNS SANs that look like principals; wrong key usages; expired, not-yet-valid,
  or denied-serial certificates; chains through an intermediate; certificates from another root.
- **Transport:** plaintext to TLS listeners; TLS 1.2; renegotiation; slow or partial handshakes;
  oversized or multi-line requests; many connections (note: denial of service is out of scope, but
  crashes and authorization mix-ups are in scope).
- **Authorization:** every action on both endpoints with every role; unknown actions; a non-pinned
  Mirage on a Ghost; actor fields on every action that has one; the read/privileged split.
- **Approvals:** signature malleability; swapping the intent, steps, ghost_id, or proposal_id between
  signed objects; re-wrapping a used intent in a new approval; replays across proposals and Ghosts;
  clock edges around expiry and skew (±1 minute); proposals pending across a Ghost restart; JSON quirks
  (duplicate keys, unknown fields, null vs empty, number precision in `*_ms` fields).
- **Ghost independence:** assume Mirage is compromised. With the `mirage.lab` certificate, talk to
  the Ghost directly and try to install anything not approved by two operators for that Ghost.
- **Deploy:** manifest values that become ssh/scp options or shell (host, user, paths, host key);
  template keys that redirect credentials or unpin Mirage; anything that leaks key material into
  logs, responses, or generated config. Read `internal/mirage/ssh_deploy.go` and use its tests' fake
  runner rather than real SSH.
- **Removed token:** confirm no path still accepts or emits the old shared token or frame auth bytes.

## Accepted limitations (not findings unless the contract is violated)

- Plaintext admin is allowed on loopback in development mode; privileged actions are still refused.
- Replay records are in memory. After a Ghost restart, intents proposed before it started are refused
  (so a pending proposal must be re-proposed); this is intentional.
- Deploy hosts must be hostnames or IPv4 literals; IPv6 literals are rejected.
- The session protocol keeps its `has_auth` frame flag (canonical `protocol.toml`); only the admin
  `execute_envelope` path refuses frames that set it.
- `brew` installs are host-global by design (allowed exception in Phase 8).
- Denial of service, multi-host behavior, and the security of the hosts themselves are out of scope.
- The lab runs Mirage sessions in development mode.

## Automated coverage you can build on

`go test ./...`; boundary-focused tests:
`internal/adminauth/*_test.go`, `internal/identity/identity_test.go`, `internal/approval/approval_test.go`,
`internal/ghost/{admin_negative,admin_tls,dependencies,e2e_admin}_test.go`,
`internal/mirage/{admin_negative,admin_tls,dependency,ssh_deploy}_test.go`.

## Report format

One Markdown file. For each finding:

```
### <short title>
- Severity: critical | high | medium | low | info
- Contract clause: admin_security.toml [section] key (or "none")
- Component: file:line
- Reproduction: exact commands from a fresh clone + lab (setup.sh / up.sh)
- Observed vs expected:
- Suggested fix (optional):
```

End with the commit you tested (`git rev-parse HEAD`), what you covered, and what you did not get to.
