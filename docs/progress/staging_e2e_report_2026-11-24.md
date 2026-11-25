# EdgeCTL staging — end-to-end functional & security test

- **Tested commit:** `951267f` (`origin/staging`, identical to local `dev` HEAD)
- **Date:** 2026-11-24 (EST)
- **Environment:** fresh clone in a throwaway dir, loopback only, Go 1.27.1, OpenSSL 3.6.5, macOS. The admin lab (`testutil/adminlab`) and a clone-only production-mode variant.
- **Contract of record:** `docs/architecture/definitions/admin_security.toml`, `transport_security.toml`, `handshake.toml`, `tlv.toml`, `protocol.toml`, `errors.toml`.
- **Scope note:** this working copy cannot host the lab — `certs/` holds a real PKI and `testutil/adminlab/setup.sh` refuses it by design. All execution was in a separate clone. Nothing with real host side effects was triggered: `seed_install_enabled=false` throughout, no `deploy_ghost` against a real manifest, no `seed.docker`/`seed.mongod` on live endpoints, no `make update-pi`.

---

## 1. What the system is and what it accomplishes

EdgeCTL is a certificate-identity control plane for driving operations onto remote hosts through a strict chain of custody. Verified end to end:

- **Mirage** (`cmd/miragectl`) — control plane. Owns desired/observed state, reconciliation, reports, the dependency catalog and approval ledger, remote deploy, and one managed in-process Ghost. Session listener `:9000`; admin listener (mTLS JSON) e.g. `127.0.0.1:7020`.
- **Ghost** (`cmd/ghostctl`) — per-host execution gateway. Routes commands to seeds, emits events, keeps an idempotency ledger. Dials out to Mirage; exposes its own mTLS admin endpoint.
- **Seeds** — service adapters (`seed.flow`, `seed.fs`, `seed.kv`, `seed.mongod`, `seed.docker`, `seed.host`).
- **edgecert** — the only certificate issuer; runtimes never mint certs.

**Custody chain (observed working):** operator `submit_issue` → Mirage plans commands → `reconcile_intent` dispatches a `command` to a Ghost → seed executes → `event` back → Mirage acks and emits a `report`. A multi-stage, multi-Ghost intent ran to `satisfied` with correct per-stage progress, seed-lock serialization, terminal states (`complete`/`failed`/`retry_exhausted`), and `trace_id=trace.<intent_id>` / `request_id` correlation on every custody line.

**Reliability (observed):** command retry with a Ghost down → `retry_exhausted after 3 attempt(s)`, clean reconnect on restart; duplicate `event_id` deduped; failed/unknown operations surface as `failed` rather than crashing.

**Two-operator dependency installs (observed):** proposer signs an intent, a different operator signs an approval embedding it; Mirage and the Ghost verify both signatures independently and the Ghost recomputes steps from its own catalog. Full flow exercised to the Ghost's disabled-install gate (`1400`). The real install path is covered by `internal/ghost/e2e_admin_test.go` with a recording runner.

### Verification method per area
| Area | How verified |
|---|---|
| Build, vet, unit + `-race` | Run in the clone: `go build/vet ./...`, `go test -race -count=1 ./...` — **24/24 packages pass** |
| Custody chain, reliability, seeds, approvals (functional) | Live, against the running lab |
| Identity/transport/authz/approval/session attacks | Live, with forged certs and a clone-only raw client |
| Decoder robustness | `go test -fuzz`, 45–60 s each |
| `seed.docker`/`seed.mongod` host effects | Code review + existing fake-runner tests only (not run live) |
| `deploy_ghost` SSH | Code review + `internal/mirage/ssh_deploy_test.go` only (no real SSH) |
| `client-tm` TUI | Unit tests only (interactive; rewrites tracked TOMLs on exit) |
| Cross-host / Proxmox (Phase 8 Slice E) | **Not exercised** — single-host lab only |

---

## 2. Boundary held (no finding)

