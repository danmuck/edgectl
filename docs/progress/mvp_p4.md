# MVP Progress — Phase 4 (Ghost Execution Layer)

Status: `Done`

### Tasks

- [x] Lock Ghost execution flow in docs (`command -> seed.execute -> seed.result -> event`) and add/update a message-flow diagram in `docs/architecture/models/`
- [x] Define minimal Ghost contracts in `internal/ghost` for executor, event emitter, and execution state with required correlation fields (`message_id`, `command_id`, `execution_id`, `trace_id`)
- [x] Implement Ghost command input boundary handler for `command` envelopes with Ghost-level semantic guards
- [x] Implement deterministic execution pipeline: resolve seed, execute action, normalize `seed.result`, emit terminal `event` (`success` or `error`)
- [x] Implement in-memory execution store keyed by `execution_id` and indexed by `command_id`
- [x] Add query methods for execution correlation (`GetExecution`, `GetByCommandID`)
- [x] Add tests for success path, unknown seed, unknown action, seed error path, and correlation/state query checks
- [x] Verify acceptance: every valid command yields one valid terminal event
- [x] Verify acceptance: execution state is queryable and correlated by command/execution IDs
- [x] Update progress docs under `docs/progress/` as each Phase 4 task/check passes

### Acceptance Checks

- [x] Every valid command yields a valid event (success or error)
- [x] Execution state is queryable and correlated

## Post-Phase-4 MVP Steps

- [x] Complete the protocol/runtime conformance checklist from `docs/progress/p4_conformance_report.md` before Mirage session implementation work
- [ ] Add required local `config.toml` boot config for `ghostctl` and `miragectl` as the next task after conformance closure; each process requires colocated network-routing + identity bootstrap config, defaults support auto-connect or headless Ghost startup with later manual Mirage attach, and empty Ghost bootstrap must allow remote seed config/install transfer to bring fresh nodes online
- [x] Implement P2 step1/step2 baseline: shared session/reliability primitives and minimal Mirage endpoint (`register`, `register.ack`, `event`, `event.ack`) to prepare integration step3
- [x] Execute remaining P2 step3 resilience tests: explicit ack-timeout and replay-across-reconnect idempotency verification
- [x] Add Mirage↔Ghost session wiring (connect/register/ready) while preserving protocol/runtime boundaries
- [x] Add TLS/mTLS transport security enforcement and certificate-backed identity binding for production mode contracts
- [ ] Implement single-intent loop end-to-end (`issue -> command -> seed.execute -> seed.result -> event -> report`)
- [ ] Add failure-path tests (disconnect, timeout, duplicate IDs, validation failures) before MVP tag
