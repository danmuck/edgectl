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

- Create one build log for the initial prompt.
- For short, concise, single-target follow-up prompts in the same workstream, update the same build log file.
- Create a new build log when prompt scope changes or when a prompt initiates a larger problem space.
- Copy `template.toml` and fill all required fields.
- Include:
  - initial prompt
  - follow-up prompts
  - all files changed
  - justification for each change
  - any completed tasks from progress checklists

## Template

- Use: `template.toml`
- Save active logs to: `local/buildlogs/YYYY-MM-DD_HH:MM.toml`
