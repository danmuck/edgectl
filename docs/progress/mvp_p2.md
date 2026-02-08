# MVP Progress — Phase 2 (Protocol Core)

Status: `Done`

### Tasks

- [x] Implement fixed header + optional auth + TLV encoder/decoder
- [x] Implement semantic schema validation layer
- [x] Define schemas for all boundary envelopes
- [x] Add tests: round-trip, malformed input, unknown fields, missing required fields

### Acceptance Checks

- [x] Protocol tests pass independently of Mirage/Ghost runtime
- [x] Unknown fields preserved/ignored per contract
- [x] Invalid wire data fails deterministically

## Baseline Contracts (Locked)

- [x] Lock canonical vocabulary: `issue`, `command`, `seed.execute`, `seed.result`, `event`, `report`
- [x] Lock authority model: Mirage = desired/observed, Ghost = execution, Seed = service interface
- [x] Lock required envelope fields and message type IDs
- [x] Lock supporting contracts: transport security, handshake, reliability, errors, observability
