# CLI Reference

Complete reference for all Raven CLI commands. For usage patterns and workflows, see `guide/cli.md`.

## Global Flags

These flags apply to all commands:

| Flag | Short | Description |
|------|-------|-------------|
| `--vault` | `-v` | Named vault from config |
| `--vault-path` | | Explicit path to vault directory |
| `--config` | | Path to config file |
| `--json` | | Output in JSON format (for agent/script use) |

Vault resolution order:
1. `--vault-path` (explicit path)
2. `--vault` (named vault from config)
3. `default_vault` in config
4. Error if none specified

---

## Core Commands

### `rvn init`

Initialize a new vault.

```bash
rvn init /path/to/vault
```

Creates:
- `schema.yaml` — types and traits
- `raven.yaml` — vault config
- `.raven/` — local index (gitignored)

---

### `rvn new`

Create a new typed object.

```bash
rvn new <type> <title> [--field key=value...]
```

| Argument | Description |
|----------|-------------|
| `type` | Object type (e.g., `person`, `project`) |
| `title` | Title/name for the object |

| Flag | Description |
|------|-------------|
| `--field` | Set field value (repeatable) |

**Examples:**

```bash
rvn new person "Freya"
rvn new project "Website Redesign"
rvn new book "The Prose Edda" --field author=people/snorri
```

**Notes:**
- If the type has a `name_field` configured, the title automatically populates that field
- Required fields (from schema) must be provided via `--field` or prompted interactively
- In `--json` mode, title is required (no prompting)

---

### `rvn add`

Append content to an existing file or daily note.

```bash
rvn add <text> [--to file] [--timestamp]
```

| Argument | Description |
|----------|-------------|
| `text` | Text to add (can include `@traits` and `[[refs]]`) |

| Flag | Description |
|------|-------------|
| `--to` | Target existing file path (must exist) |
| `--timestamp` | Prefix with current time (HH:MM) |
| `--stdin` | Read object IDs from stdin for bulk operations |
| `--confirm` | Apply bulk changes (preview-only by default) |

**Examples:**

```bash
rvn add "Quick thought"
rvn add "@priority(high) Urgent task"
rvn add "Note" --to projects/website.md
rvn add "Call Odin" --timestamp
```

**Notes:**
- Default destination is today's daily note
- Target file must already exist (use `rvn new` to create new files)

---

### `rvn set`

Set frontmatter fields on an object.

```bash
rvn set <object_id> <field=value...>
```

| Argument | Description |
|----------|-------------|
| `object_id` | Object to update (e.g., `people/freya`) |
| `field=value` | Field assignments (can specify multiple) |

| Flag | Description |
|------|-------------|
| `--stdin` | Read object IDs from stdin for bulk operations |
| `--confirm` | Apply bulk changes (preview-only by default) |

**Examples:**

```bash
rvn set people/freya email=freya@asgard.realm
rvn set people/freya name="Freya" status=active
rvn set projects/website priority=high
```

**Notes:**
- Object ID can be a full path or short reference (if unambiguous)
- Field values are validated against the schema
- Supports both file-level and embedded object IDs

---

### `rvn edit`

Surgical text replacement in vault files.

```bash
rvn edit <path> <old_str> <new_str> [--confirm]
```

| Argument | Description |
|----------|-------------|
| `path` | File path relative to vault root |
| `old_str` | String to replace (must be unique in file) |
| `new_str` | Replacement string (can be empty to delete) |

| Flag | Description |
|------|-------------|
| `--confirm` | Apply the edit (preview-only by default) |

**Examples:**

```bash
rvn edit "daily/2025-12-27.md" "- Churn analysis" "- [[churn-analysis|Churn analysis]]"
rvn edit "pages/notes.md" "reccommendation" "recommendation" --confirm
rvn edit "daily/2026-01-02.md" "- old task" "" --confirm  # Delete
```

**Notes:**
- The old string must appear exactly once to prevent ambiguous edits
- Whitespace matters — old_str must match exactly including indentation
- For multi-line replacements, include newlines in both strings

---

### `rvn delete`

Delete an object from the vault.

```bash
rvn delete <object_id> [--force]
```

