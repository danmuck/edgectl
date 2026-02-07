# EdgeCTL Terminology and Definitions

This file is the canonical vocabulary for Phase 0 (Contract Freeze).

## Scope

- Source of truth:
  - `../architecture/definitions/design.toml`
  - `../architecture/definitions/protocol.toml`
  - `../architecture/definitions/tlv.toml`
- Purpose:
  - persist naming conventions across docs, interfaces, tests, and implementation
  - remove ambiguous terms before deeper implementation

## Contract Ownership

- `../architecture/definitions/design.toml`:
  - system roles, authority boundaries, lifecycle vocabulary
- `../architecture/definitions/protocol.toml`:
  - chain of custody, wire header fields, boundary links, envelope catalog
- `../architecture/definitions/tlv.toml`:
  - primitive type IDs, message type IDs, field IDs/types, required field sets
- `definitions.md`:
  - canonical terms and naming rules only (no numeric ID tables)

## Core Roles

- `User`:
  - external actor that submits intents and receives reports
- `Mirage`:
  - orchestration layer
  - owns desired state and aggregated observed state
  - reconciles intents into commands
- `Ghost`:
  - execution layer
  - receives commands, dispatches to seeds, emits events
- `Seed`:
  - service interface provided by ghosts
  - executes concrete operations and returns execution results

## Chain of Custody

- Canonical flow:
  - `User -> Mirage(issue.intent) -> Ghost(command) -> Seed(seed.execute) -> Ghost(seed.result -> event) -> Mirage(event.ack, report) -> User`

## Control-Plane Interfaces (Canonical Envelope Names)

- `issue`:
  - User -> Mirage
  - intent ingestion into desired state
- `command`:
  - Mirage -> Ghost
  - imperative execution instruction
- `seed.execute`:
  - Ghost -> Seed
  - concrete operation invocation
- `seed.result`:
  - Seed -> Ghost
  - raw execution outcome
- `event`:
  - Ghost -> Mirage
  - observed state delta from execution
- `event.ack`:
  - Mirage -> Ghost
  - event delivery acknowledgment (`accepted` or `rejected`)
- `report`:
  - Mirage -> User
  - reconciled status/progress summary

## Reconciliation Terms

- `desired state`:
  - what User wants (owned by Mirage)
- `observed state`:
  - what actually happened (aggregated by Mirage from Ghost events)
- `reconcile`:
  - compare desired vs observed and derive next command set
- `drift`:
  - observed state diverges from desired state
- `corrective command`:
  - command issued to reduce drift

## Lifecycle Verbs

- `appear`:
  - initialize runtime node (`miragectl` or `ghostctl`)
- `shimmer`:
  - Mirage assumes control of command registry / intent routing surface
- `radiate`:
  - Ghost serves its seed registry / command routing surface
  - occurs after `seed`
  - valid with an empty seed registry
- `seed`:
  - registration and service-surface exchange between Mirage and Ghost
  - prepares Ghost registry before `radiate`

## Seed Terms

- `seed metadata`:
  - `id`, `name`, `description`
- `seed selector`:
  - identifier used by Ghost to choose a target seed
- `operation`:
  - concrete executable action exposed by a seed

## Runtime and Package Terms

- `miragectl`:
  - Mirage server runtime entrypoint
- `ghostctl`:
  - Ghost server runtime entrypoint
- `internal/protocol`:
  - wire format + codec + semantic parsing primitives
- `internal/mirage`:
  - orchestration concerns
- `internal/ghost`:
  - execution concerns
- `internal/seeds`:
  - seed service interfaces exposed by ghosts

## Naming Rules

- Use `Mirage`, `Ghost`, `Seed` as role names (capitalized) in docs.
- Use `issue`, `command`, `seed.execute`, `seed.result`, `event`, `event.ack`, `report` as canonical envelope names.
- Do not substitute synonyms (`task`, `job`, `action-event`, etc.) for canonical envelope names.
- Prefer `intent_id`, `command_id`, `event_id`, `execution_id` for correlation keys.

## Ambiguity Guardrails

- `command` is Mirage->Ghost, not User->Mirage.
- `event` is Ghost->Mirage, not Seed->Mirage directly.
- `event.ack` is Mirage->Ghost delivery closure for `event`.
- `seed.result` is Seed->Ghost internal boundary output.
- `report` is Mirage->User summary, not raw execution output.

## Phase 0 Done Criteria (Vocabulary)

- All docs and tests use canonical envelope names.
- Package/runtime references match the canonical names in `definitions/protocol.toml`.
- Ownership language is consistent:
  - Mirage = orchestration and state aggregation
  - Ghost = execution and dispatch
  - Seed = service interface and operation execution
