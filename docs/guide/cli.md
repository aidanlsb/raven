# CLI guide

This is a usage guide (patterns and workflows). For exact flags/arguments, run `rvn --help` and `rvn <command> --help`.

## Create

```bash
rvn new person "Freya" --field name="Freya"
rvn new project "Website Redesign"
```

## Capture / append

```bash
rvn add "Quick note"
rvn add "@due(2026-01-10) Follow up with [[people/freya]]"
rvn add "Design notes" --to projects/website.md
```

## Query

```bash
rvn query "object:project .status:active"
rvn query "trait:due value:past"
rvn query --list
rvn query tasks
```

## Bulk operations

```bash
# IDs for piping
rvn query "object:project .status:active" --ids

# Preview a bulk change (dry-run)
rvn query "trait:due value:past" --apply set status=overdue

# Apply it
rvn query "trait:due value:past" --apply set status=overdue --confirm
```

See `reference/bulk-operations.md` for details.