Every documented control I attacked rejected the attack with the contract's error code. Highlights, all reproduced live:

- **Identity:** certs with 0 or 2 URI SANs, wrong scheme/role, uppercase/percent-encoded/trailing-slash/query/fragment/userinfo/port ids, role-in-CN, expired, not-yet-valid, wrong EKU (serverAuth-only), another root, a lab-root-signed intermediate chain, and a leaf signed by a non-CA leaf — **all dropped at the TLS/identity layer** (`identity.PrincipalFromCert`, `ParsePrincipal`). A `noeku`/`pct`/`upper-scheme` cert that *looked* odd but was a valid single-SAN operator was correctly accepted.
- **Transport:** TLS 1.2 and plaintext to either mTLS admin port → rejected (`protocol version not supported` / connection dropped); no client cert → `tls: certificate required`; session `:9000` is not plain TLS-probeable.
- **Authorization matrix:** every action × {ops, lead, a fresh operator, mirage, ghost.lab, ghost.local, none} on both endpoints matches `admin_security.toml [authorization.*]` exactly — operator-only on Mirage; mirage-or-operator reads vs. mirage-only privileged on Ghost; a non-pinned mirage (`mirage.evil`) denied `1000 is not the pinned mirage`; operator denied on the Ghost (`operator access disabled`); unknown action → `1000`.
- **Approvals:** self-approval, approver≠caller, lifetime >15 m, intent >24 h or future-dated beyond skew, swapped `ghost_id`/`proposal_id`/steps, added/removed steps, intent swapped in from another proposal, tampered signature byte, swapped approver cert, and an **ECDSA-malleated (s→n−s) re-proposed intent** — **all `1300`**. The malleation was additionally caught by the content-digest replay key (`intent already proposed`), confirming `ReplayKey` defeats signature re-encoding. Separation of duties, single-use intents, and exact catalog-step matching all hold.
- **Compromised-Mirage (`mirage.lab` cert direct to Ghost):** `install_dependencies` without a two-operator approval → `1400`/disabled; `execute_envelope` carrying an auth block (old shared-token shape) → refused `1300`; unknown operation / unloaded seed → execution `error`, not shell; hostile command frames with duplicate TLV fields and unknown field IDs did **not** alter operation selection (unknown fields stay inert).
- **Session protocol:** bad magic/version, unknown flag bit (`1100`), `header_len<32`, truncated/oversize frames, wrong message type, and an event bearing a **foreign `ghost_id`** → `ack rejected code=1000, session closed`. Oversized (300 KiB) and HTTP-garbage registration lines rejected.
- **Removed shared token:** configs containing `admin_frame_auth_token`/`ghost_admin_frame_auth_token` **fail to load** with a pointer to the contract; the admin `execute_envelope` path refuses frames with an auth block. No live path accepts or emits the token.
- **Production-mode config:** `session_security_mode=production` without TLS/mTLS, admin plaintext on a non-loopback bind, a Ghost with no `mirage_principal`, and a `0644` key file **all refuse to start**. mTLS session link verified working (ghost.lab registered over mTLS, foreign-ghost event still rejected).

---

## 3. Findings

Severity is relative to the contract and the documented threat model (DoS and host security are explicitly out of scope).

