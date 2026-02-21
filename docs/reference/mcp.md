# MCP Reference

Raven exposes its CLI commands as MCP (Model Context Protocol) tools via `rvn serve`.

## Starting the Server

```bash
rvn serve --vault-path /path/to/vault
```

The server runs over stdin/stdout and exposes Raven tools to MCP clients.

## Claude Desktop Configuration

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "raven": {
      "command": "/path/to/rvn",
      "args": ["serve", "--vault-path", "/path/to/vault"]
    }
  }
}
```

Replace `/path/to/rvn` with the actual binary path (find with `which rvn`).

---

## MCP Resources

Raven exposes MCP resources that agents can fetch:

| URI | Name | Description |
|-----|------|-------------|
| `raven://guide/index` | Agent Guide Index | Overview of available agent guide topics |
| `raven://schema/current` | Current Schema | The vault's `schema.yaml` defining types and traits |
| `raven://queries/saved` | Saved Queries | List of saved queries defined in `raven.yaml` |
| `raven://workflows/list` | Workflows List | List of workflows defined in `raven.yaml` |
| `raven://workflows/<name>` | Workflow Details | Details for a specific workflow |

Additional topic resources are available under `raven://guide/<topic>`:

- `raven://guide/critical-rules` - Non-negotiable safety rules for Raven operations
- `raven://guide/onboarding` - Interactive vault setup for new users
- `raven://guide/getting-started` - First steps for orienting in a new vault
- `raven://guide/core-concepts` - Types, traits, references, and file formats
- `raven://guide/querying` - Raven Query Language (RQL) and query strategy
- `raven://guide/query-cheatsheet` - Common query patterns and shortcuts
- `raven://guide/key-workflows` - Common workflows and decision patterns
- `raven://guide/error-handling` - How to respond to tool errors
- `raven://guide/issue-types` - `raven_check` issue reference and fixes
- `raven://guide/best-practices` - Operating principles and safety checks
- `raven://guide/examples` - Example conversations and query translations

### Agent Guide Resources

The agent guide resources (`raven://guide/index` and `raven://guide/<topic>`) provide:
- Getting started sequence for new vaults
- Onboarding flow for first-time vault setup
- Query language syntax and composition patterns
- Key workflows (creating, editing, querying, bulk operations)
- Error handling patterns
- Best practices and example conversations

Agents should fetch the index for discovery, then pull only the topic resources they need.

### Schema Resource

The schema resource (`raven://schema/current`) returns the raw `schema.yaml` content, giving agents full visibility into:
- Available types and their fields
- Trait definitions
- Field constraints (required, enums, refs)

---

## Tool Discovery

Tools are generated from Raven's command registry. To see the full list:

```bash
rvn schema commands --json
```

---

## Available Tools

### Content Creation

| Tool | Description |
|------|-------------|
| `raven_new` | Create a new typed object |
| `raven_upsert` | Create or update a typed object idempotently |
| `raven_import` | Import objects from JSON data (create/update) |
| `raven_add` | Append content to existing file or daily note |
| `raven_daily` | Open or create a daily note |

### Content Modification

| Tool | Description |
|------|-------------|
| `raven_set` | Set frontmatter fields on an object |
| `raven_edit` | Surgical text replacement in files |
| `raven_delete` | Delete an object (moves to trash) |
| `raven_move` | Move or rename an object |

### Querying

| Tool | Description |
|------|-------------|
| `raven_query` | Query objects or traits using RQL |
| `raven_search` | Full-text search across vault |
| `raven_backlinks` | Find objects that reference a target |
| `raven_read` | Read a file (raw or enriched) |
| `raven_resolve` | Resolve a reference to its target object |

**Full-text search note:** if you see SQLite/FTS errors (e.g. `SQL logic error: no such column: ...`) when using `raven_search`, quote special/hyphenated tokens:

`"michael-truell" OR "Michael Truell"`

### Navigation

| Tool | Description |
|------|-------------|
| `raven_open` | Open a file in the editor |
| `raven_date` | Date hub - all activity for a date |

