# EdgeCTL Docs Index

This index is the navigation root for architecture and protocol documentation.

## Normative Precedence

- Source of truth is the canonical TOML contracts under `architecture/definitions/*.toml` and architecture diagrams under `architecture/models/*.mmd`.
- Narrative markdown in this docs tree MUST remain consistent with those contracts and diagrams.

## Architecture

- Control loop overview: [`architecture/control-loop.md`](architecture/control-loop.md)
- High-level design contract: [`architecture/definitions/design.toml`](architecture/definitions/design.toml)
- Protocol/package boundary contract: [`architecture/definitions/protocol.toml`](architecture/definitions/protocol.toml)
- Mirage-Ghost transport contract: [`architecture/transport.md`](architecture/transport.md)
- Mirage-Ghost framing contract: [`architecture/framing.md`](architecture/framing.md)
- TLV protocol guide (implementation behavior): [`architecture/tlv.md`](architecture/tlv.md)

## Architecture Diagrams

- Discovery and registration: [`architecture/models/discovery.mmd`](architecture/models/discovery.mmd)
- Transport session lifecycle: [`architecture/models/transport_session_lifecycle.mmd`](architecture/models/transport_session_lifecycle.mmd)
- Framing decoder pipeline: [`architecture/models/framing_decoder_pipeline.mmd`](architecture/models/framing_decoder_pipeline.mmd)
- Locality-aware control loop: [`architecture/models/control_loop_locality.mmd`](architecture/models/control_loop_locality.mmd)
- Single intent loop: [`architecture/models/single_intent.mmd`](architecture/models/single_intent.mmd)
- Phase 5 orchestration boundary: [`architecture/models/phase5_orchestration_boundary.mmd`](architecture/models/phase5_orchestration_boundary.mmd)
- Phase 5 event-to-report flow: [`architecture/models/phase5_event_report_flow.mmd`](architecture/models/phase5_event_report_flow.mmd)
- Multi-ghost reconcile fanout: [`architecture/models/multi_ghost.mmd`](architecture/models/multi_ghost.mmd)
- Decision model: [`architecture/models/decision_model.mmd`](architecture/models/decision_model.mmd)
- State authority: [`architecture/models/state_authority.mmd`](architecture/models/state_authority.mmd)
- Protocol interface boundary: [`architecture/models/proto_interface_boundary.mmd`](architecture/models/proto_interface_boundary.mmd)

## Glossary and Contracts

- Definitions and canonical vocabulary: [`glossary/definitions.md`](glossary/definitions.md)
- Object and interface shapes: [`glossary/shapes.md`](glossary/shapes.md)
- Envelope Go shapes: [`glossary/envelopes.md`](glossary/envelopes.md)
- Glossary slice index: [`glossary/README.md`](glossary/README.md)
- Progress tracker: [`progress/index.md`](progress/index.md)
- Transport slice definitions: [`glossary/transport.md`](glossary/transport.md)
- Frame codec slice definitions: [`glossary/frame.md`](glossary/frame.md)
- TLV codec slice (stub): [`glossary/tlv_codec.md`](glossary/tlv_codec.md)
- Semantic validation slice (stub): [`glossary/semantic_validation.md`](glossary/semantic_validation.md)
- Mirage reconcile slice (stub): [`glossary/mirage_reconcile.md`](glossary/mirage_reconcile.md)
- Ghost dispatch slice (stub): [`glossary/ghost_dispatch.md`](glossary/ghost_dispatch.md)
- Observability slice (stub): [`glossary/observability.md`](glossary/observability.md)

## Canonical TOML Definitions

- System design sandbox: [`architecture/definitions/design.toml`](architecture/definitions/design.toml)
- Protocol boundary contract: [`architecture/definitions/protocol.toml`](architecture/definitions/protocol.toml)
- TLV IDs and required field sets: [`architecture/definitions/tlv.toml`](architecture/definitions/tlv.toml)
- Transport security policy: [`architecture/definitions/transport_security.toml`](architecture/definitions/transport_security.toml)
- Session handshake sequence: [`architecture/definitions/handshake.toml`](architecture/definitions/handshake.toml)
- Timeout/retry/idempotency: [`architecture/definitions/reliability.toml`](architecture/definitions/reliability.toml)
- Error taxonomy and wire codes: [`architecture/definitions/errors.toml`](architecture/definitions/errors.toml)
- Observability and correlation: [`architecture/definitions/observability.toml`](architecture/definitions/observability.toml)

## Intended Read Order

1. [`architecture/definitions/design.toml`](architecture/definitions/design.toml)
2. [`architecture/definitions/protocol.toml`](architecture/definitions/protocol.toml)
3. [`architecture/transport.md`](architecture/transport.md)
4. [`architecture/framing.md`](architecture/framing.md)
5. [`glossary/definitions.md`](glossary/definitions.md)
6. [`architecture/definitions/tlv.toml`](architecture/definitions/tlv.toml)
7. [`architecture/definitions/transport_security.toml`](architecture/definitions/transport_security.toml)
8. [`architecture/definitions/handshake.toml`](architecture/definitions/handshake.toml)
9. [`architecture/definitions/reliability.toml`](architecture/definitions/reliability.toml)
10. [`architecture/definitions/errors.toml`](architecture/definitions/errors.toml)
11. [`architecture/definitions/observability.toml`](architecture/definitions/observability.toml)
12. [`architecture/tlv.md`](architecture/tlv.md)
13. [`glossary/shapes.md`](glossary/shapes.md)
14. [`progress/index.md`](progress/index.md)
15. [`glossary/transport.md`](glossary/transport.md)
16. [`glossary/frame.md`](glossary/frame.md)
17. [`architecture/control-loop.md`](architecture/control-loop.md)
