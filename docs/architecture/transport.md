# Mirage-Ghost Transport Contract

This document defines transport-layer behavior for Mirage and Ghost control-plane links.
It builds on the architecture and protocol contracts.

## References

- Architecture contract: [`definitions/design.toml`](definitions/design.toml)
- Protocol boundary contract: [`definitions/protocol.toml`](definitions/protocol.toml)
- Canonical definitions: [`../glossary/definitions.md`](../glossary/definitions.md)
- Envelope shapes: [`../glossary/envelopes.md`](../glossary/envelopes.md)
- TLV contract: [`tlv.md`](tlv.md)

## Scope

- Applies only to Mirage<->Ghost control-plane communication.
- Does not define Ghost->Seed adapter transports.

## Normative Transport Requirements

- Transport MUST be `TCP`.
- Session model MUST be one long-lived stream connection per Ghost process instance.
- Connection direction MUST be outbound from Ghost to Mirage.
- `UDP` MUST NOT be used for Mirage<->Ghost command/event transport.
- `SSH` MUST NOT be used as the Mirage<->Ghost wire protocol.

## Security Baseline

- Development mode MAY allow non-TLS transport behind explicit configuration.
- Production mode MUST require TLS.
- Production mode MUST require mTLS and peer identity binding to `ghost_id`.
- Mirage MUST reject sessions where authenticated peer identity does not map to the declared `ghost_id`.
- Mirage MUST reject sessions before command/event flow when TLS succeeds but client certificate or identity binding validation fails.

## Session Lifecycle

- Ghost starts runtime (`appear`) first.
- Ghost performs `seed` registration preparation before serving.
- Ghost may seed an empty registry and still become ready.
- Ghost transitions to serving state (`radiate`) only after `seed`.
- Ghost MAY remain in standalone serving mode without an active Mirage session.
- Mirage accepts session, validates identity, and associates peer with `ghost_id`.
- Ghost registers seed surface through canonical registration flow (`seed`).
- After registration, session is used for command/event exchange.
- Session lifecycle diagram: [`models/transport_session_lifecycle.mmd`](models/transport_session_lifecycle.mmd)

## Failure Modes and Expected Behavior

- Dial failure:
  - Ghost logs connection failure with peer + error.
  - Ghost schedules reconnect attempt.
- TLS/auth failure:
  - Session MUST be terminated immediately.
  - Mirage logs peer identity mismatch.
- Post-connect protocol violation:
  - Receiver closes session and logs frame metadata.
- Session drop during active intent:
  - Mirage marks Ghost as unavailable for new command dispatch.
  - Reconciliation continues for unaffected Ghost peers.

## Timeout / Retry / Idempotency Baseline

Current defaults are implemented in `internal/protocol/session` and mirror
`definitions/reliability.toml`:

- `connect_timeout_ms=5000`
- `handshake_timeout_ms=5000`
- `read_timeout_ms=15000`
- `write_timeout_ms=15000`
- `heartbeat_interval_ms=5000`
- `session_dead_after_ms=15000`
- retry backoff: `initial=250ms`, `multiplier=2.0`, `max=5000ms`, `jitter=required`

Current behavior:

- Ghost reconnects with bounded backoff after dial/session loss.
- Ghost retries `event` delivery until accepted `event.ack` or `ack_timeout_ms`.
- Mirage returns idempotent `event.ack` by `event_id`.

Open integration work (Phase 6+):

- complete end-to-end delivery guarantee tables by message type
- finalize command replay/idempotency windows across reconnect boundaries