| Argument | Description |
|----------|-------------|
| `object_id` | Object ID to delete (e.g., `people/freya`) |

| Flag | Description |
|------|-------------|
| `--force` | Skip confirmation prompt |
| `--stdin` | Read object IDs from stdin for bulk operations |
| `--confirm` | Apply bulk changes (preview-only by default) |

**Examples:**

```bash
rvn delete people/freya
rvn delete projects/old --force
```

**Notes:**
- By default, files are moved to `.trash/` (not permanently deleted)
- Warns about backlinks (objects that reference the deleted item)
- Configure behavior via `deletion.behavior` in `raven.yaml`

---

### `rvn move`

Move or rename an object within the vault.

```bash
rvn move <source> <destination> [flags]
```

| Argument | Description |
|----------|-------------|
| `source` | Source file path (e.g., `inbox/note.md` or `people/loki`) |
| `destination` | Destination path (e.g., `people/loki-archived` or `archive/projects/`) |

| Flag | Description |
|------|-------------|
| `--update-refs` | Update references to moved file (default: true) |
| `--skip-type-check` | Skip type-directory mismatch warning |
| `--force` | Skip confirmation prompts |
| `--stdin` | Read object IDs from stdin for bulk operations |
| `--confirm` | Apply bulk changes (preview-only by default) |

**Examples:**

```bash
rvn move people/loki people/loki-archived
rvn move inbox/task.md projects/website/task.md
rvn move drafts/person.md people/freya.md --update-refs
```

**Notes:**
- Both source and destination must be within the vault (security constraint)
- References are updated automatically by default
- Warns if moving to a type's default directory with mismatched type

---

## Query Commands

### `rvn query`

Query objects or traits using the Raven query language.

```bash
rvn query <query_string> [flags]
```

| Argument | Description |
|----------|-------------|
| `query_string` | Query string or saved query name |

| Flag | Description |
|------|-------------|
| `--list` | List available saved queries |
| `--refresh` | Refresh stale files before query (auto-reindex changed files) |
| `--ids` | Output only object/trait IDs, one per line (for piping) |
| `--object-ids` | Output object IDs (for trait queries, outputs containing object IDs) |
| `--apply` | Apply bulk operation to results |
| `--confirm` | Apply bulk changes (preview-only by default) |

**Examples:**

```bash
rvn query "object:project .status:active"
rvn query "object:meeting has:due"
rvn query "trait:due value:past"
rvn query tasks                              # Run saved query
rvn query --list                             # List saved queries
rvn query "trait:due value:past" --ids       # For piping
rvn query "trait:due value:past" --apply "set status=overdue"
rvn query "trait:due value:past" --apply "set status=overdue" --confirm
```

See `reference/query-language.md` for the full query language.

---

### `rvn query add`

Add a saved query to `raven.yaml`.

```bash
rvn query add <name> <query_string> [--description "..."]
```

| Argument | Description |
|----------|-------------|
| `name` | Name for the new query |
| `query_string` | Query string |

| Flag | Description |
|------|-------------|
| `--description` | Human-readable description |

**Examples:**

```bash
rvn query add tasks "trait:due"
rvn query add overdue "trait:due value:past" --description "Overdue items"
rvn query add active-projects "object:project .status:active"
```

---

### `rvn query remove`

Remove a saved query from `raven.yaml`.

```bash
rvn query remove <name>
```

---

### `rvn search`

Full-text search across all vault content.

```bash
rvn search <query> [--limit N] [--type TYPE]
```

| Argument | Description |
|----------|-------------|
| `query` | Search query (words, phrases, or boolean expressions) |

| Flag | Short | Description |
|------|-------|-------------|
| `--limit` | `-n` | Maximum number of results (default: 20) |
| `--type` | `-t` | Filter by object type |

**Search syntax:**
- Simple words: `meeting notes` (finds pages containing both words)
- Phrases: `"team meeting"` (exact phrase match)
- Prefix: `meet*` (matches meeting, meetings, etc.)
- Boolean: `meeting AND notes`, `meeting OR notes`, `meeting NOT private`

**Examples:**

```bash
rvn search "meeting notes"
rvn search "project*" --type project
rvn search '"world tree"' --limit 5
rvn search "freya OR thor"
```

