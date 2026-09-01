# Raven Core

Use this skill for day-to-day Raven operations: creating, reading, editing, moving, and deleting vault content.

This skill is CLI-first. Use MCP as a fallback when CLI access is unavailable, preserving the same JSON and preview/apply expectations.

## Operating rules

- Use `rvn` with `--json` for deterministic machine-readable output.
- Read the primary homogeneous result collection from `data.items`; an empty
  collection is `[]`, not `null`.
- Prefer `rvn` commands over direct file writes or shell text manipulation (`echo`, `cat >`, `sed`, `awk`).
- Choose the smallest mutation primitive that matches the user's intent.
- Read with `rvn read --raw` before constructing `rvn edit` replacements.
- Author `[[references]]` and `ref` fields with canonical object IDs. After
  `new`/`upsert`/`daily`, use the returned `data.id` (the human CLI's
  `link as <id>` value); do not generate bare short forms.
- Single-object writes (`rvn set`/`add`/`update`/`edit`, `rvn section create`/`move`/`rename`, and single `rvn move`/`rvn delete`) apply immediately; pass `--dry-run` to preview without writing. `rvn section delete` is preview-first and requires `--confirm`. Bulk operations (`--stdin`), `query --apply`, `schema rename`, `rvn check fix`, and `rvn check create-missing` also stay preview-first and require `--confirm`.

## Choose the right write command

- Create a brand-new object identity: `rvn new`
- Append a log entry or capture text: `rvn add`
- Idempotent generated output (briefs, reports): `rvn upsert`
- Update frontmatter fields only: `rvn set <reference> --field field=value --json`
- Exact body text replacement: `rvn edit`
- Update a trait value by trait ID: `rvn update`
- Copy an external non-Markdown file into the vault, then run `rvn reindex`
- Relocate a non-Markdown file already in the vault: `rvn move` (updates file links)
- Create/reorder/rename/delete headings: `rvn section create` / `rvn section move` / `rvn section rename` / `rvn section delete`

Key distinctions:
- `upsert` vs `add`: use `upsert` when reruns should converge to one canonical state. Use `add` when history should accumulate.
- `set` vs `edit`: use `set` for structured metadata (frontmatter). Use `edit` for body content changes.
- `set` arguments use `field=value` pairs, for example `rvn set project/raven --field status=active --json`.
- Use `--fields-json '{...}'` on `new`, `upsert`, `set`, and `reclassify` when
  exact JSON typing matters.
- `new` vs `upsert`: use `new` only when creating a genuinely new object identity. Use `upsert` when the same agent action might run again.
- Use `data.id` for references and follow-up commands; never derive an ID from
  `data.file`, because configured directory roots and daily-note paths can make
  them differ.

Writes remain permissive when a new `[[reference]]` target does not exist. An
`ok=true` response can include `REF_TARGET_MISSING`, `data.missing_refs`,
`data.missing_ref_items`, and a structured `create_invoke`. This differs from
fatal `REF_NOT_FOUND` on reads. Create appropriate targets with preview-first
`rvn check create-missing --json`, then `--confirm`.

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
4. For lifecycle changes: `rvn reclassify` to change type, `rvn move` to rename/relocate files, `rvn section create`/`move`/`rename`/`delete` for headings, and `rvn delete` to remove files.
5. After mutations, verify with `rvn read` or `rvn check`.

After a trashed delete, recover through `rvn trash list`, preview
`rvn restore <reference>`, then repeat with `--confirm`. Bulk reclassification
can require both `--confirm` and `--force` when the preview reports dropped
fields.

## Look things up

Raven ships its own long-form documentation. Use these when you need usage details or examples beyond what `rvn <command> --help` shows.

- List doc sections: `rvn docs --json`
- Read a topic: `rvn docs <section> <topic> --json`
- Search docs: `rvn docs search "<query>" --json` (continue with `--offset` when `has_more` is true)
- Existing stale caches refresh lazily on those docs reads; a failed refresh returns `DOCS_FETCH_FAILED` while serving cached content.
- Fetch a missing cache, force a refresh, or pin a ref: `rvn docs fetch --json`

## Cross-references

- Use `raven-query` for structured retrieval, search, link traversal, and saved queries.
- Use `raven-maintenance` for vault health checks (`rvn check`) and reindexing.
- Use `raven-schema` when the user needs to modify type, field, or trait definitions.

## Safety

- Avoid shell-level `rm`, `mv`, `sed`, or `awk` for managed vault content when
  Raven commands exist. Copying a new external non-Markdown file into the vault
  is supported; run `rvn reindex` afterward. Use `rvn move` for in-vault
  relocation.
- Use `rvn section create` for headings; `rvn add` is body-only and rejects heading content.
- Keep path operations vault-relative where possible.
- If `reclassify` reports dropped fields or missing required values, stop and resolve explicitly.
- Check `rvn backlinks` before deleting objects to avoid orphaned references.

## Load references as needed

- [Command chooser and CLI snippets](references/command-map.md)
- Canonical long-form command guide:
  `rvn docs using-your-vault common-commands --json`
