# Raven Onboarding Playbook

Use these flows as scripts. Narrate what each command proves, and pause before mutating config or schema.

## Detect vault state first

Always start here. Do not assume a vault already exists.

```bash
rvn vault list --json
```

Read the result:
- Empty `vaults` (or `meta.count` of `0`): there is no vault yet. Follow **New vault setup**.
- One or more entries: at least one vault exists. Follow **Existing vault tour**. Use `active_vault` and `default_vault` to see whether one is already selected.

Only run `rvn vault current --json` once a vault exists — it is for confirming which vault is resolved, not for detecting the empty state.

## New vault setup

This is the primary path for a brand-new user. Ask for the intended path first (suggest a sensible default such as `~/notes` or `~/raven`), then initialize:

```bash
rvn init /path/to/vault --json
```

`rvn init` creates `raven.yaml`, `schema.yaml`, `.raven/`, and starter vault content, and applies the first-run vault policy. Let init own registration and default/active routing — read its `post_init` block before running any `vault` command.

Inspect `post_init` and act accordingly:
- `is_first_vault: true` — this was the first vault on the machine: it is registered, set as the default, and active, and `post_init.actions` is empty. It is ready to use; proceed directly.
- `already_registered: true` — the vault is registered; do **not** run `rvn vault add`.
- `is_default: true` — it is already the default; do **not** run `rvn vault pin`.
- `is_active: true` — it is already active; do **not** run `rvn vault use`.
- `is_first_vault: false` with `needs_user_choice_for_activate` / `needs_user_choice_for_default` set — another vault already exists; `post_init.actions` lists the `activate` / `set_default` actions. Ask the user before running them (or before `rvn vault use <name>` / `rvn vault pin <name>`).
- Any routing field `false` (or `post_init` absent on older builds) — offer to finish setup using the commands surfaced in `post_init.commands` / `post_init.next_steps`. Ask the user before setting a default or active vault, since that is machine-wide routing.

Different builds behave differently, so branch on the fields rather than assuming:
- First vault is registered and set as default/active automatically (`is_first_vault: true`, empty `post_init.actions`).
- A later vault is registered automatically but leaves default/active to the user — ask first (`needs_user_choice_for_activate` / `needs_user_choice_for_default`).
- Older builds may register nothing in `--json` mode, in which case `post_init.commands.register_and_pin` and `post_init.commands.activate` show the exact commands to run once the user agrees.

After init, tour the new vault. If it is not yet the active vault, target it explicitly:

```bash
rvn vault stats --vault <name> --json   # or: --vault-path /path/to/vault
rvn schema --vault <name> --json
```

The active/default vault is machine config, not vault content.

## Existing vault tour

```bash
rvn vault current --json
rvn vault path --json
rvn vault stats --json
rvn schema --json
rvn query 'type:project' --limit 5 --json
rvn query 'trait:todo' --limit 5 --json
```

Explain what exists before proposing changes. If the schema does not define `project` or `todo`, choose another existing type or trait from `rvn schema --json`.

## Explain config surfaces

```bash
rvn config show --json
rvn vault config show --json
```

Teach the distinction:
- Global config tracks named vaults, default vault, editor, UI, and state file location.
- Vault-local config in `raven.yaml` controls directories, capture settings, auto-reindex, deletion policy, protected prefixes, and exclude patterns.

## Add schema safely

Preview the design with the user before running these commands.

```bash
rvn schema add type meeting --name-field title --default-path meeting/ --json
rvn schema add field meeting project --type ref --target project --json
rvn schema add field meeting with --type ref[] --target person --json
rvn schema add trait decision --type bool --json
rvn schema validate --json
rvn reindex --json
rvn check --json
```

Explain:
- Types define files/objects.
- Fields live in frontmatter and are validated by type.
- Traits live inline in Markdown body text.
- `ref` and `ref[]` fields should specify a target type.

If the target types do not exist, stop and ask whether to create them or choose different fields.

## Create the first object

Use a type from the actual schema. For a starter project/person flow:

```bash
rvn new project "Raven Onboarding Demo" --json
rvn new person "Demo User" --json
rvn read project/raven-onboarding-demo --json
```

If required fields are missing, read the error details and retry with `--field` or `--field-json`.

## Demonstrate daily capture

```bash
rvn daily --json
rvn add "Learning Raven with [[project/raven-onboarding-demo]]" --json
rvn add "@todo Try one Raven query against [[project/raven-onboarding-demo]]" --json
rvn date today --json
```

Explain that `rvn add` appends to the configured capture destination, which defaults to today's daily note.

## Demonstrate references and backlinks

```bash
rvn resolve project/raven-onboarding-demo --json
rvn backlinks project/raven-onboarding-demo --json
rvn outlinks daily/YYYY-MM-DD --json
```

Teach:
- `[[project/raven-onboarding-demo]]` creates a link.
- `rvn backlinks` finds content pointing at an object.
- `rvn outlinks` shows links from an object.
- Exact object IDs are safest for automation.

## Demonstrate query

Start broad, then narrow:

```bash
rvn query 'type:project' --count-only --json
rvn query 'type:project' --limit 10 --json
rvn query 'trait:todo' --limit 10 --json
rvn search 'onboarding demo' --json
```

Use `raven-query` for deeper RQL examples once the user understands objects, traits, and references.

## Wrap-up checklist

Before ending onboarding, make sure the user has seen:
- Which vault is active and where it lives.
- What `raven.yaml` and `schema.yaml` are responsible for.
- At least one type and one trait from the actual schema.
- One object created through `rvn new`.
- One line captured through `rvn add`.
- One `[[reference]]` plus a backlinks check.
- One query or search.
- A final `rvn check --json` result.
