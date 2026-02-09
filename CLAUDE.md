# Claude Working Contract — EdgeCTL

You are operating in a production-grade, contract-driven repository.

This project uses a strict documentation-first authority model.
Documentation is the source of truth. Code implements contracts — never the reverse.

Failure to follow these rules is a correctness bug.

---

## Canonical Sources of Truth (Read-Only Unless Explicitly Approved)

The following are authoritative and MUST NOT be modified unless the user explicitly approves changes in the active thread:

- `docs/architecture/definitions/*.toml`
- `docs/architecture/models/*.mmd`
- `docs/architecture/*.md`
- `docs/index.md`

If code disagrees with canonical documentation:

- Treat documentation as correct
- Identify and fix the implementation gap
- Never silently “fix” docs to match code
- New files can be created in docs for undefined architecture only

---

## Mandatory Reading Order (Before Any Code Changes)

Before proposing or making changes, you MUST read:

1. `docs/index.md`
2. Relevant files under `docs/architecture/definitions/`
3. Relevant architecture diagrams under `docs/architecture/models/`

If a change cannot be mapped to a specific contract or diagram, stop and ask.

---

## Change Control Rules

- Files under `docs/progress/`:
  - MUST be updated on every work pass
  - Used to record findings, status, and decisions
- Files under `local/buildlogs/`:
  - MUST be created for every feature implementation
- All other files under `docs/`:
  - Read-only by default
  - New files may be added if necessary
  - Existing files MUST NOT be modified without explicit approval

---

## Build Log Requirements (Strict)

- Build logs live under `local/buildlogs/`
- Each workstream requires a build log
- Use `docs/progress/buildlog/template.toml` for initialization
- Naming scheme (required):
  - `YYYY-MM-DD_HH:MM.toml`
  - EST (New York) timezone is implied

Each build log entry MUST include:

- User prompt(s)
- Files changed
- Summary of changes
- Justification for each change
- Progress checklist items completed

Create a new build log when scope changes materially.

---

## Implementation Discipline

- Map every code change to a specific contract reference
- Maintain package ownership boundaries defined in docs
- Update `internal/*/doc.go` when boundaries or contracts are implemented or adjusted
- Avoid speculative refactors or abstraction creep

---

## Code Commenting Rules

All newly added code MUST include concise, inspection-oriented comments:

- Every type must have a purpose comment
- Every non-trivial function or block must describe:
  - Intent
  - Boundary behavior
- Comments must be factual and brief
- Do not restate obvious syntax
- Do not start comments with the function name, it is inferred

---

## Package Documentation Rule

- Every `internal/*/doc.go` file MUST:
  - Reference the canonical contract(s) it implements
  - Avoid stale or broken doc links

---

## Conformance Gate (Before Major Changes)

Before completing a significant change, verify:

- Protocol constants align with `docs/architecture/definitions/tlv.toml`
- Framing behavior aligns with:
  - `docs/architecture/framing.md`
  - `docs/architecture/definitions/protocol.toml`
- `go test ./...` passes cleanly

If any check fails, stop and report.

---

## Absolute Prohibitions

- Do NOT modify canonical contracts without approval
- Do NOT rename canonical envelopes or roles
- Do NOT introduce new terminology without approval
- Do NOT widen scope
- Do NOT refactor unrelated code
- Do NOT assume undocumented behavior

If uncertain, stop and ask.
