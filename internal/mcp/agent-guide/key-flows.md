# Key Flows

This guide is an operational playbook for high-value end-to-end Raven tasks.

For detailed tool semantics, see:
- `raven://guide/write-patterns`
- `raven://guide/query-at-scale`
- `raven://guide/response-contract`

## 0. Switch vaults for the MCP session

```text
raven_invoke(command="vault_focus", args={"name":"work"})
# Subsequent vault-scoped calls use work unless they pass vault/vault_path.
raven_invoke(command="vault_focus", args={"clear":true})
```

Use `args={"path":"/absolute/vault/path"}` to focus an unregistered vault.
Per-call `vault` or `vault_path` overrides focus once. Clearing focus restores
the server's immutable launch pin; on an unpinned server, unqualified calls
return `VAULT_AMBIGUOUS` again.

## 1. Vault health and cleanup

```text
raven_invoke(command="check", args={"errors-only":true})
raven_invoke(command="check", args={"reference":"project/"})
raven_invoke(command="check", args={"issues":"missing_reference,broken_file_link,unknown_type"})
```

Use issue `fix_command` / `fix_hint` from JSON output when available.

`check` and `reindex` operate on Raven-managed content only. Paths matched by
`raven.yaml` `exclude` patterns are intentionally ignored; update `exclude`
with `vault_config_exclude_*` commands if support files need to enter or leave
Raven management.

## 2. Create and enrich content

```text
create = raven_invoke(command="new", args={"type":"project", "title":"Website Redesign"})
notes = raven_invoke(command="section_create", args={"file":create.data.id, "title":"Notes", "level":2})
raven_invoke(command="add", args={"text":"- Kickoff next week", "to":notes.data.section})
raven_invoke(command="set", args={"reference":"project/website-redesign", "fields":{"status":"active"}})
```

## 3. Edit safely

```text
raven_invoke(command="read", args={"reference":"project/website-redesign.md", "raw":true})

# Applies immediately
raven_invoke(command="edit", args={
  "reference":"project/website-redesign.md",
  "old_str":"Status: draft",
  "new_str":"Status: active"
})

# Optional dry run to inspect the diff first
raven_invoke(command="edit", args={
  "reference":"project/website-redesign.md",
  "old_str":"Status: draft",
  "new_str":"Status: active",
  "dry-run":true
})
```

## 4. Move, reclassify, and delete

```text
raven_invoke(command="move", args={"source":"person/loki", "destination":"person/loki-archived"})
raven_invoke(command="move", args={"source":"files/downloads/paper.pdf", "destination":"files/paper.pdf"})
raven_invoke(command="section_move", args={"section_id":"project/website#notes", "after":"project/website#tasks"})
raven_invoke(command="section_rename", args={"section_id":"project/website#tasks", "new_heading_text":"Completed Tasks"})
raven_invoke(command="reclassify", args={"reference":"pages/draft", "new-type":"project"})
raven_invoke(command="reclassify", args={"reference":"person/freya", "new-type":"company", "fields-json":{"legal_name":"false"}})
raven_invoke(command="reclassify", args={"references":["pages/one","pages/two"], "new-type":"project"})
raven_invoke(command="reclassify", args={"references":["pages/one","pages/two"], "new-type":"project", "confirm":true})
```

Deletion flow:

```text
raven_invoke(command="backlinks", args={"reference":"project/old-project"})
raven_invoke(command="delete", args={"reference":"project/old-project"})

# Section deletion previews exact subtree bounds/content and affected backlinks:
raven_invoke(command="section_delete", args={"reference":"project/website#old-plan"})
raven_invoke(command="section_delete", args={"reference":"project/website#old-plan", "confirm":true})
```

Copy external non-Markdown files into the vault directly, then invoke
`reindex`. Use `move` for sources already inside the vault so inbound Markdown
file links are rewritten.

Single-object `delete`, `move`, `section_create`, `section_move`,
`section_rename`, and `reclassify` apply immediately. Run the backlinks check
first for delete when impact is not already clear, or use `dry-run=true` on
commands that expose it to
preview. `section_delete` and bulk delete/move remain preview-first and require
`confirm=true`; bulk object move rejects section IDs. Bulk reclassify follows
the same preview/apply flow and reports required-field or dropped-field blockers
per object.

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

## 7. Query-driven analysis flow

```text
raven_invoke(command="query", args={
  "query_string":"type:meeting refs([[project/website]])",
  "limit":25,
  "offset":0
})
```

Page through large result sets by looping while `has_more` is `true`, sending the
response's `next_offset` as the next request's `offset`. See `raven://guide/query-at-scale`.

## 8. Import and template setup

```text
# Preview first with dry-run, then re-run without dry-run to apply.
raven_invoke(command="import", args={"type":"person", "file":"contacts.json", "dry-run":true})
raven_invoke(command="import", args={"type":"person", "file":"contacts.json"})
raven_invoke(command="template_write", args={"path":"meeting.md", "content":"# {{title}}\n\n## Notes"})
raven_invoke(command="schema_template_set", args={"template_id":"meeting_standard", "file":"templates/meeting.md"})
raven_invoke(command="schema_template_bind", args={"template_id":"meeting_standard", "type":"meeting", "default":true})
```