### Learning

| Tool | Description |
|------|-------------|
| `raven_learn` | Browse lesson overview and completion state |
| `raven_learn_list` | Explicit lesson list/overview |
| `raven_learn_open` | Open lesson content and advisory prerequisites |
| `raven_learn_done` | Mark a lesson complete |
| `raven_learn_next` | Get the next suggested lesson |
| `raven_learn_validate` | Validate lesson catalog integrity |

### Vault Management

| Tool | Description |
|------|-------------|
| `raven_check` | Validate vault against schema |
| `raven_stats` | Show vault statistics |
| `raven_untyped` | List pages without explicit type |
| `raven_reindex` | Rebuild the index |

### Schema Management

| Tool | Description |
|------|-------------|
| `raven_schema` | Introspect the schema |
| `raven_schema_add_type` | Add a new type |
| `raven_schema_add_trait` | Add a new trait |
| `raven_schema_add_field` | Add a field to a type |
| `raven_schema_update_type` | Update a type |
| `raven_schema_update_trait` | Update a trait |
| `raven_schema_update_field` | Update a field |
| `raven_schema_remove_type` | Remove a type |
| `raven_schema_remove_trait` | Remove a trait |
| `raven_schema_remove_field` | Remove a field |
| `raven_schema_rename_type` | Rename a type and update all references |
| `raven_schema_rename_field` | Rename a field and update all references |
| `raven_template_list` | List configured templates |
| `raven_template_get` | Get template binding and content |
| `raven_template_set` | Set template file binding |
| `raven_template_scaffold` | Create template file and register binding |
| `raven_template_write` | Replace bound template file content |
| `raven_template_remove` | Remove template binding |
| `raven_template_render` | Preview rendered template content |
| `raven_schema_validate` | Validate schema correctness |

### Saved Queries

| Tool | Description |
|------|-------------|
| `raven_query_add` | Add a saved query |
| `raven_query_remove` | Remove a saved query |

### Workflows

| Tool | Description |
|------|-------------|
| `raven_workflow_list` | List available workflows |
| `raven_workflow_add` | Add a workflow definition to `raven.yaml` |
| `raven_workflow_scaffold` | Scaffold a starter workflow file and config entry |
| `raven_workflow_remove` | Remove a workflow definition from `raven.yaml` |
| `raven_workflow_validate` | Validate workflow definitions |
| `raven_workflow_show` | Show workflow details |
| `raven_workflow_run` | Run a workflow until an agent step |
| `raven_workflow_continue` | Continue a paused workflow run |
| `raven_workflow_runs_list` | List persisted workflow runs |
| `raven_workflow_runs_step` | Fetch output for a specific workflow run step |
| `raven_workflow_runs_prune` | Prune persisted workflow runs |

### Skills

| Tool | Description |
|------|-------------|
| `raven_skill_list` | List bundled Raven skills |
| `raven_skill_install` | Install a bundled skill for a target runtime |
| `raven_skill_remove` | Remove an installed skill from a target runtime |
| `raven_skill_doctor` | Inspect skill install roots and installed skills |

---

## Tool Parameter Conventions

### Positional Arguments

CLI positional arguments become top-level tool properties:

```
# CLI
rvn new person "Freya"

# MCP
raven_new(type="person", title="Freya")
```

### Key-Value Flags

Repeatable `--flag key=value` patterns accept multiple input forms for MCP compatibility:
- object: `{"email":"freya@asgard.realm","role":"engineer"}`
- single string: `"email=freya@asgard.realm"`
- string array: `["email=freya@asgard.realm","role=engineer"]`

```
# CLI
rvn new person "Freya" --field email=freya@asgard.realm --field role=engineer

# MCP
raven_new(type="person", title="Freya", field={"email": "freya@asgard.realm", "role": "engineer"})
raven_new(type="person", title="Freya", field="email=freya@asgard.realm,role=engineer")
```

### JSON Flags

Flags ending in `-json` accept either:
- a structured object, or
- a JSON-encoded string

