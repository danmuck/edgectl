# MVP Progress — Phase 6 (Boundary Transport Integration)

Status: `In Progress`

### Tasks

- [x] Bind Mirage command dispatch link to Ghost admin boundary using protocol command/event envelopes (`execute_envelope`)
- [ ] Replace any direct action-style HTTP shortcuts between Mirage and Ghost
- [ ] Wire optional auth block handling and validation hooks
- [ ] Add contract tests for all boundaries

### Acceptance Checks

- [ ] All boundary interactions are envelope-driven
- [x] Protocol encode/decode is used end-to-end for Mirage command dispatch to Ghost admin execute path
