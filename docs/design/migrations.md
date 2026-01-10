# Migrations & upgrades (design)

## Core principle

Markdown files are the source of truth. The SQLite database under `.raven/` is a cache and can be rebuilt with `rvn reindex`.

## What “migration” means in Raven

There are a few categories of breaking change:
- **Schema format** changes (`schema.yaml` shape/version)
- **Syntax** changes in markdown (`@...` trait syntax, `::type(...)`, etc.)
- **Vault organization** changes (directory roots, default paths, etc.)

## `rvn migrate`

Raven has a top-level `rvn migrate` command with flags:
- `--dry-run`
- `--schema`
- `--syntax`
- `--all`

Current status (as implemented):
- **Schema migration**: partially implemented (creates backups and guides manual steps; not a full transformer).
- **Syntax migration**: not implemented yet (placeholder).

Run `rvn migrate` with no flags to see what Raven thinks needs attention.

## Directory migration

Directory organization has a dedicated subcommand:

```bash
rvn migrate directories --dry-run
rvn migrate directories
```

This moves files to the configured `directories.objects` / `directories.pages` roots while keeping logical object IDs stable.

## Backups

When Raven performs migrations that modify files, it writes timestamped backups under:
- `.raven/backups/<timestamp>/`

Pragmatically, you should still keep your vault under git / synced storage so rollback is trivial.

