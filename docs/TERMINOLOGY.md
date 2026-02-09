# EdgeCTL Canonical Terminology (Phase 0 — Frozen)

This document defines the mandatory vocabulary for EdgeCTL.
All code, tests, and documentation MUST use these terms verbatim.

Do not invent synonyms.
Do not reinterpret roles.
Do not collapse boundaries.

Violations are correctness bugs.

---

## Canonical Sources

- `architecture/definitions/design.toml`
- `architecture/definitions/protocol.toml`
- `architecture/definitions/tlv.toml`

This file defines names and meanings only.
Numeric IDs and field tables live in TOML contracts.

---

## Core Roles

- **User**
  - External actor submitting intents and receiving reports

- **Mirage**
  - Orchestration layer
  - Owns desired state and aggregated observed state
  - Reconciles intents into commands

- **Ghost**
  - Execution layer
  - Receives commands
  - Dispatches to Seeds
  - Emits events

- **Seed**
  - Service interface exposed by Ghosts
  - Executes concrete operations
  - Returns execution results

---

## Chain of Custody (Canonical)

→ Mirage (issue.intent)
→ Ghost (command)
→ Seed (seed.execute)
→ Ghost (seed.result → event)
→ Mirage (event.ack, report)
→ User

This flow MUST NOT be altered or bypassed.

---

## Canonical Envelopes

Use these names exactly:

- `issue`
- `command`
- `seed.execute`
- `seed.result`
- `event`
- `event.ack`
- `report`

No substitutions (`task`, `job`, `action`, etc.) are allowed.

---

## Reconciliation Vocabulary

- **desired state** — owned by Mirage
- **observed state** — aggregated from Ghost events
- **reconcile** — derive commands from state delta
- **drift** — divergence between desired and observed
- **corrective command** — command issued to reduce drift

---

## Lifecycle Verbs (Exact Meanings)

- `appear` — initialize runtime node
- `shimmer` — Mirage assumes control surface
- `seed` — registry initialization
- `radiate` — Ghost exposes seed registry

Order matters.

---

## Naming Rules

- Capitalize role names: `Mirage`, `Ghost`, `Seed`
- Envelope names are lowercase and dot-qualified
- Correlation keys:
  - `intent_id`
  - `command_id`
  - `event_id`
  - `execution_id`

---

## Ambiguity Guardrails

- `command` is Mirage → Ghost only
- `event` is Ghost → Mirage only
- `seed.result` never crosses Mirage boundary
- `report` is reconciled, not raw execution output

---

## Phase 0 Vocabulary Done Criteria

- All docs and tests use canonical names
- Runtime and package names align with contracts
- Ownership language is consistent and enforced
