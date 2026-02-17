# Key Workflows

### 1. Vault Health Check

When users ask about issues or want to clean up their vault:

1. Run `raven_check` to get structured issues
2. Review the summary to prioritize:
   - `unknown_type`: Files using undefined types
   - `missing_reference`: Broken links
   - `undefined_trait`: Traits not in schema
3. Work through fixes WITH the user:
   - "I see 14 undefined types. The most common are: saga (45 files), rune (12 files)..."
   - "Would you like me to add these to your schema?"
4. Execute fix commands based on user confirmation

**Scoped checks for precision:**

```
# Check a specific file (verify your own work after edits)
raven_check(path="people/freya.md")
raven_check(path="freya")  # References work too

# Check a directory
raven_check(path="projects/")

# Check all objects of a specific type
raven_check(type="project")

# Check all usages of a specific trait
raven_check(trait="due")

# Filter to specific issue types
raven_check(issues="missing_reference,unknown_type")

# Exclude noisy warnings
raven_check(exclude="unused_type,unused_trait,short_ref_could_be_full_path")

# Only errors (skip warnings)
raven_check(errors_only=true)
```

**When to use scoped checks:**

| Scenario | Command |
|----------|---------|
| After creating/editing a file | `raven_check(path="path/to/file.md")` |
| Validating a type's instances | `raven_check(type="project")` |
| Checking trait value correctness | `raven_check(trait="due")` |
| Quick error-only scan | `raven_check(errors_only=true)` |
| Focus on broken links | `raven_check(issues="missing_reference")` |

**Verify your own work:**

After making changes, run a scoped check to verify:
```
# After creating a person
raven_new(type="person", title="Thor")
raven_check(path="thor")  # Verify the new file is valid

# After bulk edits to projects
raven_check(type="project")  # Verify all projects are still valid
```

### 2. Creating Content

When users want to create notes:

1. Use `raven_new` for typed objects:
   ```
   raven_new(type="person", title="Freya", field={"email": "freya@asgard.realm"})
   ```
   
2. Use `raven_add` for quick capture:
   ```
   raven_add(text="@due(tomorrow) Follow up with Odin")
   raven_add(text="Meeting notes", to="cursor")  # Resolves to companies/cursor.md
   ```

**Recommended agent flow for “create a new note and then add content”:**

Raven is intentionally **not** a free-form file writer. The intended pattern is:

1. Create the file with `raven_new`
2. Append content to that file with `raven_add(to=...)`

```
create = raven_new(type="project", title="Website Redesign")
# Use the returned file path (vault-relative)
raven_add(text="## Notes\n- Kickoff next week", to=create.data.file)
```

Notes:
- `raven_add` can auto-create **daily notes**; for other targets the file must already exist.
   
3. If a required field is missing, ask the user for the value

4. Check if the type has `name_field` configured (via `raven_schema type <name>`):
   - If `name_field` is set, the title argument auto-populates that field
   - Example: person type with `name_field: name` means `title="Freya"` sets `name="Freya"`

**name_field auto-population:**

If a type has `name_field` configured, you don't need to provide that field separately:
```
# Type has name_field: name
raven_new(type="person", title="Freya")  # name="Freya" is auto-set

# Type has name_field: title  
raven_new(type="book", title="The Prose Edda")  # title="The Prose Edda" is auto-set
```

Check `raven_schema types` — if you see a hint about types without `name_field`, suggest setting it up to simplify object creation.

### 3. Schema Discovery

When you need to understand the vault structure:

1. Use `raven_schema` to see available types and traits:
   ```
   raven_schema(subcommand="types")   # List all types (includes name_field hints)
   raven_schema(subcommand="traits")  # List all traits
   raven_schema(subcommand="type person")  # Details about person type
   ```

2. Check saved queries:
   ```
   raven_query(list=true)  # See saved queries defined in raven.yaml
   ```

3. Look for `name_field` hints in the types response:
   - Types with required string fields but no `name_field` are listed
   - Suggest setting up `name_field` for easier object creation

**Understanding name_field:**

When you call `raven_schema(subcommand="type person")`, check for:
- `name_field`: Which field is the display name (e.g., "name", "title")
- If set, the title argument to `raven_new` auto-populates this field

