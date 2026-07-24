# Getting Started

Use this guide after quickstart when you need an operational first pass through a vault.

## First-session sequence

0. If no vault exists yet, initialize one:
   `raven_invoke(command="init", args={"path":"/path/to/vault"})`
   Init auto-registers the new vault. Read the `post_init` object in the response:
   - `is_first_vault=true` means it is now the default and active vault; you can proceed.
   - `is_first_vault=false` means the additional vault was registered and made active.
     Surface `active_vault`, `previous_active_vault` / `previous_vault`, and the exact
     `switch_back` command before continuing. Changing the default remains an explicit choice.
1. Understand the schema:
   `raven_invoke(command="schema", args={"subcommand":"types"})`
   `raven_invoke(command="schema", args={"subcommand":"traits"})`
2. Get a vault overview:
   `raven_invoke(command="vault_stats")`
3. Check saved queries:
   `raven://queries/saved` or `raven_invoke(command="query_saved_list")`
4. Ensure docs are available in Raven's global docs directory:
   `raven_invoke(command="docs_list")`
   Existing caches from older releases refresh lazily on this read. If refresh
   fails, `DOCS_FETCH_FAILED` warns that Raven served the existing cache.
   If this returns `FILE_NOT_FOUND`, fetch the missing cache:
   `raven_invoke(command="docs_fetch")`

## Preferred first write flow

```text
create = raven_invoke(command="new", args={"type":"project", "title":"Website Redesign"})
notes = raven_invoke(command="section_create", args={"file":create.data.id, "title":"Notes", "level":2})
raven_invoke(command="add", args={"text":"- Kickoff next week", "to":notes.data.section})
raven_invoke(command="set", args={"reference":create.data.id, "fields":{"status":"active"}})
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
