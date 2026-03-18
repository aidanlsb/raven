# Vault Lifecycle

## Create a new vault

```bash
rvn init /path/to/new-vault --json
```

This creates a vault with default config, schema, and index directories.

## Register and activate named vaults

```bash
rvn vault add work /path/to/work-vault --pin --json
rvn vault add personal /path/to/personal-vault --json
rvn vault use work --json
rvn vault list --json
rvn vault current --json
```

Use `--pin` when the newly added vault should become `default_vault`.

## Inspect resolved vault path

```bash
rvn vault path --json
rvn vault stats --json
```

Use `vault path` to confirm resolution and `vault stats` to confirm you are pointed at the expected index.

## Global config lifecycle

```bash
rvn config init --json
rvn config show --json
rvn config set --editor code --editor-mode auto --json
rvn config unset --editor --editor-mode --json
```

`rvn config` is machine-level. Do not use it for vault-local behavior.

## Safe removal sequence

```bash
rvn vault remove personal --clear-default --clear-active --json
```

If the target is currently default or active, clear those bindings in the same command.