To set up `name_field` on an existing type:
```
raven_schema_update_type(name="person", name-field="name")
```

### 4. Editing Content

When users want to modify existing notes:

1. Use `raven_set` for frontmatter changes:
   ```
   raven_set(object_id="people/freya", fields={"email": "freya@asgard.realm"})
   ```

2. Use `raven_edit` for content changes (requires unique string match):
   ```
   # Preview first (default)
   raven_edit(path="projects/website.md", old_str="Status: active", new_str="Status: completed")
   
   # Apply after reviewing preview
   raven_edit(path="projects/website.md", old_str="Status: active", new_str="Status: completed", confirm=true)
   ```

3. Use `raven_read` first to understand the file content

**Tip for reliable edits:** Prefer `raven_read(path="...", raw=true)` before building `old_str` so the match is exact (no rendered links/backlink sections). For long files, use `start-line`/`end-line` (both are **1-indexed, inclusive**) and/or `lines=true` to get copy-paste-safe anchors without transcription.

**Important:** `raven_edit` returns a preview by default. Changes are NOT applied unless you set `confirm=true`.

### 5. Moving and Renaming Files

When users want to reorganize their vault:

1. Use `raven_move` to move or rename files:
   ```
   raven_move(source="inbox/note.md", destination="projects/website/note.md")
   raven_move(source="people/loki", destination="people/loki-archived")
   ```

2. References are updated automatically (`--update-refs` defaults to true)

3. **IMPORTANT:** If the response has `needs_confirm=true`, ASK THE USER before proceeding.
   This happens when moving to a type's default directory with a mismatched type.
   Example: Moving a 'page' type file to 'people/' (which is for 'person' type)
   
   Ask: "This file has type 'page' but you're moving it to 'people/' which is for 'person' files. Should I proceed anyway, or would you like to change the file's type first?"

4. Security: Files can ONLY be moved within the vault. The command will reject any attempt to move files outside the vault or move external files in.

### 6. Bulk Operations

When users want to update many objects at once:

1. Use `raven_query` with `--apply` to update query results in bulk:
   ```
   # Preview changes (default — changes NOT applied)
   raven_query(query_string="trait:due .value==past", apply="set status=overdue")
   
   # Apply changes after user confirmation
   raven_query(query_string="trait:due .value==past", apply="set status=overdue", confirm=true)
   ```

2. Supported bulk operations:
   - `set field=value` — Update frontmatter fields
   - `delete` — Delete matching objects
   - `add <text>` — Append text to matching files
   - `move <dir/>` — Move to directory (destination must end with `/`)

3. Alternative: Use `--ids` to get IDs for piping:
   ```
   raven_query(query_string="object:project .status==archived", ids=true)
   # Returns just the IDs, one per line
   ```

**Getting file paths from query results (for editing/navigation):**

- Object queries include `items[].file_path` and `items[].line`
- Trait queries include `items[].file_path` and `items[].line`

4. Commands with `--stdin` read IDs from standard input:
   ```
   raven_set(stdin=true, fields={"status": "archived"}, confirm=true)
   raven_delete(stdin=true, confirm=true)
   raven_add(stdin=true, text="@reviewed(2026-01-07)", confirm=true)
   raven_move(stdin=true, destination="archive/", confirm=true)
   ```

5. **ALWAYS preview first, then confirm:**
   - Run without `confirm=true` to see what will change
   - Present the preview to the user
   - Only run with `confirm=true` after user approval