```
# Object form
raven_workflow_continue(
  run_id="wrf_abc123",
  agent_output_json={"outputs": {"markdown": "done"}}
)

# String form (works in clients that only pass primitive arguments)
raven_workflow_continue(
  run_id="wrf_abc123",
  agent_output_json='{"outputs":{"markdown":"done"}}'
)
```

### Preview/Confirm Pattern

Bulk operations preview by default. Pass `confirm=true` to apply:

```
# Preview (default)
raven_query(query_string="trait:due .value==past", apply="set status=overdue")

# Apply
raven_query(query_string="trait:due .value==past", apply="set status=overdue", confirm=true)
```

---

## Common Tool Examples

### Creating Objects

```python
# Simple object
raven_new(type="person", title="Freya")

# With fields
raven_new(type="person", title="Freya", field={"email": "freya@asgard.realm"})

# Check required fields first
raven_schema(subcommand="type", name="person")
```

**name_field auto-population:**

If a type has `name_field` configured, the title automatically populates that field:

```python
# If person type has name_field: name
raven_new(type="person", title="Freya")
# Creates person with name: Freya in frontmatter

# Check if name_field is configured
raven_schema(subcommand="type", name="person")
```

### Creating a new file + adding body content (recommended agent flow)

Raven is intentionally **not** a free-form file writer. The recommended pattern is:

1. **Create the file via schema** (`raven_new`) so frontmatter/templates are applied
2. **Append content** (`raven_add`) to the created file

```python
# 1) Create
create = raven_new(type="project", title="Website Redesign")

# 2) Append (use the returned relative path)
# create.data.file will look like "projects/website-redesign.md"
raven_add(text="## Notes\n- Kickoff next week", to=create.data.file)
```

### Freeform notes (use built-in `page` type)

If a user asks for a new note that **does not fit an existing schema type**, create a built-in `page` object and then append content as needed:

```python
create = raven_new(type="page", title="Quick Note")
raven_add(text="## Notes\n- ...", to=create.data.file)
```

### Quick Capture

```python
# To daily note (default)
raven_add(text="Quick thought")

# With traits
raven_add(text="@priority(high) Urgent task")

# To specific file
raven_add(text="Meeting notes", to="projects/website.md")
```

### Importing Structured Data

Use `raven_import` when a user already has structured JSON data (exports, API dumps, migrations).

```python
# Preview a homogeneous import (single type)
raven_import(type="person", file="contacts.json", dry_run=true)

# Apply after user confirmation
raven_import(type="person", file="contacts.json", confirm=true)

# Heterogeneous import via mapping file (mixed source types)
raven_import(mapping="migration.yaml", file="dump.json", dry_run=true)
raven_import(mapping="migration.yaml", file="dump.json", confirm=true)
```

Notes:
- Prefer `dry_run=true` first, then show the preview and ask before `confirm=true`.
- In MCP usage, `file` + `mapping` is usually better than stdin pipelines.
- Use `content_field` (or mapping `content_field`) when JSON should become page body content.

### Querying

```python
# Object queries
raven_query(query_string="object:project .status==active")
raven_query(query_string="object:person .email==*")

# Trait queries
raven_query(query_string="trait:due .value==past")
raven_query(query_string="trait:highlight on(object:book)")

# Saved queries
raven_query(list=true)  # List available
raven_query(query_string="overdue")  # Run by name

# Full-text search
raven_search(query="meeting notes")
```

### Getting file paths (for editing/navigation)

- **From `raven_new`**: the response includes `data.file` (vault-relative path) and `data.id` (object ID).
- **From `raven_query`**:
  - Object queries include `items[].file_path` and `items[].line`
  - Trait queries include `items[].file_path` and `items[].line`

### Reading long files for safe edits

For long files, prefer raw reads with line ranges and/or structured lines to avoid transcription errors when preparing `raven_edit(old_str=...)`:

```python
# Raw slice of a file (1-indexed inclusive lines)
raven_read(path="projects/website.md", raw=true, start_line=10, end_line=40)

# Structured lines (copy-paste-safe anchors)
raven_read(path="projects/website.md", raw=true, lines=true, start_line=10, end_line=40)
```