### F1 — `submit_issue` silently overwrites another operator's intent (no owner check)
- **Severity:** medium
- **Contract:** `admin_security.toml [authorization]` actor binding; no stated owner model for intents.
- **Component:** `internal/mirage/orchestration.go:245` (`o.desired[issue.IntentID] = …`) and `:251` (`delete(o.observed, issue.IntentID)`), reached from `server.go:266`.
- **Observed:** operator `lead` re-submitted `intent.e2e.1` (already `satisfied`, owned by `ops`) with a different objective and command args. Accepted; `Desired.Actor` flipped to `lead`, observed state cleared, intent reset to pending. Any operator can clobber or reset any in-flight intent by id.
- **Expected:** a second `submit_issue` for an existing `intent_id` owned by a different actor should be rejected or versioned, not silently replaced.
- **Status: fixed in the working tree (uncommitted), 2026-11-24.** `Orchestrator.SubmitIssue` refuses an issue for an existing `intent_id` whose recorded actor differs (`ErrIntentActorMismatch`, admin `error_code` 1000, nothing changed). The same actor may still re-submit. Regression test: `TestSubmitIssueRejectsReplacementByAnotherActor`. Live: `lead` re-submitting `ops`' intent is refused; `ops` re-submit and reconcile still work.
- **Open for review:** the contracts define the actor as the TLS principal but no intent owner model. Reject-by-other-actor is the narrowest rule; there is no supersede/transfer path, and an intent is owned for the life of the Mirage process. In development plaintext-loopback mode the actor is unauthenticated, so this check adds nothing there. Code 1000 was chosen to match authorization denials; `errors.toml` does not name this case.
- **Fix:** reject re-submit when `intent_id` exists with a different actor (or require an explicit supersede flag); record owner on the desired entry.

### F2 — `seed.fs` path confinement is lexical; a symlink escapes the root
- **Severity:** medium (high where `seed.fs` is exposed to a less-trusted operator)
- **Contract:** execution safety / seed sandboxing (`protocol.toml [envelopes.execution_safety]`).
- **Component:** `internal/seeds/fs/seed.go:186` (`resolvePath` → `filepath.Clean`+`isWithin`); no symlink resolution before the write at `:124`.
- **Observed:** `..`/absolute paths are correctly blocked (`path escapes root`). But a symlink *inside* the root pointing outside it is followed: I created `local/dir/ghost.local/link -> <outside dir>` and `seed.fs write link/pwn.txt` landed the file outside the seed root.
- **Expected:** writes/reads must not escape the root via symlinks.
- **Status: fixed in the working tree (uncommitted), 2026-11-24.** `write`, `read` and `delete` now go through an `os.Root` handle on the seed root (`Seed.openRoot`), so any path component or symlink that leaves the root is refused by the OS layer rather than by a string check; the lexical check is kept as a first gate. Regression tests: `TestSeedRefusesSymlinkEscape`, `TestSeedRejectsLexicalEscapeAndDeleteIsIdempotent`.
- **Notes:** symlinks that stay inside the root still work; the configured root itself may be a symlink. `list` is unchanged (it does not follow directory symlinks and returns names only). Not replayed live — the stock lab has no `seed.fs`; covered by the unit tests.
- **Fix:** resolve with `filepath.EvalSymlinks` (or `openat2`/`O_NOFOLLOW` semantics) and re-check containment on the resolved path; refuse symlinked components.

### F3 — Mirage accepts an unsolicited session event as proof a planned command completed
- **Severity:** medium (amplified to high in development session mode)
- **Contract:** `handshake.toml`/`transport_security.toml` binding; custody model in `protocol.toml`.
- **Component:** `internal/mirage/server.go:334` (`ingestEvent`) → `orchestration.go:468` — a matching `command_id` for a *planned* command is marked complete/satisfied from the event alone, with no check that Mirage ever dispatched that command.
- **Observed:** I submitted an intent planned for `ghost.lab`, never called reconcile (so nothing was dispatched), then from a session registered as `ghost.lab` sent a `success` event for that command_id. Mirage marked the command `complete` and reported `satisfied`; `ghost.lab` had never executed it. In the lab's **development** session mode there is no session mTLS, so any loopback peer can self-declare `ghost_id` and forge completion of any operator's intent.
- **Expected:** an event should only advance a command Mirage actually dispatched to that Ghost; the ghost-id binding check at `server.go:334-339` does not cover the never-dispatched case.
- **Status: fixed in the working tree (uncommitted), 2026-11-24.** The orchestrator records each `command_id` when it is handed to an executor (`Orchestrator.markDispatched`, set in `ReconcileOnce` before the first attempt; cleared for an intent when `SubmitIssue` replaces its plan). `ingestObservedEvent` returns `ErrCommandNotDispatched` for a planned command that was never sent, and `Server.ingestEvent` rejects it like a binding mismatch: rejected `event.ack` code 1000, nothing observed, no stored ack, session closed. Events arriving mid-retry or after `retry_exhausted` are still accepted. Regression test: `TestEventForPlannedButUndispatchedCommandIsRejected` (`internal/mirage/binding_test.go`). Verified: `go test -race ./...` 24/24, and the original live attack replayed against a patched lab build is rejected while real dispatch still completes.
- **Open question for review:** the contract does not name an ack code for this case; 1000 (same as binding mismatches) was used so no new wire behavior is introduced. `errors.toml` could state it explicitly.
- **Original suggested fix:** gate `ingestEvent` on a dispatched-command record (track outstanding `command_id`s per session); ignore/alert on events for commands not in flight. The docs already flag "the direct reconcile path trusts the executor's event" — extend that to the session ingest path.