**Bulk operation safety rules:**
- Always preview before applying
- Embedded objects (file#section): `set` supports them; `add/delete/move` skip them
- Errors are collected and reported, but don't stop other operations
- Use git to rollback if needed: `git checkout .`

### 7. Reindexing

After bulk operations or schema changes:

1. Use `raven_reindex` to rebuild the index:
   ```
   raven_reindex()              # Incremental (default) - only changed/deleted files
   raven_reindex(full=true)     # Force complete rebuild
   ```

2. This is needed after:
   - Adding new types or traits to the schema
   - Bulk file operations outside of Raven
   - If queries return stale results

### 8. Deleting Content

**⚠️ ALWAYS confirm with the user before deleting anything.**

When users want to remove files:

1. **FIRST** check for backlinks:
   ```
   raven_backlinks(target="projects/old-project")
   ```

2. **THEN** confirm with the user before deleting:
   - "I found this file is referenced by 3 other pages. Deleting it will create broken links. Are you sure you want to delete it?"
   - Even if no backlinks: "Are you sure you want to delete projects/old-project?"

3. Only after user confirms, use `raven_delete`:
   ```
   raven_delete(object_id="projects/old-project")
   ```

4. Files are moved to `.trash/` by default (not permanently deleted), but still ALWAYS get user confirmation first.

**Never delete without explicit user approval, even if they asked to delete something.**

### 9. Opening Files

When users want to open or navigate to files:

1. Use `raven_open` to open files by reference:
   ```
   raven_open(reference="cursor")           # Opens companies/cursor.md
   raven_open(reference="companies/cursor") # Partial path also works
   raven_open(reference="people/freya")     # Opens people/freya.md
   ```
   
   The reference can be a short name, partial path, or full path.

2. Use `raven_daily` to open/create daily notes:
   ```
   raven_daily()                    # Today's note
   raven_daily(date="yesterday")    # Yesterday
   raven_daily(date="2026-01-15")   # Specific date
   ```

3. Use `raven_date` for a date hub (everything related to a date):
   ```
   raven_date()                     # Today
   raven_date(date="2026-01-15")    # Specific date
   ```
   
   Returns: daily note, items due on that date, meetings, etc.

### 10. Vault Statistics & Untyped Pages

For understanding vault structure:

1. Use `raven_stats` for vault overview:
   ```
   raven_stats()
   ```
   Returns counts of objects, traits, references, files by type

2. Use `raven_untyped` to find pages without explicit types:
   ```
   raven_untyped()
   ```
   Returns files using fallback 'page' type. Helpful for cleanup: "I found 23 untyped pages. Would you like to assign types to them?"

### 11. Managing Saved Queries

Help users create reusable queries:

1. Add a saved query:
   ```
   raven_query_add(name="urgent", query_string="trait:due .value==this-week|past", 
                   description="Due soon or overdue")
   ```

2. Remove a saved query:
   ```
   raven_query_remove(name="old-query")
   ```

3. List saved queries:
   ```
   raven_query(list=true)
   ```

### 12. Adding Fields to Types

When adding fields to types, use the correct `--type` syntax:

**Field Type Reference:**

| Field Type | Syntax | Example |
|------------|--------|---------|
| Text | `type="string"` | name, email, notes |
| Array of text | `type="string[]"` | tags, keywords |
| Number | `type="number"` | priority, score |
| Date | `type="date"` | due, birthday |
| DateTime | `type="datetime"` | created_at, meeting_time |
| Boolean | `type="bool"` | active, archived |
| Single choice | `type="enum", values="a,b,c"` | status |
| Multiple choice | `type="enum[]", values="a,b,c"` | categories |
| Reference | `type="ref", target="<type>"` | owner, author |
| Array of refs | `type="ref[]", target="<type>"` | members, attendees |

**Common patterns:**

```
# Simple text field
raven_schema_add_field(type_name="person", field_name="email", type="string")

# Array of strings (tags, keywords)
raven_schema_add_field(type_name="project", field_name="tags", type="string[]")

# Reference to another type (single)
raven_schema_add_field(type_name="project", field_name="owner", type="ref", target="person")

# Array of references (team members, attendees)
raven_schema_add_field(type_name="team", field_name="members", type="ref[]", target="person")
raven_schema_add_field(type_name="meeting", field_name="attendees", type="ref[]", target="person")

# Enum with choices
raven_schema_add_field(type_name="project", field_name="status", type="enum", values="active,paused,done")

# Multiple enum selections
raven_schema_add_field(type_name="book", field_name="genres", type="enum[]", values="fiction,non-fiction,technical")
```

**Important:** The `type` parameter takes field types (string, ref, etc.), NOT schema type names. If you need a field that references objects of a certain type, use `type="ref", target="<type_name>"`.

### 13. Schema Updates, Renames & Removals

For modifying existing schema elements:

1. Update a type:
   ```
   raven_schema_update_type(name="person", default_path="contacts/")
   raven_schema_update_type(name="meeting", add_trait="due")
   raven_schema_update_type(name="person", name_field="name")  # Set display name field
   ```

2. Add a new type with name_field:
   ```
   raven_schema_add_type(name="book", default_path="books/", name_field="title")
   ```

3. Update a trait:
   ```
   raven_schema_update_trait(name="priority", values="critical,high,medium,low")
   ```

4. Update a field:
   ```
   raven_schema_update_field(type_name="person", field_name="email", required="true")
   ```

5. Rename a type (updates schema AND all files):
   ```
   # Preview first
   raven_schema_rename_type(old_name="event", new_name="meeting")
   
   # Apply after confirmation
   raven_schema_rename_type(old_name="event", new_name="meeting", confirm=true)
   
   # Always reindex after rename
   raven_reindex(full=true)
   ```

6. Remove schema elements (use with caution):
   ```
   raven_schema_remove_type(name="old-type", force=true)
   raven_schema_remove_trait(name="unused-trait", force=true)
   raven_schema_remove_field(type_name="person", field_name="nickname")
   ```

7. Validate schema:
   ```
   raven_schema_validate()
   ```

### 14. Workflows

Workflows are reusable multi-step pipelines. **Proactively check for workflows** when a user asks for complex analysis — a workflow may already exist for their request.

1. List available workflows:
   ```
   raven_workflow_list()
   ```

2. For first-time setup, scaffold a valid starter workflow:
   ```
   raven_workflow_scaffold(name="daily-brief")
   raven_workflow_validate(name="daily-brief")
   ```

3. Create custom workflows without editing `raven.yaml` directly:
   ```
   # First scaffold a valid file under directories.workflow (default workflows/)
   raven_workflow_scaffold(name="daily-brief")

   # Then register an existing file path
   raven_workflow_add(name="daily-brief", file="workflows/daily-brief.yaml")
   raven_workflow_validate(name="daily-brief")
   ```

   Notes:
   - `raven_workflow_add` is file-only (no inline definition JSON)
   - Files must be under `directories.workflow` in `raven.yaml`

4. Show workflow details:
   ```
   raven_workflow_show(name="meeting-prep")
   ```
   Returns inputs and steps.

5. Run a workflow with inputs:
   ```
   raven_workflow_run(name="meeting-prep", input={"meeting_id": "meetings/team-sync"})
   raven_workflow_run(name="research", input={"question": "How does auth work?"})
   ```
   Returns the rendered agent prompt plus `step_summaries`. Use:
   `raven_workflow_runs_step(run_id="...", step_id="...")` to fetch full step output on demand.

**How workflows work:**

1. **Inputs** are validated (required fields checked, defaults applied)
2. **Steps** execute in order, with `{{inputs.X}}` and `{{steps.<id>...}}` interpolated as needed
3. When an **agent** step is reached, Raven returns the prompt plus the declared `outputs` schema
4. Raven also returns `step_summaries` so agents can fetch heavy context incrementally by step
5. The agent responds with a JSON envelope: `{ "outputs": { ... } }`
6. If changes are needed, use explicit `tool` steps or normal Raven tools (`raven_add`, `raven_set`, `raven_edit`, `raven_move`, `raven_query --apply`, etc.)

**Variable patterns:**

| Pattern | What It Returns |
|---------|-----------------|
| `{{inputs.name}}` | Raw input value |
| `{{steps.stepId}}` | Entire step output |
| `{{steps.queryStep.data.results}}` | Result rows from a `tool: raven_query` step |
| `{{steps.readStep.data.content}}` | Raw file content from a `tool: raven_read` step |
| `{{steps.toolStep.ok}}` | Tool success boolean from any `tool` step |

**When to use workflows:**
- User asks for a complex, multi-step analysis
- User wants consistent formatting for recurring tasks
- There's a workflow matching their request (check with `raven_workflow_list`)

### 15. Setting Up Templates

Templates provide default content when users create new notes. Use the template CLI commands to manage them.

**Managing templates with CLI commands:**

```
# List all configured templates
raven_template_list()

# Scaffold + register a type template
raven_template_scaffold(target="type", type_name="meeting")

# Bind an existing file to a type template
raven_template_set(target="type", type_name="meeting", file="templates/meeting.md")

# Update template file content
raven_template_write(
  target="type",
  type_name="meeting",
  content="# {{title}}\n\n**Time:** {{field.time}}\n\n## Attendees\n\n## Notes\n\n## Action Items"
)

# Preview with variables
raven_template_render(target="type", type_name="meeting", title="Weekly Standup")

# Daily template lifecycle
raven_template_get(target="daily")
raven_template_set(target="daily", file="templates/daily.md")
raven_template_render(target="daily", date="tomorrow")

# Remove binding (optionally delete file)
raven_template_remove(target="type", type_name="meeting")
raven_template_remove(target="daily", delete_file=true)
```

**Template Variables:**

| Variable | Description | Example Output |
|----------|-------------|----------------|
| `{{title}}` | Title passed to `rvn new` | "Team Sync" |
| `{{slug}}` | Slugified title | "team-sync" |
| `{{type}}` | The type name | "meeting" |
| `{{date}}` | Today's date | "2026-01-02" |
| `{{datetime}}` | Current datetime | "2026-01-02T14:30" |
| `{{year}}` | Current year | "2026" |
| `{{month}}` | Current month (2-digit) | "01" |
| `{{day}}` | Current day (2-digit) | "02" |
| `{{weekday}}` | Day name | "Monday" |
| `{{field.X}}` | Value of field X from `--field` | Value provided at creation |

**Template policy:**

- Templates are **file-backed only** (no inline template bodies).
- Template files must be under `directories.template` (default: `templates/`).

**Important MCP notes for agents:**

- Use `raven_template_scaffold` for first-time setup (creates file + binds it).
- Use `raven_template_write` to update template content in-place.
- `raven_template_remove(..., delete_file=true)` can remove both binding and file with safety checks.

**Daily note templates (`raven.yaml`):**

Daily notes use `daily_template` as a file reference:

```yaml
daily_directory: daily
directories:
  template: templates/
daily_template: templates/daily.md
```

**Workflow for helping users set up templates:**

1. Ask what type of notes they want templates for
2. Check the schema to see if the type exists: `raven_schema(subcommand="type meeting")`
3. Ask what sections/structure they want in new notes
4. Scaffold or set the template: `raven_template_scaffold(target="type", type_name="meeting")`
5. Write content and preview: `raven_template_write(...)`, `raven_template_render(target="type", type_name="meeting", title="Test")`
6. Test it: `raven_new(type="meeting", title="Test Meeting")`

### 16. Resolving References

Use `raven_resolve` to check if a reference resolves before using it. This is a pure read operation with no side effects.

```
# Check if a short name resolves
raven_resolve(reference="freya")

# Resolve a dynamic date
raven_resolve(reference="today")

# Validate a reference before linking
raven_resolve(reference="The Prose Edda")
```

**Response fields:**
- `resolved: true/false` — whether resolution succeeded
- `object_id` — canonical object ID (when resolved)
- `file_path` — file path relative to vault (when resolved)
- `type` — object type (when resolved)
- `match_source` — how it was matched (`literal_path`, `short_name`, `alias`, `name_field`, `date`, etc.)
- `ambiguous: true` with `matches` array — when multiple objects match

### 17. Importing Structured Data

When users want to migrate or sync JSON data from other tools:

1. Use `raven_import` for batch create/update from JSON:
   ```
   raven_import(type="person", file="contacts.json", dry_run=true)
   ```

2. Always preview first with `dry_run=true`, then ask the user before applying with `confirm=true`:
   ```
   raven_import(type="person", file="contacts.json", confirm=true)
   ```

3. For mixed source records, use a mapping file with `type_field` and per-type maps:
   ```
   raven_import(mapping="migration.yaml", file="dump.json", dry_run=true)
   raven_import(mapping="migration.yaml", file="dump.json", confirm=true)
   ```

4. Use `content_field` when a JSON field should become markdown body content instead of frontmatter.

5. If the user only wants creates or only wants updates, use mode flags:
   - `create_only=true` to skip existing objects
   - `update_only=true` to skip missing objects
