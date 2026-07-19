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

- `.raven/` is derived vault state such as the local index
- `raven.yaml` is vault-local operational configuration
- `schema.yaml` is the vault data model

Markdown files are still the durable source of truth. `.raven/` can be rebuilt with `rvn reindex`.
Raven's long-form docs cache is global and lives next to global config, not inside each vault.

`rvn init` also applies Raven's first-run vault policy so the CLI can find the new vault:

- It auto-registers the vault in global config under a suggested name.
- If this is the **first vault** on the machine (no `default_vault`, no `active_vault`, no other registered vault), it also sets the vault as `default_vault` and `active_vault` — first run just works.
- If you already have another vault, `rvn init` still registers the new one but leaves `default_vault` and `active_vault` untouched. Raven records a pending selection guard: an unqualified command that would resolve to the other vault fails with `VAULT_AMBIGUOUS` instead of running there. Activate the new vault with `rvn vault use <name>`, or target either vault explicitly with `--vault` / `--vault-path`.

This is the same in interactive and `--json` mode. In `--json` mode, the `post_init` object reports what happened (`is_first_vault`, `has_existing_default`, `registered`, `is_default`, `is_active`, `selection_guard_active`), whether a choice still needs your input (`needs_user_choice_for_activate`, `needs_user_choice_for_default`), plus invocable actions and guidance. In interactive mode, Raven prompts you for the default/active choices only when another vault already exists.

If Raven cannot load global config/state or persist the selection guard, `init` fails loudly even though the vault-local files were created. JSON error details include `initialized: true`, the path, and `post_init`; fix global config/state access and rerun init before relying on ambient vault selection.

## Sanity-check the new vault

Run a few basic commands right away:

```bash
rvn vault stats
rvn schema types
rvn schema traits
```

These resolve automatically for the first vault. For an additional vault, activate it first or add `--vault <registered-name>` to each command.

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

Your first vault is registered automatically by `rvn init`. When you create more vaults, `rvn init` also registers each one, but only the first vault is set as default/active. To manage names and routing across multiple vaults:

```bash
rvn vault add work ~/work-notes --json
rvn vault list --json
rvn vault use work --json
rvn vault pin work --json
```

`rvn vault add` gives a vault a stable name, `rvn vault use <name>` switches the active vault, and `rvn vault pin <name>` changes the default. These stay explicit so an additional vault never silently changes which vault your commands target. After initializing an additional vault, use `--vault <name>` / `--vault-path <path>` until you choose an active vault; Raven refuses an ambient target that points elsewhere.

## How Raven decides which vault to use

When a command needs a vault, Raven resolves in this order:

1. `--vault-path`
2. `--vault <name>`
3. `active_vault` from `state.toml`
4. `default_vault` from `config.toml`

After an additional `rvn init`, Raven compares an ambient result from steps 3–4 with the newly initialized vault. If they differ, the command fails with `VAULT_AMBIGUOUS`; steps 1–2 remain available, and `rvn vault use <name>` resolves the pending choice.

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

## Minimal recommended setup

On a brand-new machine, `rvn init` alone gives you a working, resolvable vault:

```bash
rvn init ~/notes
rvn config set --editor cursor --editor-mode auto --json
```

The first `rvn init` registers `~/notes` and sets it as the default and active vault, so the CLI can find it immediately. Add `rvn config set` only if you want to configure the editor.

## Next steps

- Read `getting-started/core-concepts.md` for the Raven mental model
- Read `getting-started/agent-setup.md` if you want MCP and skills next
- Read `using-your-vault/configuration.md` for the full configuration reference
- Try `rvn daily` to create your first daily note — see `using-your-vault/daily-notes.md`