### Updating Objects

```python
# Set frontmatter fields
raven_set(object_id="people/freya", fields={"email": "freya@asgard.realm", "status": "active"})

# Edit content
raven_edit(path="projects/website.md", old_str="Status: draft", new_str="Status: published")
raven_edit(path="projects/website.md", old_str="Status: draft", new_str="Status: published", confirm=true)
```

### Bulk Operations

```python
# Preview changes
raven_query(query_string="trait:due .value==past", apply="set status=overdue")

# Apply after user confirmation
raven_query(query_string="trait:due .value==past", apply="set status=overdue", confirm=true)

# Other bulk operations
raven_query(query_string="object:project .status==archived", apply="move archive/")
raven_query(query_string="object:project .status==archived", apply="delete")
```

### Schema Operations

```python
# View schema
raven_schema(subcommand="types")
raven_schema(subcommand="type", name="person")

# Add to schema
raven_schema_add_type(name="book", default_path="books/", name_field="title", description="Books and long-form reading material")
raven_schema_add_trait(name="priority", type="enum", values="high,medium,low")
raven_schema_add_field(type_name="person", field_name="company", type="ref", target="company", description="Employer or organization")

# Update schema
raven_schema_update_type(name="person", name_field="name")
raven_schema_update_field(type_name="person", field_name="company", description="-")  # Remove description

# Rename a type (preview first, then confirm)
raven_schema_rename_type(old_name="event", new_name="meeting")  # Preview
raven_schema_rename_type(old_name="event", new_name="meeting", confirm=true)  # Apply
raven_reindex(full=true)  # Always reindex after rename

# Manage templates (file-backed lifecycle)
raven_template_list()
raven_template_scaffold(target="type", type_name="meeting")
raven_template_set(target="type", type_name="meeting", file="templates/meeting.md")
raven_template_write(target="type", type_name="meeting", content="# {{title}}\n\n## Notes\n")
raven_template_render(target="type", type_name="meeting", title="Weekly Standup", field={"time": "10:00 AM"})
raven_template_get(target="daily")
raven_template_set(target="daily", file="templates/daily.md")
raven_template_render(target="daily", date="tomorrow")
raven_template_remove(target="type", type_name="meeting")

# Resolve references
raven_resolve(reference="freya")         # Short name → people/freya
raven_resolve(reference="today")         # Dynamic date → daily/2026-02-07
```

Notes:
- Templates are file-backed only (no inline template bodies).
- Template files must be under `directories.template` (default: `templates/`).
- Use `raven_template_scaffold` for first-time setup, then `raven_template_write` to update content.

### Vault Health

```python
# Check entire vault
raven_check()

# Check a specific file (by path or reference)
raven_check(path="people/freya.md")
raven_check(path="freya")

# Check a directory
raven_check(path="projects/")

# Check all objects of a type
raven_check(type="project")

# Check all usages of a trait
raven_check(trait="due")

# Only check specific issue types
raven_check(issues="missing_reference,unknown_type")

# Exclude certain issue types
raven_check(exclude="unused_type,unused_trait")

# Only show errors (skip warnings)
raven_check(errors_only=true)

# Reindex after changes
raven_reindex()
raven_reindex(full=true)  # Full rebuild

# Statistics
raven_stats()
```

### Workflows

```python
# List available
raven_workflow_list()

# Scaffold a starter workflow (recommended first step)
raven_workflow_scaffold(name="daily-brief")

# Register a workflow file (MCP-safe; no manual raven.yaml editing)
raven_workflow_add(
  name="daily-brief",
  file="workflows/daily-brief.yaml"
)

# Validate syntax
raven_workflow_validate(name="daily-brief")

# Show details
raven_workflow_show(name="meeting-prep")

# Run with inputs
raven_workflow_run(name="meeting-prep", input={"meeting_id": "meetings/team-sync"})

# Fetch one step output incrementally
raven_workflow_runs_step(run_id="wrf_abcd1234", step_id="todos")
```

