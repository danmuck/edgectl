# Claude Code Policy + Repository Validation Pass (2026-02-21)

## Scope

- Apply Claude-facing agent policy hardening for commit approval and review-break behavior.
- Perform full repository validation for conformance, docs/protocol integrity, and structural duplication opportunities without changing runtime behavior.

## Contract References

- `docs/index.md`
- `docs/architecture/definitions/protocol.toml`
- `docs/architecture/definitions/tlv.toml`
- `docs/architecture/definitions/reliability.toml`
- `docs/architecture/framing.md`

## Policy Changes

- Updated `AGENTS.md` to require explicit user approval before commit creation and to enforce a pause/review handoff for large feature implementations.
- Updated `CLAUDE.md` with the same commit gate and large-feature review-break requirements.
- Updated `docs/CLAUDE_README.md` so Claude-focused onboarding guidance includes the same constraints.

## Structural Refactor (No Behavior Change)

- Added `internal/tools/strings.go` with `NormalizeNonEmptyList`.
- Replaced duplicated list-normalization helpers in:
  - `cmd/ghostctl/config.go`
  - `cmd/miragectl/config.go`
- Removed redundant local helper implementations while preserving normalized output behavior (trim whitespace, drop empty entries, preserve order).

## Validation Evidence

- `go test ./...` passed after changes.
- `go vet ./...` passed after changes.
- Markdown link integrity scan across repo docs passed (no broken local links).
- Mermaid file header/sanity scan passed for `docs/architecture/models/*.mmd` and `docs/progress/*.mmd`.
- Protocol/TLV numeric conformance script passed:
  - canonical message type IDs == implementation IDs
  - canonical primitive type IDs == implementation IDs
  - canonical field IDs == implementation IDs

## Dead/Unused/Duplicate Code Findings

- Duplicate code found and removed:
  - duplicated list-normalization helpers in `cmd/ghostctl/config.go` and `cmd/miragectl/config.go`
- No additional high-confidence dead-code removals were applied in this pass.
- Package dependency reachability remains consistent for active command targets.

## Tooling Constraints Noted

- `staticcheck` in this environment is built with older Go (`go1.23.5`) than module requirement (`go1.25.6`) and cannot produce valid results.
- `golangci-lint` could not complete module context loading under this sandbox/network setup.
- Core conformance and behavior validation were completed via deterministic repository checks listed above.
