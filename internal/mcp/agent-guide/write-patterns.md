# Write Patterns

Use this guide to choose the right mutation primitive.

## Which command to use

| Goal | Command ID | Why |
|------|------------|-----|
| Create a typed item | `new` | Applies schema, templates, and required-field checks |
| Append a note/log entry | `add` | Intentional append-only capture |
| Deterministic create-or-update | `upsert` | Idempotent convergence for generated artifacts |
| Update frontmatter fields | `set` | Schema-validated metadata updates |
| Change object types | `reclassify` | Applies target-type fields/defaults and safely moves files |
| Replace body text safely | `edit` | Unique-string replacement in content markdown (applies immediately; `dry-run` to preview) |
| Add an external non-Markdown file | Copy into vault, then `reindex` | Files are not Raven entities |
| Move or rename a non-Markdown file | `move` | Updates inbound Markdown links/images |
| Create a section heading | `section_create` | Plain title + required level; returns canonical `file#slug` |
| Reorder/reparent a section | `section_move` | Moves the complete subtree without changing heading identity |
| Rename a section heading | `section_rename` | Source `file#section`, destination plain heading text; rewrites inbound `[[...#slug]]` refs |
| Delete a section subtree | `section_delete` | Section-only, preview-first deletion with exact bounds and affected backlinks |
| Update trait value | `update` | Targeted trait mutation by trait ID |
| Delete one object or file | `delete` | Safe deletion behavior with backlink warnings for objects and trash support |

Rules:
- Use `upsert` when reruns should produce one current canonical output.
- Use `add` when history should accumulate.
- Use `new` only when you intend to create a new object identity.

## Section-targeted writes

- Create a heading: `section_create` with `file`, plain `title`, and required integer `level`. Optionally pass exactly one of `after`, `before`, or `under`; no anchor appends at EOF.
- Reorder/reparent without renaming: `section_move` with `section_id` and optionally one of `after`, `before`, or `under`. The source's complete subtree moves.
- Delete a heading and every descendant: `section_delete` with `reference="file#section"`. It previews by default and reports `line_start`, `line_end`, exact `removed_content`, `deleted_sections`, and affected `backlinks`. Apply with `confirm=true`. Reported backlinks are left unchanged because Raven cannot infer a safe replacement.
- `after` uses the anchor's complete subtree boundary; `before` uses its heading line; `under` inserts as its last direct child. Sibling levels must match, and a direct child must be exactly one level deeper. Depth mismatches are hard errors—never retry with an inferred level.
- Append body content inside a section: `add` with `to="file#section"`. Insertion happens at the end of the section's direct content, before any child headings.
- `add` rejects Markdown heading content. Its removed `heading` and `create-heading` args are invalid; use `section_create`, then `add` to the returned section ID.
- Bulk-append into sections: in MCP, call `add` with `text`, an `object_ids`
  array of section IDs, and `confirm=true`; in the CLI, pipe a `section` query's
  `--ids` output into `rvn add --stdin --confirm`.
- Rename a heading safely: `section_rename` with `section_id="file#section"` and `new_heading_text` set to plain heading text. Never rename headings with `move` (it rejects section sources) or `edit` (it leaves `stale_fragment` refs when other files link to the section).
- Edit inside a section: `edit` with `reference="file#section"` scopes replacements to the section's subtree (section plus child sections).

## Recommended sequences

Create then append:

```text
create = raven_invoke(command="new", args={"type":"project", "title":"Website Redesign"})
notes = raven_invoke(command="section_create", args={"file":create.data.id, "title":"Notes", "level":2})
raven_invoke(command="add", args={"text":"- Kickoff next week", "to":notes.data.section})
```

`new`, `upsert`, and `daily` return an identity pair: `data.file` (vault-relative
path) and `data.id` (the canonical object ID). Use `data.id` for `[[refs]]` and for
follow-up commands like `set`/`edit`/`delete`; never derive an ID from `data.file`,
since the two often differ (`type/person/freya.md` links as `person/freya`; a daily
note links as the bare date). See `raven://guide/response-contract`.

