# Key Workflows

This guide is an operational playbook for high-value end-to-end flows.

For detailed tool semantics, see:
- `raven://guide/write-patterns`
- `raven://guide/workflow-lifecycle`
- `raven://guide/query-at-scale`
- `raven://guide/response-contract`

## 1. Vault health and cleanup

```text
raven_invoke(command="check", args={"errors_only":true})
raven_invoke(command="check", args={"path":"projects/"})
raven_invoke(command="check", args={"issues":"missing_reference,unknown_type"})
```

Use issue `fix_command` / `fix_hint` from JSON output when available.

## 2. Create and enrich content

```text
create = raven_invoke(command="new", args={"type":"project", "title":"Website Redesign"})
raven_invoke(command="add", args={"text":"## Notes\n- Kickoff next week", "to":create.data.file})
raven_invoke(command="set", args={"object_id":"projects/website-redesign", "fields":{"status":"active"}})
```

## 3. Edit safely

```text
raven_invoke(command="read", args={"path":"projects/website-redesign.md", "raw":true})

# Preview
raven_invoke(command="edit", args={
  "path":"projects/website-redesign.md",
  "old_str":"Status: draft",
  "new_str":"Status: active"
})

# Apply after approval
raven_invoke(command="edit", args={
  "path":"projects/website-redesign.md",
  "old_str":"Status: draft",
  "new_str":"Status: active",
  "confirm":true
})
```

## 4. Move, reclassify, and delete

```text
raven_invoke(command="move", args={"source":"people/loki", "destination":"people/loki-archived"})
raven_invoke(command="reclassify", args={"object":"pages/draft", "new-type":"project"})
```

Deletion flow:

```text
raven_invoke(command="backlinks", args={"target":"projects/old-project"})
raven_invoke(command="delete", args={"object_id":"projects/old-project"})
```

## 5. Bulk mutation flow

```text
# Preview
raven_invoke(command="query", args={
  "query_string":"trait:todo .value==todo",
  "apply":["update done"]
})

# Apply
raven_invoke(command="query", args={
  "query_string":"trait:todo .value==todo",
  "apply":["update done"],
  "confirm":true
})

# Verify
raven_invoke(command="check", args={"trait":"todo"})
```

## 6. Schema evolution flow

```text
raven_invoke(command="schema", args={"subcommand":"types"})
raven_invoke(command="schema", args={"subcommand":"type", "name":"project"})
raven_invoke(command="schema_add_field", args={"type_name":"project", "field_name":"owner", "type":"ref", "target":"person"})
raven_invoke(command="schema_update_type", args={"name":"project", "name-field":"title"})
raven_invoke(command="schema_validate")
raven_invoke(command="reindex", args={"full":true})
```

## 7. Workflow execution flow

```text
raven_invoke(command="workflow_list")
raven_invoke(command="workflow_show", args={"name":"meeting-prep"})
raven_invoke(command="workflow_run", args={"name":"meeting-prep", "input":{"meeting_id":"meetings/team-sync"}})
```

## 8. Query-driven analysis flow

```text
raven_invoke(command="query", args={
  "query_string":"object:meeting refs([[projects/website]])",
  "limit":25,
  "offset":0
})
```

## 9. Import and template setup

```text
raven_invoke(command="import", args={"type":"person", "file":"contacts.json", "dry_run":true})
raven_invoke(command="import", args={"type":"person", "file":"contacts.json", "confirm":true})
raven_invoke(command="template_write", args={"path":"meeting.md", "content":"# {{title}}\n\n## Notes"})
raven_invoke(command="schema_template_set", args={"template_id":"meeting_standard", "file":"templates/meeting.md"})
raven_invoke(command="schema_template_bind", args={"template_id":"meeting_standard", "type":"meeting", "default":true})
```