### F4 — The security audit trail is suppressible by a log-level env var
- **Severity:** medium
- **Contract:** `admin_security.toml [audit]` — "every admin request writes an `admin.audit` line"; `observability.toml`.
- **Component:** `internal/adminauth/authorize.go:154-157` (audit emitted via `logs.Warn`/`logs.Info`); gated by `internal/logging/config.go:62-75` env overrides; `internal/observability/event.go:176` discards the write error (`_ = l.Write(e)`).
- **Observed:** with `EDGECTL_LOG_LEVEL=error`, allowed and denied admin requests were served normally but **zero `admin.audit` lines were written**. `EDGECTL_LOG_BYPASS` and `=off` do the same. Custody emit also swallows write errors.
- **Expected:** audit is a security record, not debug output; it must not be silenceable by an operational log setting, and a failed audit write should be visible (ideally fail-closed for privileged actions).
- **Status: fixed in the working tree (uncommitted), 2026-11-24.** Audit records now go through `adminauth.Record` / `Recordf` (new `internal/adminauth/audit.go`). When the configured level passes the record it is logged exactly as before; when the level would drop it (`EDGECTL_LOG_LEVEL=warn|error|off`), the line is written directly to the log destination prefixed `AUDIT `. A failed direct write is reported on stderr and returned. Routed through it: `admin.audit` (`authorize.go`), `mirage.dependency.audit` (`dependency_proposals.go`), `ghost.dependency.approval` (`ghost/dependencies.go`). `observability.Emit` now reports a custody write error (`logs.Errf`) instead of discarding it. Regression tests: `internal/adminauth/audit_test.go` (3 tests), `TestEmitReportsWriteFailure` (`internal/observability/event_test.go`). Verified: `go test -race ./...` 24/24; a patched lab build started with `EDGECTL_LOG_LEVEL=error` wrote `admin.audit` lines for allowed and denied requests on both Mirage and Ghost (previously zero).
- **Correction to the original observation:** `EDGECTL_LOG_BYPASS` does not suppress audit lines; it only switches output to raw JSON (`internal/logging/config.go`, `third_party/smplog`). Only the level setting suppressed them.
- **Deliberately not changed (open for review):**
  - Custody lines (`custody ...`) remain governed by the log level. `observability.toml` treats them as structured logs, and no contract clause makes them level-independent; only the discarded write error was fixed.
  - No fail-closed behaviour: a request is still served if its audit write fails (the failure is reported on stderr). `admin_security.toml [audit]` does not specify refusing the request, so this was not assumed.
  - The `AUDIT ` prefix on directly written lines is an implementation detail, not contract text; the `admin.audit ...` field layout is unchanged.
  - The default custody logger cannot itself fail (smplog returns no error), so the new error report only fires for a replaced `Logger`.
