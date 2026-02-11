# How to Read This Repository (For Automated Agents)

This repository is documentation-driven.

If multiple sources disagree, precedence is:

1. `docs/architecture/definitions/*.toml`
2. `docs/architecture/models/*.mmd`
3. `docs/index.md`
4. Narrative markdown
5. Code

Code is never authoritative over contracts.

---

## Navigation Root

- `docs/index.md` is the entry point
- All architecture, protocol, and glossary material is indexed there
- Intended read order is explicitly listed and must be followed

---

## Agent Expectations

- Always cite the contract or diagram motivating a change
- Record reasoning in `docs/progress/`
- Log actions in `local/buildlogs/`
- Treat undocumented behavior as invalid

When in doubt, stop.
