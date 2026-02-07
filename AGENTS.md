# EdgeCTL Agent Rules

These instructions govern how agents should make changes in this repository.

## Canonical Sources of Truth

- Protocol and architecture contracts:
  - `docs/architecture/definitions/*.toml`
  - `docs/architecture/models/*.mmd`
- Navigation and precedence:
  - `docs/index.md`

## Change Control

- Do not modify canonical contract files or architecture models without explicit user approval in the active thread.
- If code and canonical docs disagree, treat docs as authoritative and open/fix implementation gaps rather than silently changing docs.
- Agents SHOULD update `docs/progress/` on every pass to reflect current status and findings.
- Agents SHOULD update `docs/progress/buildlog/` on every pass.
- Docs outside `docs/progress/` are read-only by default and MUST NOT be modified without explicit user approval in the active thread. New files are allowed if necessary, but no modifications to existing files in these directories without express user approval.

## Build Log Policy

- Build logs live under `docs/progress/buildlog/`.
- Agents MUST create one build log file for the initial prompt using `docs/progress/buildlog/template.toml`.
- Agents MUST append follow-up prompts to the same build log when prompts are short, concise, single-target clarifications within the same workstream.
- Agents MUST create a new build log when prompt scope changes or when a prompt initiates a larger problem space.
- Build log entries MUST include:
  - initial user prompt
  - follow-up prompts
  - files changed and summaries
  - justification for each change
  - any progress checklist tasks completed in that pass
- Naming scheme is strict and ordered:
  - `YYYY-MM-DD_HH:MM.toml`
  - EST (New York) is implied for file names.

## Implementation Workflow

1. Read `docs/index.md` and the relevant canonical contract files before coding.
2. Map code changes to explicit contract references.
3. Keep package ownership boundaries aligned with doc contracts.
4. Update package `doc.go` references when boundaries or contracts change.

## Package Doc Stub Rule

- `internal/*/doc.go` files must include references to the canonical contract docs they implement.
- Avoid stale links in `doc.go` references (for example, moved docs paths).

## Conformance Gate Before Major Changes

- Verify protocol message and field constants align with `docs/architecture/definitions/tlv.toml`.
- Verify framing behavior aligns with `docs/architecture/framing.md` and `docs/architecture/definitions/protocol.toml`.
- Run `go test ./...` before closing a conformance pass.