- **Original suggested fix:** route `admin.audit` (and custody) through a dedicated sink independent of `EDGECTL_LOG_LEVEL`; treat audit-write failure as an error, not a discard.

### F5 — Decoder round-trip asymmetry: a command frame decodes but fails to re-encode
- **Severity:** low (robustness/contract hygiene; not exploitable observed)
- **Contract:** `framing.md` / `tlv.md` encode/decode symmetry; unknown-field preservation.
- **Component:** `internal/protocol/session/command_report_wire.go:105` (`DecodeCommandFrame`) vs `:26` (`Command.Validate` via `EncodeCommandFrame`).
- **Observed:** a 60 s fuzz of `frame.ReadFrame`→`Decode*`→`Encode*` found an input that passes `schema.Validate(MsgCommand)` and decodes, but the resulting `Command` fails `Validate` on re-encode (`command missing command_id`) — i.e. `schema.Validate` and `Command.Validate` disagree on required fields (an empty-but-present `command_id` passes one, not the other). `FuzzTLV` and `FuzzRegistration` found no crash and TLV round-trips cleanly.
- **Expected:** decode and encode should agree on validity; a decoded envelope should always re-encode.
- **Status: fixed in the working tree (uncommitted), 2026-11-24.** `DecodeCommandFrame`, `DecodeEventFrame`, `DecodeEventAckFrame` and `DecodeReportFrame` now run the same `Validate()` the encoders use; a blank required field is returned as a `schema.ValidationError` (code 1300). `schema.Validate` itself is unchanged, so no other message type is affected. Regression test: `TestDecodeRejectsBlankRequiredFields`.
- **Fix:** make `schema.Validate` reject empty required string values (or have both paths share one required-field check).

### F6 — No post-handshake deadline or line-size limit on admin connections
- **Severity:** low (DoS is out of scope, but worth noting)
- **Component:** `internal/mirage/admin_control.go:175-177`, `internal/ghost/admin_control.go:269-271` — `bufio.ReadBytes('\n')` with no cap and no read/idle deadline after the TLS handshake.
- **Observed:** an authenticated operator connection held idle 35 s was still served; a single 96 MiB line (no newline) grew Mirage RSS to ~116 MiB, all reclaimed on close. A deeply-nested JSON body is bounded (`exceeded max depth`, good). 20 session connections each pinning an 8 MiB-claimed header frame were each closed by the 15 s read deadline; RSS returned to baseline. Bounded by deadlines/GC, but an authenticated admin line is unbounded in memory while being read.
- **Status: fixed in the working tree (uncommitted), 2026-11-24.** Both admin endpoints read requests through `adminauth.ReadRequestLine` (new `internal/adminauth/request.go`): a 16 MiB line cap and a 5-minute idle read deadline. An oversize line gets one response with `error_code` 1101 and the connection is closed. Tests: `internal/adminauth/request_test.go`. Live: a 17 MiB line on Mirage and on Ghost is refused with 1101; the endpoint keeps serving.
- **Open for review:** neither number is in a contract. 16 MiB is sized to fit an `execute_envelope` carrying a maximum 8 MiB frame base64-encoded. The idle deadline closes a `client-tm` connection left idle for 5 minutes; its next request fails once and the one after reconnects (client-tm was not changed).
- **Fix:** cap the admin request line length and set a read/idle deadline on admin connections, as the session listener does.

