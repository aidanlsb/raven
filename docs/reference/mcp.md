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

Additional topic resources are available under `raven://guide/<topic>`:

- `raven://guide/critical-rules` - Non-negotiable safety rules for Raven operations
- `raven://guide/getting-started` - First steps for orienting in a new vault
- `raven://guide/core-concepts` - Types, traits, references, and file formats
- `raven://guide/querying` - Raven Query Language (RQL) and query strategy
- `raven://guide/key-workflows` - Common workflows and decision patterns
- `raven://guide/error-handling` - How to respond to tool errors
- `raven://guide/issue-types` - `raven_check` issue reference and fixes
- `raven://guide/best-practices` - Operating principles and safety checks
- `raven://guide/examples` - Example conversations and query translations

### Agent Guide Resources

The agent guide resources (`raven://guide/index` and `raven://guide/<topic>`) provide:
- Getting started sequence for new vaults
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
| `raven_read` | Read raw file content |

### Navigation

| Tool | Description |
|------|-------------|
| `raven_open` | Open a file in the editor |
| `raven_date` | Date hub - all activity for a date |

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
| `raven_workflow_show` | Show workflow details |
| `raven_workflow_render` | Render a workflow with context |

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

Repeatable `--flag key=value` patterns are represented as JSON objects:

```
# CLI
rvn new person "Freya" --field email=freya@asgard.realm --field role=engineer

# MCP
raven_new(type="person", title="Freya", field={"email": "freya@asgard.realm", "role": "engineer"})
```

### Preview/Confirm Pattern

Bulk operations preview by default. Pass `confirm=true` to apply:

```
# Preview (default)
raven_query(query_string="trait:due value==past", apply="set status=overdue")

# Apply
raven_query(query_string="trait:due value==past", apply="set status=overdue", confirm=true)
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
raven_schema(subcommand="type person")
```

**name_field auto-population:**

If a type has `name_field` configured, the title automatically populates that field:

```python
# If person type has name_field: name
raven_new(type="person", title="Freya")
# Creates person with name: Freya in frontmatter

# Check if name_field is configured
raven_schema(subcommand="type person")
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

### Querying

```python
# Object queries
raven_query(query_string="object:project .status==active")
raven_query(query_string="object:person .email==*")

# Trait queries
raven_query(query_string="trait:due value==past")
raven_query(query_string="trait:highlight on:{object:book}")

# Saved queries
raven_query(list=true)  # List available
raven_query(query_string="overdue")  # Run by name

# Full-text search
raven_search(query="meeting notes")
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
raven_query(query_string="trait:due value==past", apply="set status=overdue")

# Apply after user confirmation
raven_query(query_string="trait:due value==past", apply="set status=overdue", confirm=true)

# Other bulk operations
raven_query(query_string="object:project .status==archived", apply="move archive/")
raven_query(query_string="object:project .status==archived", apply="delete")
```

### Schema Operations

```python
# View schema
raven_schema(subcommand="types")
raven_schema(subcommand="type person")

# Add to schema
raven_schema_add_type(name="book", default_path="books/", name_field="title")
raven_schema_add_trait(name="priority", type="enum", values="high,medium,low")
raven_schema_add_field(type_name="person", field_name="company", type="ref", target="company")

# Update schema
raven_schema_update_type(name="person", name_field="name")

# Rename a type (preview first, then confirm)
raven_schema_rename_type(old_name="event", new_name="meeting")  # Preview
raven_schema_rename_type(old_name="event", new_name="meeting", confirm=true)  # Apply
raven_reindex(full=true)  # Always reindex after rename
```

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

# Show details
raven_workflow_show(name="meeting-prep")

# Render with inputs
raven_workflow_render(name="meeting-prep", input={"meeting_id": "meetings/team-sync"})
```

---

## Response Format

All tools return JSON with a consistent envelope:

### Success Response

```json
{
  "success": true,
  "data": { ... }
}
```

### Error Response

```json
{
  "success": false,
  "error": {
    "type": "validation_error",
    "message": "Missing required field: name",
    "details": { ... }
  }
}
```

### Common Error Types

| Type | Description |
|------|-------------|
| `validation_error` | Invalid input or schema validation failure |
| `not_found` | Object or file not found |
| `ambiguous_reference` | Reference matches multiple objects |
| `data_integrity` | Operation blocked to protect data integrity |

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
raven_schema(subcommand="type person")  # Shows name_field if configured
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
