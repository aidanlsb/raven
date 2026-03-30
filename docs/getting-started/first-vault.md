# Vault Creation & Management

Use this guide to:
- initialize your first vault
- understand the files Raven created
- configure Raven globally so the CLI can find and open your vaults consistently

If you want the Raven mental model after setup, continue to `getting-started/core-concepts.md`.

## Initialize a vault

Create a new vault and move into it:

```bash
rvn init ~/notes
cd ~/notes
```

`rvn init` creates the minimum Raven structure:

```text
notes/
├── .raven/
├── raven.yaml
└── schema.yaml
```

## What each file is for

- `.raven/` is derived state such as the local index and docs cache
- `raven.yaml` is vault-local operational configuration
- `schema.yaml` is the vault data model

Markdown files are still the durable source of truth. `.raven/` can be rebuilt with `rvn reindex`.

In an interactive terminal, `rvn init` now follows up and can help you:
- register the vault in global config
- set it as `default_vault`
- set it as `active_vault`

In `--json` mode, Raven stays non-interactive and returns structured post-init setup guidance instead.

## Sanity-check the new vault

Run a few basic commands right away:

```bash
rvn vault stats
rvn schema types
rvn schema traits
```

Those confirm:
- Raven can locate the vault
- the starter schema loaded
- the derived index is working

## Global Raven config

Raven also has machine-level config outside the vault. This is what lets you register named vaults, set defaults, and configure editor behavior.

The main global configuration files are:

| File | Scope | Purpose |
|------|-------|---------|
| `~/.config/raven/config.toml` | machine | Global defaults, vault registry, editor/UI settings |
| `~/.config/raven/state.toml` | machine | Mutable runtime state such as `active_vault` |


## Create or inspect global config

If you want Raven to remember vault names and defaults across shells, initialize the global config:

```bash
rvn config init --json
rvn config show --json
```

Typical `config.toml`:

```toml
default_vault = "notes"
editor = "cursor"
editor_mode = "auto"

[vaults]
notes = "/Users/you/notes"
work = "/Users/you/work-notes"
```

## Register this vault globally

If you want to refer to the vault by name instead of full path:

```bash
rvn vault add notes ~/notes --pin --json
rvn vault list --json
rvn vault use notes --json
```

That gives you a stable name, sets it as the default, and makes it the active vault.

## How Raven decides which vault to use

When a command needs a vault, Raven resolves in this order:

1. `--vault-path`
2. `--vault <name>`
3. `active_vault` from `state.toml`
4. `default_vault` from `config.toml`

If you mostly work in one vault, setting `default_vault` and `active_vault` makes the CLI much less noisy.

## What belongs in `raven.yaml` vs `config.toml`

Use `config.toml` for machine preferences and vault registry:
- editor
- default vault
- named vault paths
- UI preferences

Use `raven.yaml` for vault behavior that should travel with the vault:
- directory layout
- auto-reindex behavior
- capture destination
- saved queries
- workflows

## Minimal recommended setup

If you only want the essential setup and nothing more:

```bash
rvn init ~/notes
rvn vault add notes ~/notes --pin --json
rvn config set --editor cursor --editor-mode auto --json
```

At that point Raven is installed, the vault exists, and the CLI knows how to find it.

## Next steps

- Read `getting-started/core-concepts.md` for the Raven mental model
- Read `getting-started/agent-setup.md` if you want MCP and skills next
- Read `using-your-vault/configuration.md` for the full configuration reference
- Try `rvn daily` to create your first daily note — see `using-your-vault/daily-notes.md`
