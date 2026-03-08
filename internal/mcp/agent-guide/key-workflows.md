# Key Workflows

This guide is an operational playbook. Use it for high-value end-to-end flows.

For detailed tool semantics, see focused topics:
- `raven://guide/write-patterns`
- `raven://guide/workflow-lifecycle`
- `raven://guide/query-at-scale`
- `raven://guide/response-contract`

## 1. Vault health and cleanup

When users ask "what is broken" or want cleanup:

1. Run a scoped check.
2. Prioritize high-impact errors.
3. Propose fixes with explicit user confirmation.

```text
raven_check(errors_only=true)
raven_check(path="projects/")
raven_check(issues="missing_reference,unknown_type")
```

Use `issue.fix_command` / `issue.fix_hint` from JSON output where available.

## 2. Create and enrich content

Preferred sequence:

1. Create object via schema.
2. Add body content.
3. Set structured fields.

```text
create = raven_new(type="project", title="Website Redesign")
raven_add(text="## Notes\n- Kickoff next week", to=create.data.file)
raven_set(object_id="projects/website-redesign", fields={"status":"active"})
```

If output should be idempotent across reruns, use `raven_upsert` instead of repeated `raven_add`.

## 3. Edit safely

Use preview/apply flow for content edits:

```text
raven_read(path="projects/website-redesign.md", raw=true)

# Preview
raven_edit(path="projects/website-redesign.md", old_str="Status: draft", new_str="Status: active")

# Apply only after explicit approval
raven_edit(path="projects/website-redesign.md", old_str="Status: draft", new_str="Status: active", confirm=true)
```

For metadata changes, prefer `raven_set` over free-form edits.

## 4. Move, reclassify, and delete

Always use Raven primitives so refs/index stay valid:

```text
raven_move(source="people/loki", destination="people/loki-archived")
raven_reclassify(object="pages/draft", new-type="project")
```

Deletion flow:

```text
raven_backlinks(target="projects/old-project")
# Ask for explicit approval after reporting impact
raven_delete(object_id="projects/old-project")
```

Do not suggest blanket rollback commands. If rollback is needed, discuss scope and user intent first.

## 5. Bulk mutation flow

1. Select candidates with a query.
2. Preview bulk apply.
3. Ask for approval.
4. Apply with `confirm=true`.
5. Validate with scoped `raven_check`.

```text
# Preview
raven_query(query_string="trait:todo .value==todo", apply="update done")

# Apply
raven_query(query_string="trait:todo .value==todo", apply="update done", confirm=true)

# Verify
raven_check(trait="todo")
```

## 6. Schema evolution flow

When users need new structure:

```text
raven_schema(subcommand="types")
raven_schema(subcommand="type", name="project")

raven_schema_add_field(type_name="project", field_name="owner", type="ref", target="person")
raven_schema_update_type(name="project", name-field="title")
raven_schema_validate()
raven_reindex(full=true)
```

After schema changes, run validation and reindex before continuing.

## 7. Workflow execution flow

For multi-step automations:

```text
raven_workflow_list()
raven_workflow_show(name="meeting-prep")
raven_workflow_run(name="meeting-prep", input={"meeting_id":"meetings/team-sync"})
```

If the run pauses at an agent step, continue with `raven_workflow_continue`. See `raven://guide/workflow-lifecycle`.

## 8. Query-driven analysis flow

1. Compose a structured query.
2. Narrow with predicates.
3. Use `limit/offset` for large result sets.
4. Read only needed files for synthesis.

```text
raven_query(query_string="object:meeting refs([[projects/website]])", limit=25, offset=0)
```

For large-vault tactics, see `raven://guide/query-at-scale`.

## 9. Import and template setup (common setup tasks)

```text
# Import preview first
raven_import(type="person", file="contacts.json", dry_run=true)
# Apply after approval
raven_import(type="person", file="contacts.json", confirm=true)

# Template setup
raven_template_write(path="meeting.md", content="# {{title}}\n\n## Notes")
raven_schema_template_set(template_id="meeting_standard", file="templates/meeting.md")
raven_schema_type_template_default(type_name="meeting", template_id="meeting_standard")
```

## Related topics

- `raven://guide/critical-rules` - non-negotiable safety constraints
- `raven://guide/response-contract` - errors/warnings/preview semantics
- `raven://guide/write-patterns` - choosing `new` vs `add` vs `upsert`
- `raven://guide/workflow-lifecycle` - run/continue/inspect/prune workflows
- `raven://guide/querying` and `raven://guide/query-at-scale`
