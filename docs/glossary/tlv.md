# EdgeCTL TLV Protocol Guide

This file documents how to implement and use the TLV protocol contract defined in [`../architecture/definitions/tlv.toml`](../architecture/definitions/tlv.toml).
It intentionally avoids duplicating numeric/type tables from TOML.

## References

- Canonical TLV contract: [`../architecture/definitions/tlv.toml`](../architecture/definitions/tlv.toml)
- Wire/header contract: [`../architecture/definitions/protocol.toml`](../architecture/definitions/protocol.toml)
- Canonical envelope vocabulary: [`definitions.md`](definitions.md)
- Envelope shapes: [`envelopes.md`](envelopes.md)

## Purpose

- `definitions/tlv.toml` is the source of truth for:
  - primitive type IDs
  - message type IDs
  - field ID/type mapping
  - required fields per envelope
  - decoder/parser rules
- This guide defines runtime behavior expected by Mirage and Ghost when applying that contract.

## Runtime Model

- The frame format (`header + optional auth + payload`) is defined in `definitions/protocol.toml`.
- The payload is a flat sequence of TLV fields.
- `message_type` in the frame header selects semantic validation rules from `definitions/tlv.toml`.

## Encoder Behavior

- Encoder MUST use field IDs and types exactly as defined in `definitions/tlv.toml`.
- Encoder MUST include all required fields for the selected message type.
- Encoder SHOULD include common correlation fields whenever available.
- Encoder MUST emit each TLV field as:
  - `field_id:uint16`
  - `type:uint8`
  - `length:uint32`
  - `value:[]byte`
- Encoder MAY include additional non-required fields if they use defined IDs or extension IDs reserved by future policy.

## Decoder Behavior

- Decoder MUST parse TLV fields without branching on `message_type` during binary decode.
- Decoder MUST reject malformed field lengths.
- Decoder MUST produce a raw field map/list before semantic validation.
- Decoder SHOULD preserve unknown fields in raw form for observability and re-encode paths.

## Semantic Validation Behavior

- Semantic parser MUST branch on header `message_type` after decode.
- Semantic parser MUST enforce the required field set for that message type from `definitions/tlv.toml`.
- Semantic parser MUST type-check each required field against its declared primitive type.
- Semantic parser MUST ignore unknown fields for forward compatibility.
- Semantic parser MUST return deterministic validation errors for missing required fields or type mismatch.

## Correlation and Traceability

- Implementations SHOULD propagate these correlation fields end-to-end when relevant:
  - `intent_id`
  - `command_id`
  - `execution_id`
  - `event_id`
  - `phase`
  - `timestamp_ms`
- Log records SHOULD include `message_type` and key correlation IDs to reconstruct custody flow.

## Compatibility Rules

- Unknown fields: decode and ignore semantically unless promoted by a newer contract version.
- Unknown flags: follow protocol-level behavior from `definitions/protocol.toml`.
- Reuse of existing field IDs for different meanings in the same version is NOT allowed.
- New fields MUST be additive and MUST NOT break older required field sets.

## Error Handling Expectations

- Malformed TLV binary: reject frame at decode stage.
- Missing required field: reject at semantic validation stage.
- Wrong primitive type for known field: reject at semantic validation stage.
- Unknown field ID: do not fail solely for unknown field.

## Implementation Checklist

- [ ] Load and parse `definitions/tlv.toml` as contract input for tests and generated constants.
- [ ] Generate or maintain a single constant source for message types, field IDs, and primitive types.
- [ ] Implement decode -> semantic-validate as two explicit steps.
- [ ] Add test vectors for each message type required field set.
- [ ] Add malformed length/type mismatch/missing-field negative tests.
- [ ] Add compatibility tests covering unknown-field ignore behavior.
