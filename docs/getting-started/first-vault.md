# Vault creation and management

Use this guide to:
- initialize your first vault
- understand the files Raven created
- configure Raven globally so the CLI can find and open your vaults consistently

If you want the Raven mental model after setup, continue to
[Core concepts](core-concepts.md).

## Initialize a vault

Create a new vault and move into it:

```bash
rvn init ~/notes
cd ~/notes
```

`rvn init` creates the minimum Raven structure:

```text
notes/
├── .gitignore
├── .raven/
├── daily/
├── page/
├── templates/
├── type/
├── raven.yaml
└── schema.yaml
```

## What each file is for

- `.raven/` is derived vault state such as the local index
- `.gitignore` excludes Raven's derived index and trash directories
- `raven.yaml` is vault-local operational configuration
- `schema.yaml` is the vault data model
- the content directories match the defaults in `raven.yaml`

Markdown files are still the durable source of truth. `.raven/` can be rebuilt with `rvn reindex`.
Raven's long-form docs cache is global and lives next to global config, not inside each vault.

`rvn init` also applies Raven's first-run vault policy so the CLI can find the new vault:

- It auto-registers the vault in global config under a suggested name.
- If this is the **first vault** on the machine (no `default_vault`, no `active_vault`, no other registered vault), it also sets the vault as `default_vault` and `active_vault`. The next command can find it without extra flags.
- If you already have another vault, `rvn init` registers and activates the new one immediately while leaving `default_vault` unchanged. Output identifies the new active vault, the previous active/resolved vault, and the exact `rvn vault use ...` (or `rvn vault clear`) command that restores the prior routing.

This is the same in interactive and `--json` mode. In `--json` mode, the `post_init` object reports what happened (`is_first_vault`, `has_existing_default`, `registered`, `is_default`, `is_active`, `activated`), structured `active_vault`, `previous_active_vault`, and `previous_vault` details, plus `switch_back`, invocable actions, and guidance. Interactive output prints the same switch clearly and only prompts about changing the default.

If Raven cannot load global config/state or persist registration/activation, `init` fails loudly even though the vault-local files were created. JSON error details include `initialized: true`, the path, and `post_init`. Fix global config/state access and rerun init.

## Sanity-check the new vault

Run a few basic commands right away:

```bash
rvn vault stats
rvn schema types
rvn schema traits
```

These resolve automatically because `rvn init` makes the new vault active.

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

## Register additional vaults

Every vault is registered automatically by `rvn init`. The first becomes default and active; each additional vault becomes active while the existing default remains unchanged. To manage names and routing across multiple vaults:

```bash
rvn vault add work ~/work-notes --json
rvn vault list --json
rvn vault use work --json
rvn vault pin work --json
```

`rvn vault add` gives a vault a stable name, `rvn vault use <name>` switches the active vault, and `rvn vault pin <name>` changes the default. `rvn init` reports its automatic active-vault switch and the exact restore command. You can still target any vault explicitly with `--vault <name>` / `--vault-path <path>`.

## How Raven decides which vault to use

When a command needs a vault, Raven resolves in this order:

1. `--vault-path`
2. `--vault <name>`
3. `active_vault` from `state.toml`
4. `default_vault` from `config.toml`

If `active_vault` is set but does not name an entry in `[vaults]`, CLI
resolution fails instead of falling back to `default_vault`. Repair it with
`rvn vault use <name>` or clear it with `rvn vault clear`.

If you mostly work in one vault, setting `default_vault` and `active_vault` makes the CLI much less noisy.

## What belongs in `raven.yaml` vs `config.toml`

Use `config.toml` for machine preferences and vault registry:
- editor
- default vault
- named vault paths
- terminal Markdown rendering (`ui.markdown_style`)

Use `raven.yaml` for vault behavior that should travel with the vault:
- directory layout
- auto-reindex behavior
- capture destination
- saved queries

## Minimal recommended setup

On a brand-new machine, `rvn init` alone gives you a working, resolvable vault:

```bash
rvn init ~/notes
rvn config set editor=cursor editor_mode=auto --json
```

The first `rvn init` registers `~/notes` and sets it as the default and active vault, so the CLI can find it immediately. Add `rvn config set` only if you want to configure the editor.

## Next steps

- Read [Core concepts](core-concepts.md) for the Raven mental model.
- Read [Agent setup](agent-setup.md) if you want MCP and skills next.
- Read [Configuration](../using-your-vault/configuration.md) for the full
  configuration reference.
- Try `rvn daily` to create your first daily note, then read
  [Daily notes](../using-your-vault/daily-notes.md).
- Return to the [documentation map](documentation-map.md) for every topic.
