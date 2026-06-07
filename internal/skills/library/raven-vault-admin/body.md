# Raven Vault Admin

Use this skill for vault setup, active/default vault selection, and global plus vault-local Raven config.

This skill is CLI-first. Use MCP as a fallback when CLI access is unavailable, preserving the same JSON expectations.

## Operating rules

- Do not assume the current working directory is the active Raven vault.
- Use explicit vault naming and avoid guessing which vault should be active.
- Prefer `rvn vault ...` and `rvn config ...` over manual edits to config files.
- Use `--json` for deterministic automation output.

## Vault resolution

For commands that operate on a vault, Raven resolves the target in this order:

1. `--vault-path`
2. `--vault <name>`
3. `active_vault` from `state.toml`
4. `default_vault` from `config.toml`

Use explicit `--vault` or `--vault-path` when operating across multiple vaults or when the active/default selection is unclear.

## Unknown environment first pass

1. Resolve the current vault: `rvn vault current --json`, `rvn vault path --json`.
2. Inspect what is configured: `rvn vault list --json`, `rvn vault stats --json`.
3. If routing is still unclear, inspect machine config: `rvn config show --json`.

## Typical flow

1. Bootstrap or register vaults (`rvn init`, `rvn vault add`).
2. Set routing defaults (`rvn vault use`, optional `rvn vault pin`).
3. Confirm current resolution (`rvn vault current`, `rvn vault path`, `rvn vault stats`).
4. Manage machine-level settings with `rvn config show`, `rvn config set`, `rvn config unset`.
5. Manage vault-local `raven.yaml` settings with `rvn vault config ...`.

## Config surfaces

- `rvn config ...` manages global machine `config.toml`: editor settings, UI settings, state file location, named vault paths, and `default_vault`.
- `rvn vault config ...` manages vault-local `raven.yaml`: directories, assets, auto-reindex, capture, deletion, protected prefixes, and exclude patterns.

Use structured commands for both surfaces instead of editing TOML or YAML by hand.

## Cross-references

- Use `raven-core` for day-to-day vault operations after setup is complete.
- Use `raven-schema` for schema design after vault initialization.
- Use `raven-maintenance` for `rvn check` and `rvn reindex` to verify vault health.

## Safety

- On `rvn vault remove`, respect guard flags when removing active/default entries.
- Keep `default_vault` and `active_vault` coherent to avoid unexpected fallback behavior.
- In `--json` mode, `rvn init` is non-interactive: it creates vault files and returns post-init suggestions instead of mutating global config implicitly.
- After changing vault-local directories or exclude patterns, use `raven-maintenance` to run `rvn reindex --json` and `rvn check --json`.

## Reference

- End-to-end command sequences and gotchas: `references/vault-lifecycle.md`
