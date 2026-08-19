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

If the text starts with a dash, put it after the `--` flag terminator so Cobra
does not parse it as a command flag:

```bash
rvn add --to today -- "- Reviewed the rollout"
```

### Capture configuration

Configure default capture behavior in `raven.yaml`:

```yaml
capture:
  destination: daily       # "daily" or a vault-relative path like "inbox.md"
  heading: "## Captured"   # Optional: append under this existing heading
```

When `heading` is set, Raven appends beneath that literal existing heading. A
missing configured heading is an error; `rvn add` never creates it. Create the
section explicitly with `rvn section create`.

### Adding under a specific heading

Target a section by canonical ID with `--to`:

```bash
rvn add "@todo Review PR" --to 2026-03-15#tasks
```

If the section does not exist, create it first with plain title text and an
explicit level, then use the returned canonical section ID:

```bash
rvn section create 2026-03-15 "Tasks" --level 2
rvn add "@todo Review PR" --to 2026-03-15#tasks
```

`--heading` and `--create-heading` have been removed from `add`. Passing either
is an `INVALID_INPUT` error. `add` also rejects text containing Markdown
headings; it is a body-content command, not a section lifecycle command.

Section-targeted `add` inserts at the end of the section's direct body, before
any child headings. By contrast, `section create --after` and `section move`
use complete subtree boundaries.

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
rvn schema template bind daily_default --core date --default
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

# Todos from a date range
rvn query 'trait:todo within(type:date .date>=2026-03-01 .date<=2026-03-31)'

# Overdue items across all daily notes
rvn query 'trait:due .value<today within(type:date)'
```

### The date hub: `rvn date`

`rvn date` collects everything connected to a date in one view: the daily note itself, objects and traits with date fields pointing at that day, and backlinks to the daily note.

```bash
rvn date                     # Today
rvn date yesterday
rvn date 2026-03-15
rvn date today --json
```

Unlike `rvn daily`, it never creates the note — it is a read-only view for reviewing a day's activity.

### Date references

Reference daily notes with date-style wiki-links:

```markdown
See yesterday's standup notes: [[2026-03-14]]
Follow up from [[2026-03-10]]
```

These resolve to the corresponding daily note. The canonical object ID of a daily
note is the bare ISO date (`2026-03-15`), so `[[2026-03-15]]` is unambiguous date
identity regardless of where the file lives on disk.

For backward compatibility, older references that include the daily directory —
`[[daily/2026-03-15]]` or `[[<your-daily-dir>/2026-03-15]]` — still resolve to the
same daily note as compatibility aliases. Prefer the bare form in new content.

## Directory configuration

Daily notes live under `directories.daily` in `raven.yaml`:

```yaml
directories:
  daily: daily/
```

`directories.daily` controls **only** where daily-note files are stored on disk
(default `daily/2026-03-15.md`). It is **not** part of the daily note's object ID:
the object ID is always the bare date `2026-03-15`, no matter what you set the
daily directory to (e.g. `journal/`) or whether you configure it at all. Date
references like `[[2026-03-15]]` therefore resolve the same way across every
vault layout.

## Related docs

- `using-your-vault/common-commands.md` for `rvn read`, `rvn open`, and other commands
- `types-and-traits/templates.md` for the full template lifecycle
- `querying/query-language.md` for RQL syntax
- `using-your-vault/configuration.md` for `raven.yaml` reference
