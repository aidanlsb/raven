# Getting Started

Use this guide after quickstart when you need an operational first pass through a vault.

## First-session sequence

0. If no vault exists yet, initialize one:
   `raven_invoke(command="init", args={"path":"/path/to/vault"})`
1. Understand the schema:
   `raven_invoke(command="schema", args={"subcommand":"types"})`
   `raven_invoke(command="schema", args={"subcommand":"traits"})`
2. Get a vault overview:
   `raven_invoke(command="vault_stats")`
3. Check saved queries:
   `raven://queries/saved` or `raven_invoke(command="query_saved_list")`
4. Ensure docs are available locally:
   `raven_invoke(command="docs_list")`
   If this returns `NOT_FOUND`, fetch them: `raven_invoke(command="docs_fetch")`

## Preferred first write flow

```text
create = raven_invoke(command="new", args={"type":"project", "title":"Website Redesign"})
raven_invoke(command="add", args={"text":"## Notes\n- Kickoff next week", "to":create.data.file})
raven_invoke(command="set", args={"object_id":create.data.id, "fields":{"status":"active"}})
```

If the output should converge on reruns, prefer:

```text
raven_invoke(command="upsert", args={
  "type":"report",
  "title":"Weekly Status",
  "content":"# Weekly Status\n..."
})
```

## Import flow

Preview first:

```text
raven_invoke(command="import", args={"type":"project", "file":"projects.json", "dry_run":true})
```

Apply by re-running without `dry_run` (import has no `confirm` flag):

```text
raven_invoke(command="import", args={"type":"project", "file":"projects.json"})
```

## Notes

- Use `raven_describe(command="...")` before invoking unfamiliar commands. The response includes a `description` field with command-specific syntax guidance.
- Prefer registry command IDs in docs and prompts.
- The MCP surface is exactly `raven_discover`, `raven_describe`, and `raven_invoke`. There are no per-command MCP tools.
