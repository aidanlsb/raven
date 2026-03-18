# Raven Vault Admin

Use this skill for vault setup, active/default vault selection, and global Raven config.

## Operating rules

- Use explicit vault naming and avoid guessing which vault should be active.
- Prefer `rvn vault ...` and `rvn config ...` over manual file edits in machine config.
- When already connected through Raven MCP, use the matching Raven MCP tools instead of spawning nested CLI calls.
- Use `--json` for deterministic automation output.

## Typical flow

1. Bootstrap or register vaults (`rvn init`, `rvn vault add`).
2. Set routing defaults (`rvn vault use`, optional `rvn vault pin`).
3. Confirm current resolution (`rvn vault current`, `rvn vault path`, `rvn vault stats`).
4. Manage global settings (`rvn config show`, `rvn config set`, `rvn config unset`).

## Safety

- On `rvn vault remove`, respect guard flags when removing active/default entries.
- Keep `default_vault` and `active_vault` coherent to avoid unexpected fallback behavior.

## Reference

- End-to-end command sequences and gotchas: `references/vault-lifecycle.md`
