# Admin Boundary Security Tracker

Status: `In Progress (Slices A-D complete)`

Contract (approved 2026-11-23): [`docs/architecture/definitions/admin_security.toml`](../architecture/definitions/admin_security.toml)

## Why

Both admin endpoints are plaintext TCP with no caller authentication. Anyone who can reach Mirage's admin port can submit intents, approve and execute dependency installs, and run `deploy_ghost` against a host they name. Since Slice F (`0aac2bf`), that deploy also writes the shared Ghost admin token into the generated config, so a caller can obtain the one secret every Ghost uses for `execute_envelope` and `install_dependencies`. Ghost also cannot verify that a plan was approved by a person rather than just relayed by Mirage.

The goal is to replace the shared token with certificate identities end to end, not to patch individual leaks.

## Design Summary

- Every caller and server on an admin endpoint is a certificate principal `edgectl://<role>/<id>` (roles: `operator`, `mirage`, `ghost`), read from exactly one URI SAN. Common name is ignored.
- Both admin endpoints require TLS 1.3 with verified client certificates. Plaintext is allowed only on loopback in development, and never for privileged actions.
- Default-deny authorization matrix per action. The acting identity is the TLS principal; request-body `actor` fields cannot override it.
- Each Ghost pins exactly one Mirage principal.
- Install proposals start as an intent (`ghost_id, seed_ids, proposed_at_ms, nonce`) signed by the proposing operator. Approvals are signed by a different approving operator (not Mirage), embed that signed intent, and bind the proposal, ghost, catalog steps, expiry, and a nonce. Ghost verifies both signatures, roles, separation (from the two signatures), expiry, intent age, nonce, and recomputes the steps from its own catalog.
- `deploy_ghost` deploys only manifest-configured targets with pinned SSH host keys, delivering only that Ghost's own key and certificate. No shared secrets remain.
- The root CA key stays offline; runtimes never mint certificates.

## Implementation Slices

### Slice A — Identity and PKI

- [x] Shared principal package (`internal/identity`): parse/validate `edgectl://<role>/<id>` from exactly one URI SAN; reject multiple/missing.
- [x] Switch session-link identity binding to the same parser (today `peerIdentityFromCert` prefers CN).
- [x] Cert tooling: `make certs-root` (offline), `make certs-operator NAME=`, `make certs-mirage ID=`, `make certs-ghost ID=`; ECDSA P-256, 90-day leaves, serverAuth+clientAuth, URI SAN.
- [x] Private key file-mode check at startup (refuse group/other-readable keys).
- [x] `denied_serials` revocation list checked on every handshake.

Slice A notes (2026-11-23):

- `internal/identity`: `Principal`, `ParsePrincipal`, `PrincipalFromCert` (exactly one URI SAN; CN ignored), `RequireRole`, `CheckPrivateKeyFile`, `LoadKeyPair`, `DeniedSerials` with a `VerifyPeerCertificate` hook.
- `internal/pki` + `cmd/edgecert`: root and leaf issuance for operator tooling only (no runtime imports it). Leaves are ECDSA P-256, serverAuth+clientAuth, one URI SAN, optional DNS/IP SANs, at most 90 days, random 128-bit serials; existing files are never overwritten; the CA key is loaded only if mode 0600.
- `make certs-root` writes `certs/root/ca.{crt,key}` and copies `ca.crt` to `certs/ca.crt`; leaf targets write `certs/<id>.{crt,key}`, so existing config paths (e.g. `certs/ghost.pi.crt`) keep working after re-issue. The old OpenSSL `make certs` recipe is replaced by a usage message.
- Session link: Mirage requires a ghost-role principal on the session client certificate; its id must equal the registered `ghost_id`. Ghost and Mirage load keys through the 0600 check and reject denied serials on every handshake. New config keys: ghostctl `mirage_tls_denied_serials`, miragectl `session_tls_denied_serials`.
- Migration: certificates issued by the old `make certs` (CN only, no URI SAN) no longer register on the session link. Re-issue with `make certs-root` and `make certs-ghost ID=<ghost_id>` / `make certs-mirage ID=<id> DNS=.. IP=..` (remove the old files first).
- Not in this slice: admin endpoints are still plaintext (Slice B); Ghost does not yet check the Mirage server principal (Slice B dialers).

