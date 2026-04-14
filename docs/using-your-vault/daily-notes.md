# Daily Notes

Daily notes give you a date-stamped file for each day. Use them for journaling, quick capture, meeting notes, or anything you want anchored to a date. Each daily note is a `date`-typed item managed by Raven.

## Creating daily notes

```bash
rvn daily                    # Today's note (creates if needed)
rvn daily yesterday          # Yesterday
rvn daily tomorrow           # Tomorrow
rvn daily 2026-03-15         # Specific date
rvn daily --json             # Resolve/create and return structured data
rvn daily --json --edit      # CLI-only: also launch your editor
```

`rvn daily` resolves the note for the requested date and creates it if needed. In the human CLI, Raven opens the note in your configured editor when available; in JSON mode, use `--edit` if you want the CLI to launch the editor as well. The `--edit` flag is CLI-only and is not part of the shared MCP/canonical command contract.

Daily notes land under `directories.daily` (default `daily/`) as `YYYY-MM-DD.md`.

## Capturing content

The fastest way to add content to a daily note is `rvn add`:

```bash
rvn add "Met with [[person/freya]] about the rollout"
rvn add "@todo Send scope doc to [[person/freya]]"
rvn add "Quick thought about the redesign" --to today
rvn add "Prep for standup" --to tomorrow
```

By default, `rvn add` appends to today's daily note. Use `--to` to target a different date or any other file.

### Capture configuration

Configure default capture behavior in `raven.yaml`:

```yaml
capture:
  destination: daily       # "daily" or a vault-relative path like "inbox.md"
  heading: "## Captured"   # Optional: append under this heading
```

When `heading` is set, Raven creates the heading if it does not exist and appends new content beneath it.

### Adding under a specific heading

Use `--heading` to append under a particular section:

```bash
rvn add "@todo Review PR" --heading "## Tasks"
```

This creates the heading in today's note if it is missing, then appends the text beneath it.

## Daily note templates

Templates give new daily notes consistent structure. Set one up in three steps:

```bash
# 1. Create the template file
rvn template write daily.md --content "# {{date}}

## Tasks

## Notes

## End of Day"

# 2. Register and bind it to the date core type
rvn schema template set daily_default --file templates/daily.md
rvn schema template bind daily_default --core date
rvn schema template default daily_default --core date
```

Now `rvn daily` uses this template when creating a new note. See `types-and-traits/templates.md` for the full template lifecycle.

## Querying daily notes

Daily notes are `date`-typed items. Query them like any other type:

```bash
# All daily notes
rvn query 'type:date'

# Todos captured in daily notes
rvn query 'trait:todo within(type:date)'

# Todos from a specific day
rvn query 'trait:todo within([[2026-03-15]])'

# Overdue items across all daily notes
rvn query 'trait:due .value<today within(type:date)'
```

### Date references

Reference daily notes with date-style wiki-links:

```markdown
See yesterday's standup notes: [[2026-03-14]]
Follow up from [[2026-03-10]]
```

These resolve to the corresponding daily note file.

## Directory configuration

Daily notes live under `directories.daily` in `raven.yaml`:

```yaml
directories:
  daily: daily/
```

This means daily notes are stored as `daily/2026-03-15.md`. With the default directory configuration, the `daily/` prefix is stripped from IDs, so the ID is just `2026-03-15`. Date references like `[[2026-03-15]]` resolve accordingly. If you remove the `directories.daily` setting (flat layout), the file path becomes the full ID (`daily/2026-03-15`).

## Related docs

- `using-your-vault/common-commands.md` for `rvn read`, `rvn open`, and other commands
- `types-and-traits/templates.md` for the full template lifecycle
- `querying/query-language.md` for RQL syntax
- `using-your-vault/configuration.md` for `raven.yaml` reference