---

### `rvn backlinks`

Find objects that reference a target.

```bash
rvn backlinks <target>
```

| Argument | Description |
|----------|-------------|
| `target` | Target object ID (e.g., `people/freya`) |

**Examples:**

```bash
rvn backlinks people/freya
rvn backlinks projects/website
```

---

## Daily Notes & Dates

### `rvn daily`

Open or create a daily note.

```bash
rvn daily [date] [--edit]
```

| Argument | Description |
|----------|-------------|
| `date` | Date (today, yesterday, tomorrow, YYYY-MM-DD). Default: today |

| Flag | Short | Description |
|------|-------|-------------|
| `--edit` | `-e` | Open the note in the configured editor |

**Examples:**

```bash
rvn daily
rvn daily yesterday
rvn daily 2025-02-01
rvn daily --edit
```

---

### `rvn date`

Date hub — all activity for a date.

```bash
rvn date [date] [--edit]
```

| Argument | Description |
|----------|-------------|
| `date` | Date (today, yesterday, YYYY-MM-DD). Default: today |

| Flag | Description |
|------|-------------|
| `--edit` | Open the daily note in editor |

Returns: daily note content, items due on that date, meetings, and other related objects.

**Examples:**

```bash
rvn date today
rvn date 2025-02-01
```

---

## Navigation & Reading

### `rvn open`

Open a file in your editor.

```bash
rvn open <reference>
```

| Argument | Description |
|----------|-------------|
| `reference` | Reference to the file (short name, partial path, or full path) |

**Examples:**

```bash
rvn open cursor                    # Opens companies/cursor.md
rvn open companies/cursor          # Partial path
rvn open people/freya
```

**Notes:**
- Editor is determined by `editor` setting in config or `$EDITOR`

---

### `rvn read`

Read raw file content.

```bash
rvn read <path>
```

| Argument | Description |
|----------|-------------|
| `path` | File path relative to vault |

**Examples:**

```bash
rvn read daily/2025-02-01.md
rvn read people/freya.md
```

---

## Vault Management

### `rvn reindex`

Rebuild the SQLite index from vault files.

```bash
rvn reindex [--full] [--dry-run]
```

| Flag | Description |
|------|-------------|
| `--full` | Force full reindex of all files (default is incremental) |
| `--dry-run` | Show what would be reindexed without doing it |

**When to use:**
- After bulk file operations outside of Raven
- After schema changes that affect indexing
- Recovering from index corruption

**Notes:**
- Incremental reindex (default) only processes files changed since last index
- Deleted files are automatically detected and removed
- Use `--full` after schema changes to ensure all files are re-parsed

---

### `rvn check`

Validate vault against schema.

```bash
rvn check [path] [flags]
```

| Argument | Description |
|----------|-------------|
| `path` | Optional: file, directory, or reference to check (defaults to entire vault) |

| Flag | Short | Description |
|------|-------|-------------|
| `--type` | `-t` | Check only objects of this type |
| `--trait` | | Check only usages of this trait |
| `--issues` | | Only check these issue types (comma-separated) |
| `--exclude` | | Exclude these issue types (comma-separated) |
| `--errors-only` | | Only report errors, skip warnings |
| `--strict` | | Treat warnings as errors (exit code 1) |
| `--by-file` | | Group output by file path |
| `--create-missing` | | Interactively create missing pages (CLI only) |

**Examples:**

```bash
# Check entire vault
rvn check

# Check a specific file
rvn check people/freya.md
rvn check freya              # Using a reference

# Check a directory
rvn check projects/

# Check all objects of a type
rvn check --type project

# Check all usages of a trait
rvn check --trait due

# Only check specific issue types
rvn check --issues missing_reference,unknown_type

# Exclude certain issue types
rvn check --exclude unused_type,unused_trait

# Only show errors
rvn check --errors-only
```

**JSON output includes scope:**

```json
{
  "vault_path": "/path/to/vault",
  "scope": {
    "type": "file",
    "value": "people/freya.md"
  },
  "file_count": 1,
  "error_count": 0,
  "warning_count": 0,
  "issues": [],
  "summary": []
}
```

