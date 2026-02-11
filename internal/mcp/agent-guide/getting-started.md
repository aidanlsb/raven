# Getting Started

When first interacting with a Raven vault, follow this discovery sequence:

1. **Understand the schema**: `raven_schema(subcommand="types")` and `raven_schema(subcommand="traits")`
2. **Get vault overview**: `raven_stats()` to see object counts and structure
3. **Check saved queries**: `raven://queries/saved` or `raven_query(list=true)`
4. **Discover workflows**: `raven://workflows/list` or `raven_workflow_list()`
5. **Grab the query cheatsheet**: `raven://guide/query-cheatsheet` for common patterns
6. **Ask about existing data**: if they have JSON exports, use `raven_import` to seed the vault quickly

You can also fetch the `raven://schema/current` MCP resource for the complete schema.yaml.

## Fast start from existing JSON

If the user already has structured data from another tool, prefer import over manual re-entry:

```
# Preview first
raven_import(type="project", file="projects.json", dry_run=true)

# Apply after user confirmation
raven_import(type="project", file="projects.json", confirm=true)
```

## Creating new notes (recommended flow)

Raven is intentionally **not** a free-form file writer. Agents should create new files via the schema, then append content to them:

1. **Create the file** with `raven_new` (applies templates + schema validation)
2. **Append content** with `raven_add` using the returned `data.file` path

Example:

```
# Create a new object file
create = raven_new(type="project", title="Website Redesign")

# Append body content to that file (must already exist)
raven_add(text="## Notes\n- Kickoff next week", to=create.data.file)
```

Notes:
- `raven_add` can auto-create **daily notes**; for other files it requires the file to already exist.
- If `raven_new` fails in MCP mode, it will usually return `ok:false` with an error code like `MISSING_ARGUMENT` (title required) or `REQUIRED_FIELD_MISSING` (see `error.details.retry_with`).

## Freeform notes (use built-in `page` type)

If a user asks to create a note that **doesn't fit an existing schema type**, use the built-in `page` type:

```
create = raven_new(type="page", title="Quick Note")
raven_add(text="## Notes\n- ...", to=create.data.file)
```

Tip:
- For long files, `raven_read` supports `start_line`/`end_line` (**1-indexed, inclusive**) and `lines=true` for copy-paste-safe anchors.
