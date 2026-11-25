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