**Issue types:**

| Issue Type | Meaning |
|------------|---------|
| `unknown_type` | File uses a type not in schema |
| `missing_reference` | Link to non-existent page |
| `undefined_trait` | Trait not in schema |
| `unknown_frontmatter_key` | Field not defined for type |
| `missing_required_field` | Required field not set |
| `invalid_enum_value` | Value not in allowed enum list |
| `wrong_target_type` | Ref field points to wrong type |
| `invalid_date_format` | Date/datetime value is malformed |
| `duplicate_object_id` | Multiple objects share the same ID |
| `unused_type` | Type defined but never used (warning) |
| `unused_trait` | Trait defined but never used (warning) |
| `stale_index` | Index needs reindexing (warning) |

---

### `rvn stats`

Show vault statistics.

```bash
rvn stats
```

Returns counts of objects, traits, references, and files by type.

---

### `rvn untyped`

List pages without an explicit type.

```bash
rvn untyped
```

Lists all markdown files that don't have an explicit `type:` in their frontmatter (fallback to `page` type).

---

### `rvn vaults`

List configured vaults.

```bash
rvn vaults
```

---

## Schema Commands

### `rvn schema`

Introspect the schema.

```bash
rvn schema [subcommand]
```

| Subcommand | Description |
|------------|-------------|
| `types` | List all types |
| `traits` | List all traits |
| `type <name>` | Show details for a specific type |
| `trait <name>` | Show details for a specific trait |
| `commands` | List all commands (for MCP tool generation) |

**Examples:**

```bash
rvn schema types
rvn schema type person
rvn schema traits
rvn schema trait due
```

---

### `rvn schema add type`

Add a new type to the schema.

```bash
rvn schema add type <name> [--default-path PATH] [--name-field FIELD]
```

| Argument | Description |
|----------|-------------|
| `name` | Name of the new type |

