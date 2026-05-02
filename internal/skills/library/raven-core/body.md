# Raven Core

Use this skill for day-to-day Raven operations: creating, reading, editing, moving, and deleting vault content.

This skill is for agents driving Raven through the `rvn` CLI. Raven MCP is a separate, equivalent surface and is not in scope here.

## Operating rules

- Use `rvn` with `--json` for deterministic machine-readable output.
- Prefer `rvn` commands over direct file writes or shell text manipulation (`echo`, `cat >`, `sed`, `awk`).
- Choose the smallest mutation primitive that matches the user's intent.
- Read with `rvn read --raw` before constructing `rvn edit` replacements.
- For bulk or destructive operations, preview first, then re-run with `--confirm`.

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

## Look things up

Raven ships its own long-form documentation. Use these when you need usage details or examples beyond what `rvn <command> --help` shows.

- List doc sections: `rvn docs list --json`
- Read a topic: `rvn docs <section> <topic> --json`
- Search docs: `rvn docs search "<query>" --json`
- Refresh local doc cache if missing or stale: `rvn docs fetch --json`

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
