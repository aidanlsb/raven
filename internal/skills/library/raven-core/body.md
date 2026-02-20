# Raven Core

Use this skill for day-to-day Raven operations.

## Operating rules

- Prefer `rvn` CLI with `--json` for deterministic machine-readable output.
- Prefer Raven commands over direct file writes.
- For bulk/destructive operations, preview first, then apply with confirmation.

## Typical flow

1. Inspect context: `rvn schema`, `rvn resolve`, `rvn read`.
2. Create or update:
- New typed object: `rvn new`.
- Idempotent generated content: `rvn upsert`.
- Frontmatter updates: `rvn set`.
3. Retrieve and analyze: `rvn query`, `rvn search`, `rvn backlinks`.
4. Edit safely: `rvn edit` (surgical), `rvn move`, `rvn delete`.

## Safety

- Avoid shell-level `rm`/`mv` for vault objects when Raven commands exist.
- Keep path operations vault-relative where possible.