| Flag | Description |
|------|-------------|
| `--default-path` | Default directory for files of this type |
| `--name-field` | Field to use as display name (auto-created if doesn't exist) |

**Examples:**

```bash
rvn schema add type person --name-field name --default-path people/
rvn schema add type project --name-field title --default-path projects/
```

---

### `rvn schema add trait`

Add a new trait to the schema.

```bash
rvn schema add trait <name> [--type TYPE] [--values VALUES]
```

| Argument | Description |
|----------|-------------|
| `name` | Name of the new trait |

| Flag | Description |
|------|-------------|
| `--type` | Trait type: string, date, enum, bool (default: string) |
| `--values` | Enum values (comma-separated) |

**Examples:**

```bash
rvn schema add trait priority --type enum --values high,medium,low
rvn schema add trait due --type date
rvn schema add trait highlight --type bool
```

---

### `rvn schema add field`

Add a field to an existing type.

```bash
rvn schema add field <type_name> <field_name> [--type TYPE] [--required] [--target TYPE]
```

| Argument | Description |
|----------|-------------|
| `type_name` | Type to add field to |
| `field_name` | Name of the new field |

| Flag | Description |
|------|-------------|
| `--type` | Field type: string, date, enum, ref, bool (default: string) |
| `--required` | Mark field as required |
| `--target` | Target type for ref fields |

**Examples:**

```bash
rvn schema add field person email --type string --required
rvn schema add field book author --type ref --target person
```

---

### `rvn schema update type`

Update an existing type.

```bash
rvn schema update type <name> [flags]
```

| Argument | Description |
|----------|-------------|
| `name` | Name of the type to update |

| Flag | Description |
|------|-------------|
| `--default-path` | Update default directory |
| `--name-field` | Set/update display name field (use `-` to remove) |
| `--add-trait` | Add a trait to this type |
| `--remove-trait` | Remove a trait from this type |

**Examples:**

```bash
rvn schema update type person --name-field name
rvn schema update type person --default-path people/
rvn schema update type meeting --add-trait due
```

---

### `rvn schema update trait`

Update an existing trait.

```bash
rvn schema update trait <name> [--type TYPE] [--values VALUES] [--default VALUE]
```

| Argument | Description |
|----------|-------------|
| `name` | Name of the trait to update |

| Flag | Description |
|------|-------------|
| `--type` | Update trait type |
| `--values` | Update enum values (comma-separated) |
| `--default` | Update default value |

**Examples:**

```bash
rvn schema update trait priority --values critical,high,medium,low
```

---

### `rvn schema update field`

Update a field on an existing type.

```bash
rvn schema update field <type_name> <field_name> [flags]
```

| Argument | Description |
|----------|-------------|
| `type_name` | Type containing the field |
| `field_name` | Field to update |

| Flag | Description |
|------|-------------|
| `--type` | Update field type |
| `--required` | Update required status (true/false) |
| `--default` | Update default value |
| `--target` | Update target type for ref fields |

**Notes:**
- Making a field required is blocked if any objects lack that field
- Add the field to all objects first, then make it required

**Examples:**

```bash
rvn schema update field person email --required=true
rvn schema update field project status --default=active
```

---

### `rvn schema remove type`

Remove a type from the schema.

```bash
rvn schema remove type <name> [--force]
```

| Flag | Description |
|------|-------------|
| `--force` | Skip confirmation prompt |

**Notes:**
- Existing files of this type will become `page` type (fallback)
- Warns about affected files before removal

---

### `rvn schema remove trait`

Remove a trait from the schema.

```bash
rvn schema remove trait <name> [--force]
```

| Flag | Description |
|------|-------------|
| `--force` | Skip confirmation prompt |

**Notes:**
- Existing `@trait` instances will remain in files but no longer be indexed
- Warns about affected trait instances before removal

---

### `rvn schema remove field`

Remove a field from a type.

```bash
rvn schema remove field <type_name> <field_name>
```

**Notes:**
- If the field is required, removal is blocked (make it optional first)
- Existing field values remain in files but are no longer validated

---

### `rvn schema rename type`

Rename a type and update all references.

```bash
rvn schema rename type <old_name> <new_name> [--confirm]
```

| Argument | Description |
|----------|-------------|
| `old_name` | Current type name |
| `new_name` | New type name |

| Flag | Description |
|------|-------------|
| `--confirm` | Apply the rename (default: preview only) |

**What it updates:**
1. The type definition in `schema.yaml`
2. All `type:` frontmatter fields in files
3. All `::type()` embedded declarations
4. All ref field `target:` values pointing to the old type

**Examples:**

```bash
# Preview changes
rvn schema rename type event meeting

# Apply changes
rvn schema rename type event meeting --confirm
```

**Notes:**
- Always run `rvn reindex --full` after renaming to update the index
- Built-in types (`page`, `section`, `date`) cannot be renamed

---

### `rvn schema validate`

Validate the schema for correctness.

```bash
rvn schema validate
```

Checks for internal consistency issues in `schema.yaml`.

---

## Workflow Commands

### `rvn workflow list`

List available workflows.

```bash
rvn workflow list
```

Lists all workflows defined in `raven.yaml` with their descriptions and required inputs.

---

### `rvn workflow show`

Show workflow details.

```bash
rvn workflow show <name>
```

Shows the full definition including inputs, context queries, and prompt template.

---

### `rvn workflow render`

Render a workflow with context.

```bash
rvn workflow render <name> [--input key=value...]
```

| Argument | Description |
|----------|-------------|
| `name` | Workflow name |

| Flag | Description |
|------|-------------|
| `--input` | Set input value (repeatable) |

**Examples:**

```bash
rvn workflow render meeting-prep --input meeting_id=meetings/alice-1on1
rvn workflow render research --input question="How does the auth system work?"
```

Returns the complete prompt with pre-gathered context.

---

## Server Commands

### `rvn serve`

Start the MCP server.

```bash
rvn serve --vault-path /path/to/vault
```

Exposes Raven's CLI commands as MCP tools. See `reference/mcp.md`.

---

### `rvn lsp`

Start the Language Server Protocol server.

```bash
rvn lsp
```

Provides editor integration for reference completion, hover info, etc.

---

## Shell Completion

### `rvn completion`

Generate shell completion scripts.

```bash
rvn completion bash
rvn completion zsh
rvn completion fish
rvn completion powershell
```

Follow the printed instructions to install completions for your shell.