### F7 — Stale token variables in `.env` (informational)
- **Severity:** info
- **Component:** `.env` (gitignored) still defines `MIRAGE_TOKEN` / `GHOST_TOKEN`; `Makefile:104-109` rsyncs `.env` (and `certs/`) to the Pi via `make update-pi`.
- **Observed:** no Go code reads these; the token mechanism is genuinely gone. They are leftover secrets that `update-pi` would copy off-host.
- **Status: partly done, 2026-11-24.** `MIRAGE_TOKEN` and `GHOST_TOKEN` removed from the local `.env` (gitignored; nothing in the repo read them). `MIRAGE_ADDR`/`GHOST_ADDR` left in place. `update-pi` (user-approved, 2026-11-24) no longer sends `.env`, `certs/ca.key`, `certs/ca.srl` or `certs/root/`; the Makefile now uses `-include .env` so it still loads on a host without one. Verified by `make -n update-pi` and a local rsync dry run; not run against the Pi.
- **Still to do by hand:** copies of `ca.key` and `.env` already on the Pi are not removed by the new rsync (excluded files are never deleted on the receiver). `certs/mirage.mac.key` is now excluded as well (user-approved); any copy already on the Pi also needs removing by hand.
- **Fix:** delete the stale vars; reconsider rsyncing `.env`/`certs/` (root key included) in `update-pi`.

---

## 4. Accepted limitations confirmed (not findings)

Per the brief, these are permitted by the contract and were confirmed to behave as documented:
- Plaintext admin on loopback in **development** runs non-privileged actions unauthenticated (`status`, `registered_ghosts`, `submit_issue`, `reconcile_intent`, `attach_ghost_admin`, `propose_dependencies`, and `ghost.local status`); privileged actions (`deploy_ghost`, `approve/execute_dependencies`, `spawn_ghost`) are refused `authenticated peer required`. *(Note the tension with F3: dev-mode no-mTLS is what amplifies the forged-event issue; worth revisiting whether `submit_issue` belongs in the unauthenticated set.)*
- Replay records are in-memory; intents proposed before a Ghost started are refused after restart.
- IPv6 deploy hosts rejected; session `has_auth` frame flag retained; `brew` installs host-global.

---

## 5. Recommendations (suggested order)

1. **F3** — gate event ingest on dispatched commands. This is the one that lets state be forged; highest custody-integrity value.
2. **F4** — make audit independent of log level and fail-visible. Cheap, and everything else depends on the audit trail being trustworthy.
3. **F2** — resolve symlinks in `seed.fs` before the containment check.
4. **F1** — add an owner check / supersede semantics to `submit_issue`.
5. **F5, F6, F7** — reconcile the two validators; cap/deadline admin lines; clean `.env`.
6. **Phase 8 Slice E (cross-host/Proxmox) remains entirely unverified** — none of the multi-host behavior was exercisable in a single-host loopback lab. Doc inconsistencies also remain: `progress/admin_security.md` header says "Slices A–D" while the body covers A–F; `observability.toml` still marks the Phase 8 logging target pending though Slice D is done.

## 6. Coverage / what was not reached
- Covered: static baseline, full functional custody + reliability, the complete admin security boundary (identity, transport, authz, approvals, compromised-Mirage, session framing, removed token), production-mode config enforcement, decoder fuzzing, bounded resource probes, audit completeness.
- Not reached: real cross-host deploy/SSH (fake-runner + review only), `seed.docker`/`seed.mongod`/`seed.host` live host effects (review only), `client-tm` interactive TUI (unit tests only), multi-host cluster (Phase 8 Slice E), sustained DoS/load (out of scope).

---

## Follow-up: local bring-up guide (2026-11-24)

README "Local Development" was rewritten as a step-by-step guide; every command in it was run against a fresh clone of `e0e97fd`. Two things found while verifying it:

- **The plaintext stack cannot execute commands.** With the default `make run-mirage` / `make run-ghost` configs (no admin TLS), status and catalog reads work and `submit_issue` is accepted, but reconcile ends `failed`: `execute_envelope` and `bind_mirage` are privileged and refused with 1000. This matches `admin_security.toml`; the README now says so and points to the certificate lab for end-to-end runs.
- **No Mirage example config is tracked.** `cmd/*/config.toml` is gitignored and only `cmd/ghostctl/ex.config.toml` exists, so a fresh clone cannot run `make run-mirage` without hand-writing a config. Fixed: added `cmd/miragectl/ex.config.toml` (plaintext loopback by default, commented admin mTLS block); verified in a fresh clone in both modes.
