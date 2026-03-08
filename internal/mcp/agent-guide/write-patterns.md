# Write Patterns

Use this guide to choose the correct write primitive and keep operations idempotent.

## Choose the right tool

| Goal | Primary Tool | Why |
|------|--------------|-----|
| Create a typed object | `raven_new` | Applies schema + templates + required-field checks. |
| Append a note/log entry | `raven_add` | Intentional append-only capture. |
| Deterministic create-or-update | `raven_upsert` | Idempotent convergence for generated artifacts. |
| Update frontmatter fields | `raven_set` | Schema-validated metadata updates. |
| Replace body text safely | `raven_edit` | Unique-string replacement with preview/confirm. |
| Update trait value | `raven_update` | Targeted trait mutation by trait ID. |

## Idempotency guidance

- Use `raven_upsert` when reruns should produce one current canonical output.
- Use `raven_add` when history/logging should accumulate entries.
- Use `raven_new` only when you intend to create a new object identity.

## Recommended creation flows

### Create structured object + body

```text
create = raven_new(type="project", title="Website Redesign")
raven_add(text="## Notes\n- Kickoff next week", to=create.data.file)
```

### Idempotent generated report

```text
raven_upsert(
  type="report",
  title="Weekly Status",
  field={"status":"draft"},
  content="# Weekly Status\n..."
)
```

### Update metadata only

```text
raven_set(object_id="projects/website-redesign", fields={"status":"active"})
```

### Update content with preview/apply

```text
# Preview (default)
raven_edit(path="projects/website-redesign.md", old_str="Status: draft", new_str="Status: active")

# Apply
raven_edit(path="projects/website-redesign.md", old_str="Status: draft", new_str="Status: active", confirm=true)
```

## Frontmatter vs body decision

- If data should be queryable/filterable: use frontmatter (`raven_set`, `raven_new field=...`, `raven_upsert field=...`).
- If data is narrative/content: use body (`raven_add`, `raven_edit`, `raven_upsert content=...`).

## Safety notes

- Prefer `raven_read(path="...", raw=true)` before building `old_str` for `raven_edit`.
- For bulk writes, preview first and require user approval before `confirm=true`.
- Avoid shell file writes (`echo`, `sed`, `mv`, `rm`) inside vault operations.

## Related topics

- `raven://guide/critical-rules` - non-negotiable safety
- `raven://guide/response-contract` - handling previews/errors/warnings
- `raven://guide/key-workflows` - end-to-end mutation workflows
