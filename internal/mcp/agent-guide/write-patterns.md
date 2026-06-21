# Write Patterns

Use this guide to choose the right mutation primitive.

## Which command to use

| Goal | Command ID | Why |
|------|------------|-----|
| Create a typed item | `new` | Applies schema, templates, and required-field checks |
| Append a note/log entry | `add` | Intentional append-only capture |
| Deterministic create-or-update | `upsert` | Idempotent convergence for generated artifacts |
| Update frontmatter fields | `set` | Schema-validated metadata updates |
| Replace body text safely | `edit` | Unique-string replacement in content markdown (applies immediately; `dry-run` to preview) |
| Move or rename an asset | `move` | Updates Markdown links/images and refreshes the asset index |
| Update trait value | `update` | Targeted trait mutation by trait ID |
| Delete one object | `delete` | Safe deletion behavior with backlink warnings and trash support |

Rules:
- Use `upsert` when reruns should produce one current canonical output.
- Use `add` when history should accumulate.
- Use `new` only when you intend to create a new object identity.

## Recommended sequences

Create then append:

```text
create = raven_invoke(command="new", args={"type":"project", "title":"Website Redesign"})
raven_invoke(command="add", args={"text":"## Notes\n- Kickoff next week", "to":create.data.file})
```

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
  "object_id":"project/website-redesign",
  "fields":{"status":"active"}
})
```

Use `fields` for ordinary literal-style updates. Use `fields-json` when exact JSON typing matters, such as preserving the string `"true"` instead of a boolean or sending arrays/nulls explicitly.

Edit applies immediately; preview only when you need to verify the diff first:

```text
# Applies on the first call:
raven_invoke(command="edit", args={
  "path":"project/website-redesign.md",
  "old_str":"Status: draft",
  "new_str":"Status: active"
})

# Optional dry run to inspect the before/after without writing:
raven_invoke(command="edit", args={
  "path":"project/website-redesign.md",
  "old_str":"Status: draft",
  "new_str":"Status: active",
  "dry-run":true
})
```

Immediate single-object delete:

```text
raven_invoke(command="backlinks", args={"target":"project/old-project"})
raven_invoke(command="delete", args={"object_id":"project/old-project"})
```

Single-object `set`, `add`, `update`, `edit`, `delete`, and `move` all apply
immediately. Only call them after clear user approval or an unambiguous request,
and use `dry-run=true` when you want to confirm the effect first. Bulk operations
(`stdin=true`) stay preview-first and require `confirm=true`.

## Missing reference targets

Writes are permissive: `new`, `upsert`, `set`, `add`, and `edit` succeed even when a
reference (a typed `ref`/`ref-array` field value or a body `[[wikilink]]`) points at a
target that does not exist yet. The write is not blocked.

When a write introduces such a reference, the successful response still carries `ok=true`
and adds:
- `data.missing_refs` — count of missing reference targets.
- `data.missing_ref_items` — the missing references, including an inferred `type` when known
  (same shape as `check create-missing`).
- one `REF_NOT_FOUND` warning per missing target, with `suggested_type` and a `create_command`.

Remediate by creating the targets when appropriate:

```text
raven_invoke(command="check", subcommand="create-missing", args={"confirm":true})
```

or create a specific page directly with the suggested `new` command. Link integrity is a
vault-health concern surfaced by `check`, not a write-time error.

## Practical rules

- If data should be queryable/filterable, prefer frontmatter (`set`, `new`, `upsert`).
- If data is narrative, prefer body content (`add`, `edit`, `upsert content=...`).
- Use `edit` only for vault content files, not `raven.yaml`, `schema.yaml`, or template files.
- Use `move` for assets instead of shell `mv`; asset destinations must include a file extension.
- Prefer raw reads before constructing `old_str` for `edit`.
