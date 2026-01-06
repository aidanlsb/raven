package mcp

// getAgentGuide returns the embedded agent guide content.
// This guide helps AI agents understand how to effectively use Raven.
func getAgentGuide() string {
	return `# Raven Agent Guide

This guide helps AI agents effectively use Raven to manage a user's knowledge base.

## Core Concepts

**Raven** is a plain-markdown knowledge system with:
- **Types**: Schema definitions for what things are (e.g., person, project, book) — defined in schema.yaml
- **Objects**: Instances of types — each file declares its type in frontmatter (e.g., people/freya.md is an object of type person)
- **Traits**: Inline annotations on content (@due, @priority, @highlight)
- **References**: Wiki-style links between notes ([[people/freya]])
- **Schema**: User-defined in schema.yaml — types and traits must be defined here to be queryable

## Key Workflows

### 1. Vault Health Check

When users ask about issues or want to clean up their vault:

1. Run raven_check to get structured issues
2. Review the summary to prioritize:
   - unknown_type: Files using undefined types
   - missing_reference: Broken links  
   - undefined_trait: Traits not in schema
3. Work through fixes WITH the user:
   - "I see 14 undefined types. The most common are: saga (45 files), rune (12 files)..."
   - "Would you like me to add these to your schema?"
4. Execute fix commands based on user confirmation

### 2. Creating Content

When users want to create notes:

1. Use raven_new for typed objects:
   raven_new(type="person", title="Freya", field={"email": "freya@asgard.realm"})
   
2. Use raven_add for quick capture:
   raven_add(text="@due(tomorrow) Follow up with Odin")
   
3. If a required field is missing, ask the user for the value

### 3. Querying

When users ask about their data:

1. Use raven_query for all queries:
   raven_query(query_string="trait:due value:today")  # What's due today?
   raven_query(query_string="trait:due value:past")   # What's overdue?
   raven_query(query_string="object:project .status:active")  # Active projects
   raven_query(query_string="tasks")  # Run saved query "tasks"

2. Use raven_search for full-text search:
   raven_search(query="meeting notes")

3. Use raven_backlinks to find what references something:
   raven_backlinks(target="people/freya")

### 4. Schema Discovery

When you need to understand the vault structure:

1. Use raven_schema to see available types and traits:
   raven_schema(subcommand="types")   # List all types
   raven_schema(subcommand="traits")  # List all traits
   raven_schema(subcommand="type person")  # Details about person type

2. Check raven_query with list=true to see saved queries

### 5. Editing Content

When users want to modify existing notes:

1. Use raven_set for frontmatter changes:
   raven_set(object_id="people/freya", fields={"email": "freya@asgard.realm"})

2. Use raven_edit for content changes (requires unique string match):
   raven_edit(path="projects/website.md", old_str="Status: active", new_str="Status: completed", confirm=true)

3. Use raven_read first to understand the file content

## Issue Types Reference

When raven_check returns issues, here's how to fix them:

| Issue Type | Meaning | Fix Command |
|------------|---------|-------------|
| unknown_type | File uses a type not in schema | raven_schema_add_type(name="book") |
| missing_reference | Link to non-existent page | raven_new(type="person", title="Freya") |
| undefined_trait | Trait not in schema | raven_schema_add_trait(name="toread", type="boolean") |
| unknown_frontmatter_key | Field not defined for type | raven_schema_add_field(type_name="person", field_name="company") |
| missing_required_field | Required field not set | raven_set(object_id="...", fields={"name": "..."}) |
| missing_required_trait | Required trait not set | raven_set(object_id="...", fields={"due": "2025-02-01"}) |
| invalid_enum_value | Value not in allowed list | raven_set(object_id="...", fields={"status": "done"}) |

### 6. Reindexing

After bulk operations or schema changes:

1. Use raven_reindex to rebuild the index:
   raven_reindex()              # Incremental (default) - only changed/deleted files
   raven_reindex(full=True)     # Force complete rebuild

2. This is needed after:
   - Adding new types or traits to the schema
   - Bulk file operations outside of Raven
   - If queries return stale results

### 7. Setting Up Templates

Templates provide default content when creating notes. Help users by editing their schema.yaml.

**Template Variables:**
- {{title}} - Title from rvn new
- {{slug}} - Slugified title  
- {{type}} - Type name
- {{date}} - Today (YYYY-MM-DD)
- {{datetime}} - Current datetime
- {{year}}, {{month}}, {{day}} - Date parts
- {{weekday}} - Day name (Monday, etc.)
- {{field.X}} - Field value from --field

**Adding a type template (edit schema.yaml):**

types:
  meeting:
    default_path: meetings/
    template: templates/meeting.md   # File path
    fields:
      time: { type: datetime }

Or inline:

types:
  quick-note:
    template: |
      # {{title}}
      
      Created: {{date}}
      
      ## Notes

**Create the template file:**
raven_add(text="# {{title}}\n\n**Time:** {{field.time}}\n\n## Agenda\n\n## Notes\n\n## Action Items", to="templates/meeting.md")

**Daily note templates (edit raven.yaml):**

daily_directory: daily
daily_template: |
  # {{weekday}}, {{date}}
  
  ## Morning
  
  ## Afternoon

**Workflow:**
1. Ask user what structure they want
2. Create template file with raven_add
3. Edit schema.yaml to add template field using raven_edit
4. Test with raven_new

## Best Practices

1. **Always ask before bulk changes**: "I found 45 files with unknown type 'book'. Should I add this type to your schema?"

2. **Use the schema as source of truth**: If something isn't in the schema, it won't be indexed or queryable. Guide users to define their types and traits.

3. **Prefer structured queries over search**: Use raven_query before falling back to raven_search.

4. **Check before creating**: Use raven_backlinks or raven_search to see if something already exists before creating duplicates.

5. **Respect user's organization**: Look at existing default_path settings to understand where different types of content belong.

6. **Reindex after schema changes**: If you add types or traits, run raven_reindex(full=True) so all files are re-parsed with the new schema.

## Example Conversations

**User**: "What do I have due this week?"
- Use raven_query(query_string="trait:due value:this-week")
- Summarize results for user

**User**: "Add a new person for my colleague Thor Odinson"
- Use raven_schema(subcommand="type person") to check required fields
- Use raven_new(type="person", title="Thor Odinson", field={"name": "Thor Odinson"})

**User**: "My vault has a lot of broken links, can you help fix them?"
- Use raven_check() to get structured issues
- Review summary, explain to user
- "I see 2798 missing references. The most-referenced missing pages are:
    - 'bifrost-bridge' (referenced 15 times)
    - 'Baldur' (referenced 12 times)
   Would you like me to create pages for the most common ones? What type should they be?"
- Create pages based on user input

**User**: "Create a project for the website redesign"  
- Use raven_schema(subcommand="type project") to check fields/traits
- Use raven_new(type="project", title="Website Redesign")
- "Created projects/website-redesign.md. Would you like to set any fields like client or due date?"

**User**: "I want a template for my meeting notes"
- Ask: "What sections would you like in your meeting template? Common ones include Attendees, Agenda, Notes, and Action Items."
- Create template: raven_add(text="# {{title}}\n\n**Time:** {{field.time}}\n\n## Attendees\n\n## Agenda\n\n## Notes\n\n## Action Items", to="templates/meeting.md")
- Read current schema: raven_read(path="schema.yaml")
- Edit schema to add template field: raven_edit(path="schema.yaml", old_str="meeting:\n    default_path: meetings/", new_str="meeting:\n    default_path: meetings/\n    template: templates/meeting.md", confirm=true)
- "Done! Now when you run 'rvn new meeting \"Team Sync\"' it will include those sections automatically."
`
}
