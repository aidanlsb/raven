# Keep Lifecycle

## Create a new keep

```bash
rvn init /path/to/new-keep --json
```

This creates a keep with default config, schema, and index directories.

## Register and activate named keeps

```bash
rvn keep add work /path/to/work-keep --pin --json
rvn keep add personal /path/to/personal-keep --json
rvn keep use work --json
rvn keep list --json
rvn keep current --json
```

Use `--pin` when the newly added keep should become `default_keep`.

## Inspect resolved keep path

```bash
rvn keep path --json
rvn path --json
```

Both are useful when debugging which keep a command resolved to.

## Global config lifecycle

```bash
rvn config init --json
rvn config show --json
rvn config set --editor code --editor-mode auto --json
rvn config unset --editor --editor-mode --json
```

`rvn config` is machine-level. Do not use it for keep-local behavior.

## Safe removal sequence

```bash
rvn keep remove personal --clear-default --clear-active --json
```

If the target is currently default or active, clear those bindings in the same command.
