# Mirage-Ghost Framing Contract

This document defines stream framing and frame validation behavior for Mirage and Ghost control-plane links.
It is transport-agnostic above an ordered byte stream, and is expected to run over the transport contract.

## References

- Architecture contract: [`definitions/design.toml`](definitions/design.toml)
- Protocol boundary contract: [`definitions/protocol.toml`](definitions/protocol.toml)
- Canonical definitions: [`../glossary/definitions.md`](../glossary/definitions.md)
- Object and interface shapes: [`../glossary/shapes.md`](../glossary/shapes.md)
- TLV field contract: [`tlv.md`](tlv.md)

## Frame Model

Each control-plane message is encoded as one frame:

1. fixed header
2. optional auth block (when `has_auth` flag is set)
3. payload encoded as flat TLV fields

## Header Contract

Canonical header fields:

- `magic:uint32`
- `version:uint16`
- `header_len:uint16`
- `message_id:uint64`
- `message_type:uint32`
- `flags:uint32`
- `payload_len:uint64`

Canonical flags:

- `0x01 has_auth`
- `0x02 is_response`
- `0x04 is_error`

## Decoder Pipeline

- Step 1: read header bytes exactly (`header_len`).
- Step 2: validate `magic`, `version`, and declared lengths.
- Step 3: read optional auth bytes when `has_auth` is set.
- Step 4: read payload bytes exactly (`payload_len`).
- Step 5: decode TLV fields without semantic branching.
- Step 6: validate semantic required fields by `message_type`.
- Decoder pipeline diagram: [`models/framing_decoder_pipeline.mmd`](models/framing_decoder_pipeline.mmd)

## Normative Framing Rules

- Receiver MUST reject unsupported protocol `version`.
- Receiver MUST reject unknown `magic`.
- Receiver MUST reject frames with unsupported flag bits.
- Receiver MUST reject payloads above configured maximum frame size.
- Receiver MUST reject malformed TLV field lengths.
- Receiver MUST decode TLV before semantic envelope parsing.
- Semantic parser MUST ignore unknown field IDs.
- Unknown field IDs MUST be preserved as inert raw data for observability/re-encode paths.
- Unknown field IDs MUST NOT influence operation selection or execution behavior.
- Receiver MUST treat `message_id` as session-scoped unique correlation key.

## Error and Logging Contract

On framing error, receiver MUST:

- include `component`, `peer`, `direction`, `message_id` (if available), `message_type` (if available), and reason code in logs
- terminate session when stream safety cannot be guaranteed

## Timeout / Retry / Idempotency Baseline

Current framing/runtime baseline:

- per-read and per-write deadlines are enforced via session config defaults:
  - `read_timeout_ms=15000`
  - `write_timeout_ms=15000`
- malformed/partial frames are treated as stream-unsafety and terminate session
- unsupported flag bits are rejected at header decode
- unknown field IDs are ignored semantically and preserved as inert TLV data

Open integration work (Phase 6+):

- publish retry eligibility matrix by message type and failure class
- finalize replay handling guidance for duplicate session `message_id` values
- finalize command/event idempotency key requirements for end-to-end contracts
