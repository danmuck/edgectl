# EdgeCTL Terminology and Definitions

This file is the canonical vocabulary for Phase 0 (Contract Freeze).

## Scope

- Source of truth:
  - `/Users/macbook/local/edgectl/docs/architecture/design.toml`
  - `/Users/macbook/local/edgectl/docs/architecture/protocol.toml`
- Purpose:
  - persist naming conventions across docs, interfaces, tests, and implementation
  - remove ambiguous terms before deeper implementation

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
  - `User -> Mirage(issue.intent) -> Ghost(command) -> Seed(seed.execute) -> Ghost(seed.result -> event) -> Mirage(report) -> User`

## Control-Plane Interfaces (Canonical Envelope Names)

- `issue`:
  - User -> Mirage
  - intent ingestion into desired state
- `command`:
  - Mirage -> Ghost
  - imperative execution instruction
- `seed_execute`:
  - Ghost -> Seed
  - concrete operation invocation
- `seed_result`:
  - Seed -> Ghost
  - raw execution outcome
- `event`:
  - Ghost -> Mirage
  - observed state delta from execution
- `report`:
  - Mirage -> User
  - reconciled status/progress summary

## Required Envelope Fields

- `issue`:
  - `intent_id`, `actor`, `target_scope`, `objective`
- `command`:
  - `command_id`, `intent_id`, `ghost_id`, `seed_selector`, `operation`
- `seed_execute`:
  - `execution_id`, `command_id`, `seed_id`, `operation`, `args`
- `seed_result`:
  - `execution_id`, `seed_id`, `status`, `stdout`, `stderr`, `exit_code`
- `event`:
  - `event_id`, `command_id`, `intent_id`, `ghost_id`, `seed_id`, `outcome`
- `report`:
  - `intent_id`, `phase`, `summary`, `completion_state`

## Protocol (Wire-Level) Terms

- `Control Plane Protocol`:
  - binary, message-oriented protocol for control-plane boundaries
- `Header` (fixed-size):
  - `magic`, `version`, `header_len`, `message_id`, `message_type`, `flags`, `payload_len`
- `Auth Block`:
  - optional opaque bytes when `has_auth` flag is set
- `TLV`:
  - payload field encoding: `field_id`, `type`, `length`, `value`
- `Message Type`:
  - semantic category of message (e.g., intent/command/event)
- `Field Type`:
  - primitive type of TLV value (uint/string/bytes/etc.)

## Protocol Flags

- `has_auth` (`0x01`):
  - message includes auth block
- `is_response` (`0x02`):
  - message is a response shape
- `is_error` (`0x04`):
  - message communicates an error condition

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
- `seed`:
  - registration and service-surface exchange between Mirage and Ghost

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
- Use `issue`, `command`, `seed_execute`, `seed_result`, `event`, `report` as canonical envelope names.
- Do not substitute synonyms (`task`, `job`, `action-event`, etc.) for canonical envelope names.
- Prefer `intent_id`, `command_id`, `event_id`, `execution_id` for correlation keys.

## Ambiguity Guardrails

- `command` is Mirage->Ghost, not User->Mirage.
- `event` is Ghost->Mirage, not Seed->Mirage directly.
- `seed_result` is Seed->Ghost internal boundary output.
- `report` is Mirage->User summary, not raw execution output.

## Phase 0 Done Criteria (Vocabulary)

- All docs and tests use canonical envelope names.
- Package/runtime references match the canonical names in `protocol.toml`.
- Ownership language is consistent:
  - Mirage = orchestration and state aggregation
  - Ghost = execution and dispatch
  - Seed = service interface and operation execution
