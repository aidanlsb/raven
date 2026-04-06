# Raven Core

Use this skill for day-to-day Raven operations: creating, reading, editing, moving, and deleting vault content.

## Operating rules

- Prefer `rvn` CLI with `--json` for deterministic machine-readable output.
- When operating through Raven MCP, use the equivalent Raven MCP tools instead of shelling out redundantly.
- Prefer Raven commands over direct file writes or shell text manipulation.
- Choose the smallest mutation primitive that matches the user's intent.
- Read with `rvn read --raw` before constructing `rvn edit` replacements.
- For bulk or destructive operations, preview first, then apply with confirmation.

## Choose the right write command

- Create a brand-new object identity: `rvn new`
- Append a log entry or capture text: `rvn add`
- Idempotent generated output (briefs, reports): `rvn upsert`
- Update frontmatter fields only: `rvn set`
- Exact body text replacement: `rvn edit`
- Update a trait value by trait ID: `rvn update`

Key distinctions:
- `upsert` vs `add`: use `upsert` when reruns should converge to one canonical state. Use `add` when history should accumulate.
- `set` vs `edit`: use `set` for structured metadata (frontmatter). Use `edit` for body content changes.
- `new` vs `upsert`: use `new` only when creating a genuinely new object identity. Use `upsert` when the same agent action might run again.

## Daily notes

- Open or create today's daily note: `rvn daily --json`
- Open a specific date: `rvn daily 2026-04-05 --json` or `rvn daily yesterday --json`
- Quick capture to today's note: `rvn add "text" --json`
- Capture to a specific date: `rvn add "text" --to tomorrow --json`
- View all activity for a date: `rvn date today --json`

## Typical flow

1. Inspect context: `rvn schema`, `rvn resolve`, `rvn read --raw`.
2. Choose a write primitive (see command chooser above).
3. For edits, always read the file raw first, then construct the exact `old_str` match.
4. For lifecycle changes: `rvn reclassify` to change type, `rvn move` to rename/relocate, `rvn delete` to remove.
5. After mutations, verify with `rvn read` or `rvn check`.

## Cross-references

- Use `raven-query` for structured retrieval, search, link traversal, and saved queries.
- Use `raven-maintenance` for vault health checks (`rvn check`) and reindexing.
- Use `raven-schema` when the user needs to modify type, field, or trait definitions.

## Safety

- Avoid shell-level `rm`, `mv`, `sed`, or `awk` for vault objects when Raven commands exist.
- Keep path operations vault-relative where possible.
- If `reclassify` reports dropped fields or missing required values, stop and resolve explicitly.
- Check `rvn backlinks` before deleting objects to avoid orphaned references.

## Load references as needed

- Command chooser and CLI snippets: `references/command-map.md`
