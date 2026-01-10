# Configuration

Raven uses:
- `schema.yaml` — types + traits (structure)
- `raven.yaml` — vault settings + saved queries + workflows (behavior)
- `~/.config/raven/config.toml` — global settings (default vault, editor)

## `schema.yaml` (types + traits)

Use this for:
- defining types and their frontmatter fields
- defining traits and their value types

Start here: `reference/schema.md`.

## `raven.yaml` (vault config)

Use this for:
- daily notes directory/template
- saved queries (short names for RQL queries)
- workflows
- directory roots (`directories.objects`, `directories.pages`)
- auto-reindex and capture settings

Start here: `reference/vault-config.md`.

## Templates

Templates can be:
- per-type templates in `schema.yaml` (used by `rvn new`)
- daily note template in `raven.yaml` (used by `rvn daily`)

Template variables are documented in `reference/schema.md` (type templates) and `reference/vault-config.md` (daily template).

## Next steps

- See `reference/schema.md` for complete type and trait definition options
- See `reference/vault-config.md` for all vault configuration settings
- See `cli.md` for schema management commands

