# Lab mode: one-command local stack (2026-11-24)

Status: approved by the user in thread ("Build it"); implemented 2026-11-24, uncommitted.

## Why

Testing the system by hand took a second worktree, two setup scripts, hand-edited tracked config
files and a long README. The user wants one command that brings up a working Mirage and Ghost, a
terminal front end that can add Ghosts and run commands and explains itself, and a short README.

## Goals

- `make lab` starts a Mirage and its managed Ghost on loopback with mutual TLS and opens a menu.
- From the menu: see what is running, add a Ghost, run a command and see its output, see recent
  activity, and reach the existing full admin console.
- Works in a checkout that has a real `certs/`; never reads or writes it.
- Everything the lab creates lives under `local/lab/` (gitignored) and stops on exit.

## Non-goals

- No change to canonical contracts, admin actions, or wire behavior.
- No change to real-host setup (`make run-mirage`, `make run-ghost`, `make certs-*`).
- Not a replacement for `testutil/adminlab`, which stays as the attack-style test brief.
- Dependency installs stay disabled; `seed.mongod` and `seed.docker` are not in the lab defaults.

## Approaches considered

1. **A lab mode in `client-tm` that starts the real `miragectl` binary as a child process** (chosen).
   Exercises the real runtime, keeps runtime logs out of the menu, and reuses the existing admin
   client and menus.
2. Run Mirage in-process inside `client-tm`. Fewer moving parts, but runtime logging shares the
   terminal with the menu and the code under test is no longer the shipped binary.
3. Extend the `testutil/adminlab` shell scripts. Still needs a separate checkout or a second cert
   directory convention, and leaves the front end disconnected from bring-up.

## Design

- Entry: `client-tm -mode lab` (`make lab`). It builds `miragectl` into `local/lab/bin/`, creates a
  throwaway root CA and certificates in `local/lab/certs/` (regenerated every start), writes
  `local/lab/mirage.toml`, starts Mirage with logs in `local/lab/mirage.log`, and waits until the
  managed Ghost is registered.
- Identities: `edgectl://mirage/mirage.lab`, `edgectl://ghost/ghost.local`, operators `ops` and `lead`.
  The client acts as `ops`.
- Ports: 7020 (Mirage admin), 9000 (session), 7010 (managed Ghost); a free port is chosen when one
  is taken. Added Ghosts get the next free port from 7030.
- Seeds on every lab Ghost: `seed.flow`, `seed.kv`, `seed.fs`, `seed.host`. Mirage runs with
  `local/lab/` as its working directory, so `seed.fs` files land in `local/lab/local/dir/<ghost_id>/`.
- Add a Ghost: issue a certificate for `ghost.local.<name>`, then `spawn_local_ghost`
  (admin_security.toml [authorization.mirage_admin]).
- Run a command: pick Ghost, command and arguments from Mirage's `seed_catalog`; `submit_issue` then
  `reconcile_intent`; then read the execution record from the Ghost (`execution_by_command_id`) to
  show stdout, stderr and exit code.
- Recent activity: `recent_reports` plus the latest `admin.audit` and `custody` log lines.
- Exit stops Mirage; a stale lab from a crashed client is stopped at the next start.

## Decisions made by the builder (review these)

- **Spawned child Ghosts inherit the parent's `admin_operator_access`.** A child has no config file
  of its own and already inherits seeds, Mirage pin and security mode from its parent. Without this
  the lab cannot show command output from an added Ghost. admin_security.toml says operator access
  is "disabled unless ghost config enables it"; the parent's config is the only config a child has.
- The lab enables `local_ghost_admin_operator_access` (read actions only, per the contract).
- Certificates are regenerated on every start rather than reused; nothing depends on them persisting.
- `client-tm`'s own target files for the lab are written to `local/lab/`, so the tracked
  `cmd/client-tm/a.*.toml` files are not touched.

## Outcome (2026-11-24)

Implemented in `cmd/client-tm/lab.go` and `lab_menu.go`, `make lab`. Verified by running real
sessions: start, overview, add a Ghost, `seed.kv` put/get and `seed.fs` write on the added Ghost,
`seed.host status`, recent activity, the existing intent wizard through the full console, Ctrl-C,
restart after a killed client, and a busy default port. `go test -race ./...` passes.

Building the lab exposed four existing defects, all fixed:

- **Spawn through Mirage never worked over the wire.** `mirage.SpawnGhostRequest`/`Result` had no
  JSON tags, so Ghost received an empty `spawn_ghost` request (`target_name required`). The existing
  test mocked Ghost with Mirage's own type, which hid it. Tags now match Ghost's wire names.
