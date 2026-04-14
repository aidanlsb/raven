# Write Patterns

Use this guide to choose the right mutation primitive.

## Which command to use

| Goal | Command ID | Why |
|------|------------|-----|
| Create a typed item | `new` | Applies schema, templates, and required-field checks |
| Append a note/log entry | `add` | Intentional append-only capture |
| Deterministic create-or-update | `upsert` | Idempotent convergence for generated artifacts |
| Update frontmatter fields | `set` | Schema-validated metadata updates |
| Replace body text safely | `edit` | Unique-string replacement in content markdown with preview/confirm |
| Update trait value | `update` | Targeted trait mutation by trait ID |

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

Preview then apply edit:

```text
raven_invoke(command="edit", args={
  "path":"project/website-redesign.md",
  "old_str":"Status: draft",
  "new_str":"Status: active"
})

raven_invoke(command="edit", args={
  "path":"project/website-redesign.md",
  "old_str":"Status: draft",
  "new_str":"Status: active",
  "confirm":true
})
```

## Practical rules

- If data should be queryable/filterable, prefer frontmatter (`set`, `new`, `upsert`).
- If data is narrative, prefer body content (`add`, `edit`, `upsert content=...`).
- Use `edit` only for vault content files, not `raven.yaml`, `schema.yaml`, or template files.
- Prefer raw reads before constructing `old_str` for `edit`.