Notes:
- `raven_workflow_add` is file-only; inline definitions are not supported
- Workflow files must be under `directories.workflow` (default `workflows/`)

### Skills

```python
# List available bundled skills
raven_skill_list()

# Preview install (default)
raven_skill_install(name="raven-core", target="codex")

# Apply install
raven_skill_install(name="raven-core", target="codex", confirm=true)

# Inspect target install health
raven_skill_doctor(target="codex")
```

Notes:
- Skill install/remove are preview-first; pass `confirm=true` to apply.
- Supported targets are `codex`, `claude`, and `cursor`.

---

## Response Format

All Raven tools return JSON with a consistent envelope in their stdout:

### Success Response

```json
{
  "ok": true,
  "data": { ... },
  "warnings": [ ... ],
  "meta": { ... }
}
```

### Error Response

```json
{
  "ok": false,
  "error": { "code": "MISSING_ARGUMENT", "message": "title is required" }
}
```

### Notes for MCP clients/agents

- The MCP server returns tool output as **text**; that text is the Raven JSON envelope shown above.
- Many mutation commands **preview by default** and require `confirm=true` to apply (`raven_edit`, `raven_query(apply=..., confirm=true)`, and bulk `--stdin` modes).

### Common Error Codes

| Code | Description |
|------|-------------|
| `MISSING_ARGUMENT` | Missing required argument (common in non-interactive/MCP mode) |
| `TYPE_NOT_FOUND` | Unknown type (check `raven_schema(subcommand="types")`) |
| `REF_NOT_FOUND` | Reference/path doesn't resolve |
| `REF_AMBIGUOUS` | Short reference matches multiple objects |
| `REQUIRED_FIELD_MISSING` | Missing required schema field(s); see `error.details.retry_with` when present |

---

## Agent Best Practices

See [`internal/mcp/agent-guide/index.md`](../../internal/mcp/agent-guide/index.md) for comprehensive agent guidelines (also available via the `raven://guide/index` and `raven://guide/<topic>` MCP resources). Key points:

1. **Check schema first** — Use `raven_schema` to understand types and required fields before creating objects

2. **Prefer structured queries** — Use `raven_query` with RQL before falling back to `raven_search`

3. **Preview bulk operations** — Always run without `confirm=true` first, show the preview to the user

4. **Check before creating** — Use `raven_search` or `raven_backlinks` to avoid duplicates

5. **Reindex after schema changes** — Run `raven_reindex(full=true)` after modifying the schema

6. **Confirm deletions** — Always check `raven_backlinks` and confirm with the user before deleting

---

## name_field Feature

Types can specify a `name_field` which:

1. **Auto-populates from title** — The `title` argument to `raven_new` sets this field automatically
2. **Enables semantic resolution** — References like `[[The Prose Edda]]` can find the book by its title field

### Checking name_field

```python
raven_schema(subcommand="types")  # Shows hints for types without name_field
raven_schema(subcommand="type", name="person")  # Shows name_field if configured
```

### Setting name_field

```python
# When creating a type
raven_schema_add_type(name="book", name_field="title")

# On existing type
raven_schema_update_type(name="person", name_field="name")
```

### Usage

```python
# With name_field: name on person type
raven_new(type="person", title="Freya")
# Creates: people/freya.md with name: Freya in frontmatter

# Without name_field (must provide field explicitly)
raven_new(type="person", title="Freya", field={"name": "Freya"})
```

---

## description Feature

Types and fields can include optional `description` values for extra context.

```python
# Set descriptions
raven_schema_add_type(name="book", description="Books and long-form reading material")
raven_schema_add_field(type_name="book", field_name="author", type="ref", target="person", description="Primary author")

# Update/remove descriptions
raven_schema_update_type(name="book", description="Reading and reference materials")
raven_schema_update_field(type_name="book", field_name="author", description="-")  # Remove

# Read descriptions in schema introspection output
raven_schema(subcommand="type", name="book")
```