Idempotent generated artifact:

```text
raven_invoke(command="upsert", args={
  "type":"report",
  "title":"Weekly Status",
  "content":"# Weekly Status\n..."
})
```

Metadata update:

```text
raven_invoke(command="set", args={
  "reference":"project/website-redesign",
  "fields":{"status":"active"}
})
```

Use `fields` for ordinary literal-style updates. Use `fields-json` when exact JSON typing matters, such as preserving the string `"true"` instead of a boolean or sending arrays/nulls explicitly.

Edit applies immediately; preview only when you need to verify the diff first:

```text
# Applies on the first call:
raven_invoke(command="edit", args={
  "reference":"project/website-redesign.md",
  "old_str":"Status: draft",
  "new_str":"Status: active"
})

# Optional dry run to inspect the before/after without writing:
raven_invoke(command="edit", args={
  "reference":"project/website-redesign.md",
  "old_str":"Status: draft",
  "new_str":"Status: active",
  "dry-run":true
})
```

Immediate single-object delete:

```text
raven_invoke(command="backlinks", args={"reference":"project/old-project"})
raven_invoke(command="delete", args={"reference":"project/old-project"})
```

For a non-Markdown file, pass its explicit vault-relative path:

```text
raven_invoke(command="delete", args={"reference":"files/old-paper.pdf", "dry-run":true})
```

Copy an external file into the vault with the host filesystem, then invoke
`reindex`. Use `move` when the source is already in the vault.

Single-object `set`, `add`, `update`, `edit`, `section_create`, `section_move`,
`section_rename`, `delete`, `move`, and `reclassify` all apply immediately. Only
call them after clear user approval or an unambiguous request, and use
`dry-run=true` when the command exposes it and you want to confirm the effect
first. `section_delete` is always preview-first and requires `confirm=true`.
Bulk operations (`stdin=true`) also stay preview-first and require
`confirm=true`.

For bulk reclassification, pass `references` plus `new-type`. The preview reports
planned moves, dropped/added fields, required-field failures, and reference
rewrites. Applying requires `confirm=true`; items that drop fields also require
`force=true`.

Regardless of command or flags, read `meta.mutation.phase` to confirm what
happened: `"applied"` means the change was written, `"preview"` means nothing was
written yet. See `raven://guide/response-contract` for the full contract.

## Missing reference targets

Writes are permissive: `new`, `upsert`, `set`, `add`, and `edit` succeed even when a
reference (a typed `ref`/`ref-array` field value or a body `[[wikilink]]`) points at a
target that does not exist yet. The write is not blocked.

When a write introduces such a reference, the successful response still carries `ok=true`
and adds:
- `data.missing_refs` — count of missing reference targets.
- `data.missing_ref_items` — the missing references, including an inferred `type` when known
  (same shape as `check create-missing`).
- one `REF_TARGET_MISSING` warning per missing target, with `suggested_type`, a
  `create_command` string, and a structured `create_invoke` (`{command, args}`) you can
  pass straight to `raven_invoke` without shell-parsing. (`REF_TARGET_MISSING` is a
  benign write-time warning — distinct from the fatal `REF_NOT_FOUND` read/resolve error.)

Remediate by creating the targets when appropriate:

```text
raven_invoke(command="check create-missing", args={"confirm":true})
```

or create a specific page directly with the suggested `new` command. Link integrity is a
vault-health concern surfaced by `check`, not a write-time error.

## Practical rules

- If data should be queryable/filterable, prefer frontmatter (`set`, `new`, `upsert`).
- If data is narrative, prefer body content (`add`, `edit`, `upsert content=...`).
- Use `edit` only for vault content files, not `raven.yaml`, `schema.yaml`, or template files.
- Use `move` for in-vault file relocation; non-Markdown destinations must include a file extension.
- Copy external non-Markdown files into the vault directly, then invoke `reindex`.
- Prefer raw reads before constructing `old_str` for `edit`.
