# Raven Agent Guide

This guide helps AI agents effectively use Raven to manage a user's knowledge base.

## Core Concepts

**Raven** is a plain-markdown knowledge system with:
- **Types**: Schema definitions for what things are (e.g., `person`, `project`, `book`) — defined in `schema.yaml`
- **Objects**: Instances of types — each file declares its type in frontmatter (e.g., `people/freya.md` is an object of type `person`)
- **Traits**: Inline annotations on content (`@due`, `@priority`, `@highlight`)
- **References**: Wiki-style links between notes (`[[people/freya]]`)
- **Schema**: User-defined in `schema.yaml` — types and traits must be defined here to be queryable

## Key Workflows

### 1. Vault Health Check

When users ask about issues or want to clean up their vault:

```
1. Run raven_check to get structured issues
2. Review the summary to prioritize:
   - unknown_type: Files using undefined types
   - missing_reference: Broken links
   - undefined_trait: Traits not in schema
3. Work through fixes WITH the user:
   - "I see 14 undefined types. The most common are: saga (45 files), rune (12 files)..."
   - "Would you like me to add these to your schema?"
4. Execute fix commands based on user confirmation
```

### 2. Creating Content

When users want to create notes:

```
1. Use raven_new for typed objects:
   raven_new(type="person", title="Freya", field={"email": "freya@asgard.realm"})
   
2. Use raven_add for quick capture:
   raven_add(text="@due(tomorrow) Follow up with Odin")
   
3. If a required field is missing, ask the user for the value
```

### 3. Querying

When users ask about their data:

```
1. Use raven_query with the Raven query language:
   
   Object queries:
   raven_query(query_string="object:project .status:active")  # Active projects
   raven_query(query_string="object:meeting has:due")         # Meetings with due items
   raven_query(query_string="object:meeting parent:date")     # Meetings in daily notes
   raven_query(query_string="object:meeting refs:[[people/freya]]")  # Meetings mentioning Freya
   
   Trait queries:
   raven_query(query_string="trait:due value:past")           # Overdue items
   raven_query(query_string="trait:highlight on:book")        # Highlights in books
   
   Reference queries (find things that mention X):
   raven_query(query_string="object:meeting refs:[[projects/website]]")  # Meetings about website
   raven_query(query_string="object:meeting refs:{object:project .status:active}")  # Meetings about active projects
   
   Saved queries (defined in raven.yaml):
   raven_query(query_string="tasks")  # Run the "tasks" saved query

2. Use raven_search for full-text search:
   raven_search(query="meeting notes")

3. Use raven_backlinks to find what references something:
   raven_backlinks(target="people/freya")
```

Query language predicates:
- `.field:value` — filter by field (`.status:active`, `.email:*`)
- `has:trait` — object has trait (`has:due`, `has:{trait:due value:past}`)
- `refs:[[target]]` — object references target (`refs:[[people/freya]]`)
- `refs:{object:type ...}` — references objects matching subquery
- `value:val` — trait value equals (`value:past`, `!value:done`)
- `on:type` — trait's parent object is type
- `within:type` — trait is within ancestor of type
- `parent:type` / `ancestor:type` — object hierarchy
- `!pred` — negate, `pred1 | pred2` — OR

### 4. Schema Discovery

When you need to understand the vault structure:

```
1. Use raven_schema to see available types and traits:
   raven_schema(subcommand="types")   # List all types
   raven_schema(subcommand="traits")  # List all traits
   raven_schema(subcommand="type person")  # Details about person type

2. Check saved queries:
   raven_query(list=true)  # See saved queries defined in raven.yaml
```

### 5. Editing Content

When users want to modify existing notes:

```
1. Use raven_set for frontmatter changes:
   raven_set(object_id="people/freya", fields={"email": "freya@asgard.realm"})

2. Use raven_edit for content changes (requires unique string match):
   raven_edit(path="projects/website.md", old_str="Status: active", new_str="Status: completed", confirm=true)

3. Use raven_read first to understand the file content
```

### 6. Moving and Renaming Files

When users want to reorganize their vault:

```
1. Use raven_move to move or rename files:
   raven_move(source="inbox/note.md", destination="projects/website/note.md")
   raven_move(source="people/loki", destination="people/loki-archived")

2. References are updated automatically (--update-refs defaults to true)

3. IMPORTANT: If the response has needs_confirm=true, ASK THE USER before proceeding.
   This happens when moving to a type's default directory with a mismatched type.
   Example: Moving a 'page' type file to 'people/' (which is for 'person' type)
   
   Ask: "This file has type 'page' but you're moving it to 'people/' which is 
         for 'person' files. Should I proceed anyway, or would you like to 
         change the file's type first?"

4. Security: Files can ONLY be moved within the vault. The command will reject
   any attempt to move files outside the vault or move external files in.
```

