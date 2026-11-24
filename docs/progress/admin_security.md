# Admin Boundary Security Tracker

Status: `In Progress (Slice A complete)`

Contract (approved 2026-11-23): [`docs/architecture/definitions/admin_security.toml`](../architecture/definitions/admin_security.toml)

## Why

Both admin endpoints are plaintext TCP with no caller authentication. Anyone who can reach Mirage's admin port can submit intents, approve and execute dependency installs, and run `deploy_ghost` against a host they name. Since Slice F (`0aac2bf`), that deploy also writes the shared Ghost admin token into the generated config, so a caller can obtain the one secret every Ghost uses for `execute_envelope` and `install_dependencies`. Ghost also cannot verify that a plan was approved by a person rather than just relayed by Mirage.

The goal is to replace the shared token with certificate identities end to end, not to patch individual leaks.

## Design Summary

- Every caller and server on an admin endpoint is a certificate principal `edgectl://<role>/<id>` (roles: `operator`, `mirage`, `ghost`), read from exactly one URI SAN. Common name is ignored.
- Both admin endpoints require TLS 1.3 with verified client certificates. Plaintext is allowed only on loopback in development, and never for privileged actions.
- Default-deny authorization matrix per action. The acting identity is the TLS principal; request-body `actor` fields cannot override it.
- Each Ghost pins exactly one Mirage principal.
- Install approvals are signed by the approving operator's key (not Mirage's), bound to the proposal, ghost, steps, expiry, and a nonce. Approver must differ from proposer. Ghost verifies the signature, role, separation, expiry, nonce, and catalog membership.
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

- [ ] Mirage admin listener: TLS 1.3, `RequireAndVerifyClientCert`.
- [ ] Ghost admin listener: same.
- [ ] Mirage's Ghost admin client dials with Mirage's cert and verifies the Ghost server principal matches the target ghost_id.
- [ ] `client-tm` dials both endpoints with an operator cert and verifies server principals.
- [ ] Development exception: plaintext only when bound to loopback; privileged actions still refused without mTLS.

### Slice C — Authorization and audit

- [ ] Default-deny action matrix from the contract, enforced before dispatch on both endpoints.
- [ ] Actor derived from principal; reject requests whose `actor` disagrees.
- [ ] Ghost `mirage_principal` pinning; optional operator access flag.
- [ ] Per-request audit line: principal, role, action, decision, error_code.

### Slice D — Signed install approvals

- [ ] Canonical approval payload and ECDSA signing in `client-tm` with the operator key.
- [ ] Mirage verifies and stores the signed approval; enforces approver != proposer and expiry.
- [ ] Ghost verifies the approval independently (signature chain, role, separation, expiry, ghost_id, catalog, nonce replay ledger).
- [ ] Remove the `admin_frame_auth_token` requirement from `install_dependencies` once approvals are verified.

### Slice E — Deploy hardening and token removal

- [ ] `deploy_ghost` takes a ghost_id only; host/user/port come from configured manifests; unknown targets rejected.
- [ ] Pinned SSH host keys per manifest (`StrictHostKeyChecking=yes`).
- [ ] Deliver the Ghost's own key/cert, the root CA cert, and the pinned Mirage principal; mode 0600; never logged.
- [ ] Remove `admin_frame_auth_token` / `ghost_admin_frame_auth_token` config keys and the command-frame auth path; configs carrying them fail to load with a migration message.

### Slice F — Verification

- [ ] Negative tests per endpoint: no cert, wrong CA, expired, revoked serial, wrong role, wrong pinned Mirage, multiple URI SANs, plaintext on non-loopback.
- [ ] Approval tests: forged signature, Mirage-signed instead of operator, approver == proposer, expired, replayed nonce, ghost_id mismatch, uncataloged step.
- [ ] Deploy tests: unknown target rejected, request-supplied host ignored, no token or other Ghost's key in generated config.
- [ ] End-to-end over real TLS sockets for both endpoints.

## Decisions (user, 2026-11-23: "Approve as drafted")

- [x] Contract approved as written and linked from `docs/index.md`.
- [x] Separation of duties: approver must always differ from proposer; no single-operator override.
- [x] Operator access to Ghost admin endpoints: off by default, enabled per Ghost in config.
- [x] Authorization denial: reuse error code `1000` with reason `authorization_denied`; `errors.toml` unchanged.
- [x] Development plaintext: allowed only on loopback listeners; privileged actions require mTLS in every mode.

## Interim Risk (until Slice E lands)

- Do not set `ghost_admin_frame_auth_token` on a Mirage whose admin port is reachable by untrusted callers; `deploy_ghost` can write it to a caller-chosen host.
- Keep both admin listeners bound to loopback or a trusted network segment.