### Slice B — Mutual TLS on both admin endpoints

- [x] Mirage admin listener: TLS 1.3, `RequireAndVerifyClientCert`.
- [x] Ghost admin listener: same.
- [x] Mirage's Ghost admin client dials with Mirage's cert and verifies the Ghost server principal matches the target ghost_id.
- [x] `client-tm` dials both endpoints with an operator cert and verifies server principals.
- [x] Development exception: plaintext only when bound to loopback; privileged actions still refused without mTLS.

Slice B notes (2026-11-23):

- `internal/adminauth`: `ServerTLSConfig` (TLS 1.3, client certs required and verified, denied serials, valid principal required, server's own cert must match its runtime principal), `ClientTLSConfig` (chain to root with serverAuth, denied serials, then an explicit server-principal check; hostname verification is replaced by the principal check, so admin addresses need no DNS/IP SANs), `Listen` (mTLS when credentials are set; plaintext only on loopback outside production), `PeerFromConn`, `Privileged`.
- Both admin handlers authenticate each connection at accept and refuse privileged actions from unauthenticated peers with `error_code=1000`. Per-action role authorization is Slice C.
- Mirage's Ghost clients pin `edgectl://ghost/<ghost_id>` for known targets. `attach_ghost_admin` without an id accepts any ghost principal, then requires the reported `ghost_id` to equal the certificate id. The Ghost spawner now takes a client so it shares the pinned TLS dialer.
- Child Ghosts: a TLS parent requires operator-issued `<child_id>.crt/.key` next to its own certificate (runtimes never mint); a plaintext parent may only spawn loopback children. Checked before spawning.
- Session security finding fixed here: Ghost's session dialer now requires a mirage-role server principal, matching the pinned `mirage_principal` when set; with TLS in production the pin is required (`ErrMiragePrincipalRequired`).
- Config: ghostctl `admin_tls_cert_file`, `admin_tls_key_file`, `admin_tls_ca_file`, `admin_tls_denied_serials`, `mirage_principal`; miragectl the same `admin_tls_*` keys (Mirage identity for serving and dialing) plus `local_ghost_admin_tls_cert_file`/`_key_file` (local Ghost inherits Mirage's CA/deny list and pins Mirage); client-tm `operator_cert_file`, `operator_key_file`, `ca_file` in either config file.
- Behavior change: without admin TLS, a local Mirage cannot dispatch commands to its Ghost (`execute_envelope` is privileged). Local development needs `make certs-root`, `make certs-mirage ID=mirage.local`, `make certs-ghost ID=ghost.local`, `make certs-operator NAME=<you>` and the config keys above.
- Still pending: the shared `admin_frame_auth_token` (removed in Slice E); per-action role authorization and actor binding (Slice C).

### Slice C — Authorization and audit

- [x] Default-deny action matrix from the contract, enforced before dispatch on both endpoints.
- [x] Actor derived from principal; reject requests whose `actor` disagrees.
- [x] Ghost `mirage_principal` pinning; optional operator access flag.
- [x] Per-request audit line: principal, role, action, decision, error_code.

Slice C notes (2026-11-23):

- `adminauth.Policy.Authorize` encodes `[authorization.mirage_admin]` and `[authorization.ghost_admin]` exactly; unknown actions are denied. Unauthenticated peers (development loopback only) may run listed non-privileged actions; privileged ones return `ErrAuthRequired`. Denials return code `1000` with reason `authorization_denied`.
- Ghost endpoint: the mirage role is honored only for the pinned `mirage_principal` (no pin = no Mirage may act); operator principals only when `admin_operator_access = true` (default off), and operators never hold mirage-only actions (`execute_envelope`, `spawn_ghost`, `bind_mirage`, `install_dependencies`). Ghost principals hold no Ghost-admin action. This closes the security review finding on `b639bd2` (authentication without role check).
- Mirage endpoint: operators only. `submit_issue` and `propose/approve/execute_dependencies` actor fields must be empty or equal the caller principal; empty is filled from the certificate (e.g. `edgectl://operator/dan`).
- `adminauth.Audit` writes `admin.audit component, peer, principal, role, action, decision, error_code, timestamp_ms` for every request on both endpoints (warn on deny).
- Config: ghostctl `admin_operator_access`; miragectl `local_ghost_admin_operator_access`. `client-tm` Ghost mode requires operator access on the target Ghost; Mirage-mode admin works with an operator certificate.

### Slice D — Signed install approvals

- [x] Canonical approval payload and ECDSA signing in `client-tm` with the operator key.
- [x] Mirage verifies and stores the signed approval; enforces approver != proposer and expiry.
- [x] Ghost verifies the approval independently (signature chain, role, separation, expiry, ghost_id, catalog, nonce replay ledger).
- [x] Remove the `admin_frame_auth_token` requirement from `install_dependencies` once approvals are verified.

Slice D notes (2026-11-23):

- `internal/approval`: `Payload` (`proposal_id, ghost_id, proposed_by, steps, expires_at_ms, nonce`), sha256 over canonical JSON, ECDSA P-256 ASN.1 signature, approver certificate carried with the approval. `Verify` checks chain to the admin root (clientAuth), operator role, signature, unexpired with lifetime <= 15 min (+1 min skew), and approver != proposer. `LoadSigner` loads only operator keys (0600).
- Mirage `approve_dependencies` requires a signed approval: verified against `admin_tls_ca_file`; signer must be the calling operator; payload must match the stored proposal exactly; recorded on the proposal and audited with proposer, approver, and a signature digest. Unsigned or mismatched approvals return `1300`.
- Ghost `install_dependencies` requires the signed approval (no shared token): verified against its own `admin_tls_ca_file`; must name this `ghost_id` and the request's proposal; only the signed steps run (listed steps must match); every step must pass policy and be an exact catalog entry; nonces are recorded until expiry and replays rejected. Without an admin CA, installs are disabled.
- `client-tm`: Mirage console > Dependencies (list, propose, approve, execute). Approve fetches the stored proposal, shows it, and signs exactly its content with the operator key (10-minute expiry, random nonce).
- A compromised Mirage can no longer forge an approval: it lacks any operator key, and Ghost verifies independently.

Slice D amendment — proposer signatures (2026-11-23, user: "Add proposer signatures"):

- Finding fixed: separation of duties compared the approver to a free-text `proposed_by` that the approver signed, so one operator could approve their own install by writing another name; Ghost could not tell. Contract `[install_approval]` amended (marked `amended` there) so every Ghost enforces two-person control itself.
- [x] `internal/approval`: `Intent` / `SignedIntent` signed by the proposer (`SignIntent`); `VerifyIntent` checks chain, operator role, signature, not denied, and age <= 24 h (not in the future). `Payload` embeds `proposer_intent` in place of `proposed_by`. `Verify(signed, Trust)` verifies both signatures, requires distinct principals, intent ghost == payload ghost, steps == `CatalogSteps(intent seed_ids)` (`ErrStepsMismatch`), and expiry. Returns both principals.
- [x] Revocation (security review finding): `Trust.Denied` rejects signer certificates whose serial is in `admin_tls_denied_serials`, for proposer and approver, at Mirage and Ghost. Applies `trust.revocation` to signatures as well as handshakes.
- [x] Mirage: `propose_dependencies` requires a signed intent; signer must be the calling operator; `proposed_by` is the verified principal; steps come only from the catalog for the intent seeds; a signed intent can open at most one proposal. `approve_dependencies` requires the embedded intent to equal the stored one. Audit records carry the intent and approval signature digests.
- [x] Ghost: verifies with its own CA, denied serials, and compiled catalog; logs proposer, approver, and both signature digests on acceptance. Step mismatch reports `ErrDependencyNotCataloged` (`1300`).
- [x] `client-tm`: propose shows ghost, seeds, catalog steps, and intent digest, then asks `[y/N]` before signing. Approve refuses non-pending proposals, verifies the intent under the operator CA, refuses self-approval, recomputes steps from its own catalog, shows the full steps, proposer, expiry, and payload digest, then asks `[y/N]` before signing. A compromised Mirage cannot get an operator to sign content other than what is shown.
- Wire change: `propose_dependencies` no longer accepts bare `ghost_id`/`seed_ids`; approvals signed before this change no longer verify (none are long-lived: 15-minute maximum).
- Replay follow-up (security review of the amendment commit): an approver could re-wrap a consumed intent in a fresh approval (new nonce) and install again without the proposer; Mirage keyed intent reuse on signature bytes, which ECDSA malleability can change. Fixes: intents are keyed by proposer principal + intent content digest (`SignedIntent.ReplayKey`) at Mirage and Ghost; Ghost records consumed intents until they age out and refuses intents proposed before it started (a restart clears the in-memory record, so pre-start intents could otherwise be reused). Cost: a proposal still pending when its Ghost restarts must be proposed again.

### Slice E — Deploy hardening and token removal

- [x] `deploy_ghost` takes a ghost_id only; host/user/port come from configured manifests; unknown targets rejected.
- [x] Pinned SSH host keys per manifest (`StrictHostKeyChecking=yes`).
- [x] Deliver the Ghost's own key/cert, the root CA cert, and the pinned Mirage principal; mode 0600; never logged.
- [x] Remove `admin_frame_auth_token` / `ghost_admin_frame_auth_token` config keys and the command-frame auth path; configs carrying them fail to load with a migration message.

Slice E notes (2026-11-23):

- `DeployGhostRequest` is `{ghost_id, force_redeploy}`. Mirage looks up the `ghost_manifests` entry; unknown ids return `1300` (`ErrUnknownDeployTarget`) before any ssh. Any host/user/manifest fields in request JSON are ignored. `binary_path` and `config_template_path` moved into the manifest (no request-supplied local paths).
- New manifest keys (miragectl `[[ghost_manifests]]`): `ssh_host_key` (`"<type> <base64>"`, single line, known types only), `tls_cert_file`, `tls_key_file`, `binary_path`. Host must be a hostname or IPv4 literal not starting with `-` (IPv6 literals not supported by the scp target syntax); user matches `[a-z_][a-z0-9_-]{0,31}`.
- Before ssh: Mirage requires admin TLS (CA to deliver, principal to pin); the leaf must load with a 0600 key, carry `edgectl://ghost/<ghost_id>`, verify under the admin CA for serverAuth and clientAuth, and not be a denied serial. Failures return `1300` (`ErrInvalidDeployManifest`).
- ssh/scp run with `StrictHostKeyChecking=yes`, a temp 0600 known_hosts holding only the pinned key, `GlobalKnownHostsFile=/dev/null`, and `BatchMode=yes` (no TOFU, no prompts).
- Remote layout: `~/.edgectl/tls` created under `umask 077` with mode 0700 before upload; a stale key file is removed first so scp cannot keep a loose mode; key chmod 600, certs 644. Generated config uses absolute paths (remote `$HOME` checked against `^/[A-Za-z0-9._/-]+$`) for `admin_tls_*` and `mirage_tls_*`, sets `mirage_principal` to Mirage's principal and `admin_tls_denied_serials` to Mirage's list; templates cannot override these and any `admin_frame_auth_token` is dropped. Logs and results carry paths and status only.
- Token removal: ghostctl `admin_frame_auth_token` and miragectl `ghost_admin_frame_auth_token` now fail config load with a migration message. Ghost admin `execute_envelope` refuses frames with an auth block (`1300`); Mirage never sends one. The session protocol's `has_auth` flag (protocol.toml, framing.md) is unchanged.
- `client-tm` Deploy Remote Ghost now asks only for ghost_id and whether to stop an existing process.

### Slice F — Verification

- [x] Negative tests per endpoint: no cert, wrong CA, expired, revoked serial, wrong role, wrong pinned Mirage, multiple URI SANs, plaintext on non-loopback.
- [x] Approval tests: forged signature, Mirage-signed instead of operator, approver == proposer, expired, replayed nonce, ghost_id mismatch, uncataloged step.
- [x] Deploy tests: unknown target rejected, request-supplied host ignored, no token or other Ghost's key in generated config.
- [x] End-to-end over real TLS sockets for both endpoints.

Slice F notes (2026-11-23):

- `internal/ghost/admin_negative_test.go` and `internal/mirage/admin_negative_test.go`: one matrix per endpoint over a real mTLS listener using the service's own handler. Each case (no client cert, wrong CA, expired, revoked serial, wrong role, wrong/peer Mirage, multiple URI SANs, plaintext client) must fail the connection or get `1000`, and the side effect (Ghost `execute_envelope`; Mirage `submit_issue`, `deploy_ghost`) must not happen; a valid principal on the same listener is the control. Plaintext on `0.0.0.0` is refused at listen time for both services.
- Test PKI helpers: `tlstest.PKI.IssueExpired`, `IssueMultiURI` (same root, so only the property under test fails).
- Approval coverage map: forged/tampered (`approval_test.go` TestVerifyRejections), Mirage certificate as signer (TestNonOperatorCannotSign), approver == proposer (approval, Mirage ledger and admin, Ghost, end-to-end), expired and stale intent (approval, Ghost), replayed nonce and re-wrapped intent (Ghost), ghost_id mismatch (approval, Ghost), uncataloged step (Ghost real catalog), denied signer (approval, Mirage, Ghost).
- Deploy coverage: `internal/mirage/ssh_deploy_test.go` (Slice E).
- `internal/ghost/e2e_admin_test.go` (package `ghost_test`): Ghost and Mirage admin endpoints on real mTLS loopback listeners with one root. Operator `ops` attaches the Ghost and proposes (signed intent); self-approval and an approval relayed by another operator are refused (`1300`); `lead` approves; execute dispatches over pinned mTLS; the Ghost verifies both signatures and runs the real catalog step (installer runner recorded, nothing installed on the host). Then: re-execute refused; operator `install_dependencies` on the Ghost refused (`1000`); the pinned Mirage replaying the consumed approval refused (`1300`).
- Seams: `Service.ServeAdmin(ctx, ln)` exported on both services (the listen path calls it; transport rules stay in `adminauth.Listen`); `internal/ghost/export_test.go` exposes the installer runner and bootstrap to `ghost_test` only.

## Status

All six slices complete (2026-11-23). Not yet exercised on separate hosts; Phase 8 Slice E (cluster validation) remains the place for that.

## Decisions (user, 2026-11-23: "Approve as drafted")

- [x] Contract approved as written and linked from `docs/index.md`.
- [x] Separation of duties: approver must always differ from proposer; no single-operator override.
- [x] Operator access to Ghost admin endpoints: off by default, enabled per Ghost in config.
- [x] Authorization denial: reuse error code `1000` with reason `authorization_denied`; `errors.toml` unchanged.
- [x] Development plaintext: allowed only on loopback listeners; privileged actions require mTLS in every mode.

## Interim Risk

- Resolved by Slice E: no shared token exists, and `deploy_ghost` reaches only configured, host-key-pinned targets.
- Slice F verified the boundary in-process over real sockets; until a multi-host run (Phase 8 Slice E), treat cross-host behavior as unverified.
