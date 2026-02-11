# Build Log Format and Naming

This directory stores buildlog policy/docs and templates.
Active buildlog TOML files live under `local/buildlogs/`.

## Required Naming Scheme

- File name format: `YYYY-MM-DD_HH:MM.toml`
- Timestamp is EST (New York) and represents log creation time.
- Lexicographic file ordering is the canonical pass order.

Examples:
- `2026-02-07_15:27.toml`
- `2026-02-07_16:00.toml`

## Required Process

- Create one new build log for every user prompt.
- Do not append follow-up prompts to an existing build log file.
- Copy `template.toml` and fill all required fields.
- Include:
  - initial prompt
  - all files changed
  - justification for each change
  - any completed tasks from progress checklists

## Template

- Use: `template.toml`
- Save active logs to: `local/buildlogs/YYYY-MM-DD_HH:MM.toml`