- **A spawned Ghost could not be sent commands.** `spawn_local_ghost` added a route but registered no
  command executor. It now attaches the child the same way `attach_ghost_admin` does.
- **Seed lock leak.** A reconcile that failed with "no executor registered" kept the seed lock, so
  later intents for that seed waited forever. The lock is now released on that path.
- **client-tm intent wizard failed under mutual TLS.** It always sent `actor = "user:client-tm"`,
  which Mirage refuses when the caller has a certificate. With an operator certificate the actor is
  now left empty.

Not covered: the Dependencies and deploy menus were not exercised through the lab.

## Persistent bootstrap and serve mode (2026-11-25)

Status: plan approved by the user in thread; implemented 2026-11-25, uncommitted.

### Why

With `api/v1` in place the user drives the lab from tracker_suite and wants to watch Mirage's
output live instead of using the menu. They also asked that the lab stop replacing its
certificates on every start: check what is there, bootstrap only what is missing or incomplete,
and keep it between sessions, for both the menu and the dev server.

This supersedes two statements above: certificates are no longer "regenerated every start", and a
free port is no longer chosen on every start (see below).

### What changed

- `client-tm -mode serve` (`make serve`): same bootstrap and build as lab mode, then `miragectl`
  runs with its output on the terminal until Ctrl-C. No menu. Its own lines use smplog like the
  runtime's. SIGINT, SIGTERM and SIGHUP stop Mirage; a stale Mirage is stopped at the next start.
- `ensureLab` replaces `prepareLab` for both modes.
  - Certificates are kept when the root loads and matches its key, `certs/ca.crt` is the same
    root, and each fixed identity (`mirage.lab`, `ghost.local`, `ops`, `lead`) has a 0600 key
    pair that chains to the root, carries its principal, and is valid for at least 24 more hours.
    Otherwise the whole PKI is replaced, as before.
  - `mirage.toml` is kept when it names the lab identities, four loopback addresses and this lab
    directory's certificate files. Other keys are left alone, so edits survive. Otherwise it is
    rewritten with the default ports (or free ones).
  - Client target files are rewritten on every start from the addresses in use.

- [x] Certificate reuse check (`labCertsValid`) with tests.
- [x] Config reuse check (`loadMirageConfig`) with tests.
- [x] `serve` mode, `make serve`, README notes.
- [x] Lab menu uses the same bootstrap.

### Decisions made by the builder (review these)

- **A kept config with a busy port is an error, not a silent move.** The start fails naming the
  address and the file. A front end is configured against fixed addresses, and rewriting the file
  would discard the user's edits. A first start still falls back to a free port.
- **Any certificate problem replaces the whole PKI**, including child Ghost certificates from the
  old root. Children are not checked on start; they are reissued when a Ghost is added.
- **The lab root key now stays on disk** (`local/lab/certs/root/ca.key`, 0600, gitignored) for up
  to 30 days instead of one session. Its certificates are valid for `localhost`/`127.0.0.1` only.
- Serve mode also writes `mirage.log` (user request, 2026-11-25): the same lines as the terminal
  with color codes removed, replaced on each start as in lab mode. Serve mode's own startup lines
  are not in it.
- Ghosts added through `spawn-local` do not survive a restart; only certificates and config persist.

### Verified

`go test ./...` passes. Against the real stack: serve start with kept certificates and config
(same certificate serial across two starts), `api/v1` status and ghosts over mutual TLS with the
audit lines appearing in the stream, Ctrl-C and SIGTERM leaving no Mirage behind, refusal when
7021 was held by another process, new PKI after removing one certificate or one key, and lab mode
through its overview and exit.

Not covered: expiry was tested with a shifted clock in unit tests, not by waiting; the lab menu's
"Add a Ghost" was not re-run against a kept root (covered by a unit test that issues from it).

### Follow-up (2026-11-25, uncommitted)

- `make serve` printed `make: *** [serve] Error 1` after Ctrl-C. The client exits 0; `go run`
  itself exits 1 when interrupted. The target now builds `local/lab/bin/client-tm` and runs it
  directly. Verified by sending SIGINT to the whole process group: no error line, no Mirage left.
- Open question from the user: the `ghost.admin client connected/disconnected ... active_clients=0`
  lines are logged at WARN but are normal. Mirage opens one short connection per Ghost admin
  request and closes it, so the count returns to 0 after each. The same pattern is in
  `mirage.admin` and `mirage.session`. Whether to lower these to INFO is undecided.
