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
   raven_query(query_string="trait:due value==past", apply="set status=overdue")
   
   # Apply changes after user confirmation
   raven_query(query_string="trait:due value==past", apply="set status=overdue", confirm=true)
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
   raven_query_add(name="urgent", query_string="trait:due value==this-week|past", 
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

Workflows are reusable prompt templates. **Proactively check for workflows** when a user asks for complex analysis — a workflow may already exist for their request.

1. List available workflows:
   ```
   raven_workflow_list()
   ```

2. Show workflow details:
   ```
   raven_workflow_show(name="meeting-prep")
   ```
   Returns inputs required, context queries, and prompt template

3. Render a workflow with inputs:
   ```
   raven_workflow_render(name="meeting-prep", input={"meeting_id": "meetings/team-sync"})
   raven_workflow_render(name="research", input={"question": "How does auth work?"})
   ```
   Returns rendered prompt with gathered context. Use the prompt to guide your response to the user.

**How workflows work:**

1. **Inputs** are validated (required fields checked, defaults applied)
2. **Context queries** execute with `{{inputs.X}}` substituted first:
   ```yaml
   context:
     person:
       read: "{{inputs.person_id}}"     # Input substituted BEFORE read
     tasks:
       query: "object:task .owner=={{inputs.person_id}}"  # Also substituted
   ```
3. **Prompt** is rendered with both `{{inputs.X}}` and `{{context.X}}` substituted
4. **Result** contains the rendered prompt + raw context data

**Prompt variable patterns:**

| Pattern | What It Returns |
|---------|-----------------|
| `{{inputs.name}}` | Raw input value |
| `{{context.X}}` | Auto-formatted result (readable for prompts) |
| `{{context.X.content}}` | Document content (for `read:` results) |
| `{{context.X.id}}` | Object ID |
| `{{context.X.fields.name}}` | Specific field value |

**When to use workflows:**
- User asks for a complex, multi-step analysis
- User wants consistent formatting for recurring tasks
- There's a workflow matching their request (check with `raven_workflow_list`)

### 15. Setting Up Templates

Templates provide default content when users create new notes. Help users set up templates by editing their schema and creating template files.

**Adding a template to a type (schema.yaml):**

Templates are defined using the `template` field on a type definition. You can help users by:

1. Reading the current schema: `raven_read(path="schema.yaml")`
2. Using `raven_edit` to add a template field to a type

**Example: Adding a meeting template**

```yaml
# In schema.yaml, add template field to the meeting type:
types:
  meeting:
    default_path: meetings/
    template: templates/meeting.md    # File-based template
    fields:
      time: { type: datetime }
      attendees: { type: string }
```

Then create the template file:
```
raven_add(text="# {{title}}\n\n**Time:** {{field.time}}\n\n## Attendees\n\n## Agenda\n\n## Notes\n\n## Action Items", to="templates/meeting.md")
```

**Template Variables:**

| Variable | Description | Example Output |
|----------|-------------|----------------|
| `{{title}}` | Title passed to `rvn new` | "Team Sync" |
| `{{slug}}` | Slugified title | "team-sync" |
| `{{type}}` | The type name | "meeting" |
| `{{date}}` | Today's date | "2026-01-02" |
| `{{datetime}}` | Current datetime | "2026-01-02T14:30:00Z" |
| `{{year}}` | Current year | "2026" |
| `{{month}}` | Current month (2-digit) | "01" |
| `{{day}}` | Current day (2-digit) | "02" |
| `{{weekday}}` | Day name | "Monday" |
| `{{field.X}}` | Value of field X from `--field` | Value provided at creation |

**Inline templates (for simple cases):**

For short templates, use inline YAML instead of a file:

```yaml
types:
  quick-note:
    template: |
      # {{title}}
      
      Created: {{date}}
      
      ## Notes
```

**Daily note templates (raven.yaml):**

Daily notes use a special config in `raven.yaml`:

```yaml
daily_directory: daily
daily_template: templates/daily.md
```

Or inline:

```yaml
daily_directory: daily
daily_template: |
  # {{weekday}}, {{date}}
  
  ## Morning
  
  ## Afternoon
  
  ## Evening
```

**Workflow for helping users set up templates:**

1. Ask what type of notes they want templates for
2. Check the schema to see if the type exists: `raven_schema(subcommand="type meeting")`
3. Ask what sections/structure they want in new notes
4. Create the template file: `raven_add(text="...", to="templates/[type].md")`
5. Edit schema.yaml to add the template field: `raven_edit(path="schema.yaml", ...)`
6. Test it: `raven_new(type="meeting", title="Test Meeting")`