## Issue Types Reference

When `raven_check` returns issues, here's how to fix them:

| Issue Type | Meaning | Fix Command |
|------------|---------|-------------|
| `unknown_type` | File uses a type not in schema | `raven_schema_add_type(name="book")` |
| `missing_reference` | Link to non-existent page | `raven_new(type="person", title="Freya")` |
| `undefined_trait` | Trait not in schema | `raven_schema_add_trait(name="toread", type="boolean")` |
| `unknown_frontmatter_key` | Field not defined for type | `raven_schema_add_field(type_name="person", field_name="company")` |
| `missing_required_field` | Required field not set | `raven_set(object_id="...", fields={"name": "..."})` |
| `missing_required_trait` | Required trait not set | `raven_set(object_id="...", fields={"due": "2025-02-01"})` |
| `invalid_enum_value` | Value not in allowed list | `raven_set(object_id="...", fields={"status": "done"})` |

### 6. Reindexing

After bulk operations or schema changes:

```
1. Use raven_reindex to rebuild the index:
   raven_reindex()              # Incremental (default) - only changed/deleted files
   raven_reindex(full=True)     # Force complete rebuild

2. This is needed after:
   - Adding new types or traits to the schema
   - Bulk file operations outside of Raven
   - If queries return stale results
```

### 7. Setting Up Templates

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

```
1. Ask what type of notes they want templates for
2. Check the schema to see if the type exists: raven_schema(subcommand="type meeting")
3. Ask what sections/structure they want in new notes
4. Create the template file: raven_add(text="...", to="templates/[type].md")
5. Edit schema.yaml to add the template field: raven_edit(path="schema.yaml", ...)
6. Test it: raven_new(type="meeting", title="Test Meeting")
```

## Best Practices

1. **Always ask before bulk changes**: "I found 45 files with unknown type 'book'. Should I add this type to your schema?"

2. **Use the schema as source of truth**: If something isn't in the schema, it won't be indexed or queryable. Guide users to define their types and traits.

3. **Prefer structured queries over search**: Use `raven_query` with the query language (`object:type .field:value`, `trait:name value:val`) before falling back to `raven_search`.

4. **Check before creating**: Use `raven_backlinks` or `raven_search` to see if something already exists before creating duplicates.

5. **Respect user's organization**: Look at existing `default_path` settings to understand where different types of content belong.

6. **Reindex after schema changes**: If you add types or traits, run `raven_reindex` so they become queryable. Use `raven_reindex(full=True)` after schema changes to ensure all files are re-parsed with the new schema.

## Example Conversations

**User**: "What do I have due this week?"
```
→ raven_query(query_string="trait:due value:this-week")
→ Summarize results for user
```

**User**: "Add a new person for my colleague Thor Odinson"
```
→ raven_schema(subcommand="type person")  # Check required fields
→ raven_new(type="person", title="Thor Odinson", field={"name": "Thor Odinson"})
```

**User**: "My vault has a lot of broken links, can you help fix them?"
```
→ raven_check()
→ Review summary, explain to user
→ "I see 2798 missing references. The most-referenced missing pages are:
    - 'bifrost-bridge' (referenced 15 times)
    - 'Baldur' (referenced 12 times)
   Would you like me to create pages for the most common ones? What type should they be?"
→ Create pages based on user input
```

**User**: "Create a project for the website redesign"
```
→ raven_schema(subcommand="type project")  # Check fields/traits
→ raven_new(type="project", title="Website Redesign")
→ "Created projects/website-redesign.md. Would you like to set any fields like client or due date?"
```

**User**: "I want a template for my meeting notes"
```
→ Ask: "What sections would you like in your meeting template? Common ones include 
   Attendees, Agenda, Notes, and Action Items."
→ Create template file:
   raven_add(text="# {{title}}\n\n**Time:** {{field.time}}\n\n## Attendees\n\n## Agenda\n\n## Notes\n\n## Action Items", 
             to="templates/meeting.md")
→ Read current schema:
   raven_read(path="schema.yaml")
→ Edit schema to add template field:
   raven_edit(path="schema.yaml", 
              old_str="meeting:\n    default_path: meetings/", 
              new_str="meeting:\n    default_path: meetings/\n    template: templates/meeting.md", 
              confirm=true)
→ "Done! Now when you run 'rvn new meeting \"Team Sync\"' it will include those sections automatically."
```
