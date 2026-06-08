# Raven Onboarding Playbook

Use these flows as scripts. Narrate what each command proves, and pause before mutating config or schema.

## New vault setup

Ask for the intended path first, then run:

```bash
rvn init /path/to/vault --json
rvn vault add personal /path/to/vault --json
rvn vault use personal --json
rvn vault stats --vault personal --json
rvn schema --vault personal --json
```

Explain:
- `rvn init` creates `raven.yaml`, `schema.yaml`, `.raven/`, and starter vault content.
- `rvn vault add` registers a machine-local name for the vault.
- `rvn vault use` sets the active vault for later commands.
- The active/default vault is machine config, not vault content.

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
