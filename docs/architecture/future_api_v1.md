# Future API v1 (Proposal)

Status: `Proposed (future_)`

This document defines a proposed northbound `api/v1` surface for User/Operator to Mirage control.
It is a proposal artifact and does not replace canonical contracts under `architecture/definitions/*.toml`.

## Purpose

- Provide a stable external control boundary for intent ingestion, reconcile triggers, and read models.
- Keep Mirage<->Ghost envelope transport (`command`, `event`, `event.ack`) internal to the control plane.
- Preserve chain-of-custody correlation from user intent through reconciled report.

## Scope

- `api/v1` owns user-facing orchestration APIs.
- Mirage remains authority for desired/observed state and report emission.
- Ghost remains authority for command execution and seed dispatch.

## Non-Goals

- No public endpoint for direct raw `seed.execute` operations.
- No bypass of Mirage reconcile authority.
- No change to binary framed TLV protocol between Mirage and Ghost.

## Endpoint Surface (Proposed)

| Method | Path | Purpose | Contract Mapping |
|---|---|---|---|
| `GET` | `/api/v1/status` | Control-plane health and identity status | runtime status |
| `POST` | `/api/v1/intents` | Submit staged issue intent | `issue` envelope |
| `GET` | `/api/v1/intents` | List known intent IDs | intent index |
| `GET` | `/api/v1/intents/{intent_id}` | Snapshot desired/observed state for one intent | snapshot/read model |
| `POST` | `/api/v1/intents/{intent_id}/reconcile` | Trigger single-intent reconcile loop | `reconcile_intent` |
| `POST` | `/api/v1/reconcile` | Trigger reconcile for all intents | `reconcile_all` |
| `GET` | `/api/v1/reports` | Read bounded report history | `report` boundary |
| `GET` | `/api/v1/ghosts` | Read connected ghost inventory | `registered_ghosts` |
| `GET` | `/api/v1/routing` | Read ghost routing table | `routing_table` |
| `GET` | `/api/v1/services` | Read available seed/service view | `available_services` |
| `POST` | `/api/v1/ghosts/attach` | Attach remote ghost admin route | `attach_ghost_admin` |
| `POST` | `/api/v1/ghosts/spawn-local` | Spawn Mirage-managed local ghost | `spawn_local_ghost` |

## Request and Response Shape

All write requests should include:

- `request_id` (required)
- `trace_id` (optional if server-derived)
- idempotency key (`intent_id` for intent submission)

Success envelope:

```json
{
  "ok": true,
  "request_id": "req.123",
  "trace_id": "trace.123",
  "timestamp_ms": 1760000000000,
  "data": {}
}
```

Error envelope:

```json
{
  "ok": false,
  "request_id": "req.123",
  "trace_id": "trace.123",
  "timestamp_ms": 1760000000000,
  "error": {
    "class": "transport|framing|tlv_decode|semantic|runtime",
    "code": 1300,
    "message": "semantic validation failure",
    "retryable": false
  }
}
```

## Reliability, Retry, and Idempotency

- `POST /api/v1/intents` is idempotent by `intent_id`.
- Reconcile endpoints are retry-safe when no new terminal event has changed intent state.
- Retry/backoff behavior should map to canonical defaults in `reliability.toml`.
- Duplicate event ingestion must remain deduplicated by `event_id`.
- `event.ack` behavior remains internal and idempotent by `event_id`.

## Failure Modes

- Validation error (`semantic`) -> reject request, no state mutation.
- Runtime dispatch failure (`runtime`) -> return error, keep intent visible for re-reconcile.
- Transport/session fault (`transport`) -> report recoverable/unrecoverable based on policy and timeout window.
- Duplicate submissions -> return existing resource state, no duplicate command mutation.

## Observability Requirements at API Edge

Required response/log correlation fields:

- `component`
- `peer`
- `direction`
- `trace_id`
- `request_id`
- `message_id`
- `message_type`
- `timestamp_ms`
- optional custody fields: `intent_id`, `command_id`, `execution_id`, `event_id`, `ack_status`

## Identity and Addressing

- `intent_id` is the authoritative idempotency key for issue ingest.
- `ghost_id` and `seed_selector` identify command execution locality and service target.
- `command_id`, `execution_id`, and `event_id` preserve custody across boundaries.

## Associated Models

- `architecture/models/future_api_v1_boundary.mmd`
- `architecture/models/future_api_v1_issue_reconcile_flow.mmd`

## Canonical References

- `architecture/definitions/design.toml`
- `architecture/definitions/protocol.toml`
- `architecture/definitions/tlv.toml`
- `architecture/definitions/reliability.toml`
- `architecture/definitions/errors.toml`
- `architecture/definitions/observability.toml`
