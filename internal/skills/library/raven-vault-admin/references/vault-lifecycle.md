# Vault Lifecycle

## Create a new vault

```bash
rvn init /path/to/new-vault --json
```

This creates a vault with default config, schema, and index directories.
In `--json` mode, init is non-interactive: it returns post-init suggestions instead of registering, pinning, or activating the vault implicitly.

## Register and activate named vaults

```bash
rvn vault add work /path/to/work-vault --pin --json
rvn vault add personal /path/to/personal-vault --json
rvn vault use work --json
rvn vault list --json
rvn vault current --json
```

Use `--pin` when the newly added vault should become `default_vault`.

## Vault resolution order

When a command needs a vault, Raven resolves in this order:

1. `--vault-path`
2. `--vault <name>`
3. `active_vault` from `state.toml`
4. `default_vault` from `config.toml`

## Inspect resolved vault path

```bash
rvn vault path --json
rvn vault stats --json
```

Use `vault path` to confirm resolution and `vault stats` to confirm you are pointed at the expected index.

## Global machine config lifecycle

```bash
rvn config init --json
rvn config show --json
rvn config set --editor code --editor-mode auto --json
rvn config unset --editor --editor-mode --json
```

`rvn config` is machine-level. Use it for editor/UI settings, state file location, named vault paths, and `default_vault`. Do not use it for vault-local behavior.

## Vault-local config lifecycle

Use `rvn vault config` for `raven.yaml` settings that travel with the vault.

```bash
# Inspect effective vault-local config
rvn vault config show --json

# Auto-reindex behavior
rvn vault config auto-reindex set --value=false --json
rvn vault config auto-reindex unset --json

# Directory layout, including assets
rvn vault config directories get --json
rvn vault config directories set --daily journal --type types --template templates --assets assets --json
rvn vault config directories unset --template --json

# Capture destination
rvn vault config capture get --json
rvn vault config capture set --destination inbox.md --heading "## Captured" --json
rvn vault config capture unset --heading --json

# Deletion behavior
rvn vault config deletion get --json
rvn vault config deletion set --behavior trash --trash-dir archive/trash --json
rvn vault config deletion unset --trash-dir --json

# Protected paths and excluded paths
rvn vault config protected-prefixes list --json
rvn vault config protected-prefixes add private --json
rvn vault config protected-prefixes remove private/ --json
rvn vault config exclude list --json
rvn vault config exclude add '.cursor/' --json
rvn vault config exclude remove '.cursor/' --json
```

After changing directories, assets, or exclude patterns, run `rvn reindex --json` and `rvn check --json`.

## Safe removal sequence

```bash
rvn vault remove personal --clear-default --clear-active --json
```

If the target is currently default or active, clear those bindings in the same command.

## Verify setup

```bash
rvn vault list --json
rvn vault current --json
rvn vault path --json
rvn vault stats --json
```

Use these commands after init, add, use, pin, clear, or remove operations to confirm routing and index state.
