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

## Querying: The Raven Query Language

The query language is powerful and can answer complex questions in a single query. Master it to serve users efficiently.

### Query Types

**Object queries** return objects of a type:
  object:<type> [predicates...]

**Trait queries** return trait instances:
  trait:<name> [predicates...]

### All Predicates

**For object queries:**
- .field:value — Field equals value (.status:active, .priority:high)
- .field:"value with spaces" — Quoted value for spaces/special chars
- .field:* — Field exists (has any value)
- !.field:value — Field does NOT equal value
- !.field:* — Field does NOT exist
- has:trait — Has trait directly on object (has:due, has:priority)
- has:{trait:X ...} — Has trait matching sub-query (has:{trait:due value:past})
- contains:trait — Has trait anywhere in subtree (self or nested sections)
- contains:{trait:X ...} — Has matching trait anywhere in subtree
- refs:[[target]] — References specific target (refs:[[people/freya]])
- refs:{object:X ...} — References objects matching sub-query
- parent:type — Direct parent is type (parent:date for meetings in daily notes)
- parent:{object:X ...} — Direct parent matches sub-query
- parent:[[target]] — Direct parent is specific object
- ancestor:type — Any ancestor is type
- ancestor:{object:X ...} — Any ancestor matches sub-query
- ancestor:[[target]] — Specific object is an ancestor
- child:type — Has direct child of type
- child:{object:X ...} — Has direct child matching sub-query
- descendant:type — Has descendant of type at any depth
- descendant:{object:X ...} — Has descendant matching sub-query
- content:"term" — Full-text search on object content

**For trait queries:**
- value:X — Trait value equals X (value:past, value:high, value:todo)
- !value:X — Trait value does NOT equal X
- on:type — Trait's direct parent object is type (on:meeting, on:book)
- on:{object:X ...} — Direct parent matches sub-query
- on:[[target]] — Direct parent is specific object
- within:type — Trait is inside an object of type (any ancestor)
- within:{object:X ...} — Inside object matching sub-query
- within:[[target]] — Inside specific object
- refs:[[target]] — Trait's line references target
- refs:{object:X ...} — Trait's line references matching objects
- content:"term" — Trait's line contains term

**Boolean operators:**
- Space between predicates = AND
- | = OR (use parentheses for grouping)
- ! = NOT (prefix)
- (...) = grouping

**Special date values for trait:due:**
- value:past — Before today
- value:today — Today
- value:tomorrow — Tomorrow
- value:this-week — This week
- value:next-week — Next week

### Query Composition: Translating Requests to Queries

When a user asks a question, decompose it into query components:

1. **What am I looking for?** → trait:X or object:X
2. **What value/state?** → value:X or .field:X
3. **Where is it located?** → within:X, on:X, parent:X, ancestor:X
4. **What does it reference?** → refs:[[X]] or refs:{object:X ...}

**Example decomposition:**

User: "Find open todos from meetings about the growth project"

1. What? → trait:todo (looking for todo traits)
2. What value? → value:todo (open/incomplete)
3. Where? → within:meeting (inside meeting objects)
4. References? → refs:[[projects/growth]] (mentions the project)

Query: trait:todo value:todo within:meeting refs:[[projects/growth]]

### Compound Queries vs. Multiple Queries

**Try a compound query first.** The query language can often express complex requests in one query.

**Fall back to multiple queries when:**
- The request has genuinely independent parts
- A compound query returns no results and simpler queries might help debug
- The user's intent is ambiguous (run variations to cover interpretations)

**Handling ambiguity:** If the request could be interpreted multiple ways, run queries for each interpretation and consolidate results. Err on providing MORE information to the user, not less.

**Example of ambiguity:**

User: "Todos related to the website project"

This could mean:
- Todos that reference the project: trait:todo refs:[[projects/website]]
- Todos inside the project file: trait:todo within:[[projects/website]]
- Todos in meetings about the project: trait:todo within:{object:meeting refs:[[projects/website]]}

Run the most likely interpretation first. If results seem incomplete, try variations.

### Complex Query Examples

**Overdue items assigned to a person:**
  trait:due value:past refs:[[people/freya]]

**Highlights from books currently being read:**
  trait:highlight on:{object:book .status:reading}

**Todos in meetings that reference active projects:**
  trait:todo within:meeting refs:{object:project .status:active}

**Meetings in daily notes that mention a specific person:**
  object:meeting parent:date refs:[[people/thor]]

**Projects that have any incomplete todos (anywhere in document):**
  object:project contains:{trait:todo value:todo}

**Tasks due this week on active projects:**
  trait:due value:this-week within:{object:project .status:active}

**Items referencing either of two people:**
  trait:due (refs:[[people/freya]] | refs:[[people/thor]])

**Sections inside a specific project:**
  object:section ancestor:[[projects/website]]

**Meetings without any due items:**
  object:meeting !has:due

**Active projects that reference a specific company:**
  object:project .status:active refs:[[companies/acme]]

### Query Strategy

1. **Understand the schema first** if unsure what types/traits exist:
   raven_schema(subcommand="types")
   raven_schema(subcommand="traits")

2. **Start with the most specific compound query** that captures the user's intent

3. **If no results**, consider:
   - Is the query too restrictive? Remove a predicate
   - Wrong type/trait name? Check schema
   - Try an alternative interpretation

4. **If ambiguous**, run 2-3 query variations and consolidate:
   - Present all relevant results to the user
   - Note which interpretation each result came from if helpful

5. **Avoid reading files directly** when a query can answer the question. File reads are for:
   - Getting full content after identifying relevant objects
   - Understanding file structure for edits
   - NOT for searching or filtering (use queries instead)

6. **Use backlinks sparingly** — refs: predicate in queries is usually sufficient. Use raven_backlinks only when you need ALL incoming references to an object.

### Query Examples for Common Questions

**"What's due soon?"**
  trait:due value:this-week
  trait:due value:today
  trait:due (value:today | value:tomorrow | value:this-week)

**"Show me my tasks"**
  trait:todo value:todo

**"What did I capture about X?"**
  First try: object:section content:"X" or trait:highlight content:"X"
  Fallback: raven_search(query="X")

**"Meetings with person X"**
  object:meeting refs:[[people/X]]

**"Notes from project X"**
  object:section ancestor:[[projects/X]]
  Or for traits: trait:highlight within:[[projects/X]]

**"What references this project?"**
  raven_backlinks(target="projects/X")
  Or in a query: object:meeting refs:[[projects/X]]

**"Incomplete items in daily notes"**
  trait:todo value:todo within:date

**"High priority items that are overdue"**
  Run both and consolidate:
  trait:priority value:high
  trait:due value:past
  (If same items have both traits, they match the user's intent)

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
   raven_add(text="Meeting notes", to="cursor")  # Resolves to companies/cursor.md
   
3. If a required field is missing, ask the user for the value

### 3. Schema Discovery

When you need to understand the vault structure:

1. Use raven_schema to see available types and traits:
   raven_schema(subcommand="types")   # List all types
   raven_schema(subcommand="traits")  # List all traits
   raven_schema(subcommand="type person")  # Details about person type

2. Check raven_query with list=true to see saved queries

### 4. Editing Content

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

### 5. Bulk Operations

When users want to update many objects at once:

1. Use raven_query with apply to update results in bulk:
   
   Preview changes (dry-run by default):
   raven_query(query_string="trait:due value:past", apply="set status=overdue")
   
   Apply after user confirmation:
   raven_query(query_string="trait:due value:past", apply="set status=overdue", confirm=true)

2. Supported bulk operations:
   - set field=value  — Update frontmatter fields
   - delete          — Delete matching objects
   - add <text>      — Append text to matching files
   - move <dir/>     — Move to directory (must end with /)

3. Use ids=true to get IDs for piping:
   raven_query(query_string="object:project .status:archived", ids=true)

4. Commands with stdin read IDs from input:
   raven_set(stdin=true, fields={"status": "archived"}, confirm=true)
   raven_delete(stdin=true, confirm=true)
   raven_add(stdin=true, text="@reviewed", confirm=true)
   raven_move(stdin=true, destination="archive/", confirm=true)

IMPORTANT: Always preview first, then confirm with user before applying.

### 6. Reindexing

After bulk operations or schema changes:

1. Use raven_reindex to rebuild the index:
   raven_reindex()              # Incremental (default) - only changed/deleted files
   raven_reindex(full=True)     # Force complete rebuild

2. This is needed after:
   - Adding new types or traits to the schema
   - Bulk file operations outside of Raven
   - If queries return stale results

### 7. Deleting Content

**ALWAYS confirm with the user before deleting anything.**

1. FIRST check for backlinks:
   raven_backlinks(target="projects/old-project")

2. THEN ask user to confirm:
   "Are you sure you want to delete projects/old-project?"
   If backlinks exist: "This is referenced by 3 pages. Deleting creates broken links. Continue?"

3. Only AFTER user confirms:
   raven_delete(object_id="projects/old-project")

4. Files go to .trash/ (not permanent), but STILL always confirm first.

Never delete without explicit user approval.

### 8. Opening Files & Daily Notes

1. Use raven_open to open files by reference:
   raven_open(reference="cursor")           # Opens companies/cursor.md
   raven_open(reference="companies/cursor") # Partial path also works
   raven_open(reference="people/freya")     # Opens people/freya.md

2. Use raven_daily for daily notes:
   raven_daily()                    # Today
   raven_daily(date="yesterday")
   raven_daily(date="2026-01-15")

3. Use raven_date for date hub (everything for a date):
   raven_date()
   raven_date(date="2026-01-15")

### 9. Vault Statistics

1. raven_stats() - vault overview with counts
2. raven_untyped() - pages without explicit types

### 10. Managing Saved Queries

1. Add: raven_query_add(name="urgent", query_string="trait:due value:this-week|past")
2. Remove: raven_query_remove(name="old-query")
3. List: raven_query(list=true)

### 11. Schema Updates

1. Update type: raven_schema_update_type(name="person", default_path="contacts/")
2. Update trait: raven_schema_update_trait(name="priority", values="critical,high,medium,low")
3. Update field: raven_schema_update_field(type_name="person", field_name="email", required="true")
4. Remove: raven_schema_remove_type, raven_schema_remove_trait, raven_schema_remove_field
5. Validate: raven_schema_validate()

### 12. Workflows

Workflows are reusable prompt templates:

1. List: raven_workflow_list()
2. Show: raven_workflow_show(name="meeting-prep")
3. Render: raven_workflow_render(name="research", input={"question": "How does auth work?"})

**How workflows work:**
- Inputs are validated first
- Context queries run with {{inputs.X}} substituted BEFORE execution
- Prompt is rendered with both {{inputs.X}} and {{context.X}}

**Context queries support input substitution:**
  context:
    person:
      read: "{{inputs.person_id}}"     # Input substituted before read
    tasks:
      query: "object:task .owner:{{inputs.person_id}}"  # Also works

**Prompt patterns:**
- {{inputs.name}} → Raw input value
- {{context.X}} → Auto-formatted result  
- {{context.X.content}} → Document content (for read: results)
- {{context.X.id}} → Object ID
- {{context.X.fields.name}} → Specific field

When to use: User asks for complex analysis, or there's a workflow matching their request.

### 13. Setting Up Templates

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

1. **Always use Raven commands instead of shell commands**: Raven commands maintain index consistency and update references automatically.

   Use raven_move (not mv) - updates all references to the moved file
   Use raven_delete (not rm) - warns about backlinks, moves to .trash/
   Use raven_new (not writing files) - applies templates, validates schema
   Use raven_set (not manual edits) - validates fields, triggers reindex
   Use raven_edit (not sed/awk) - safe content replacement
   Use raven_read (not cat) - for reading vault files

2. **Master the query language**: A single well-crafted query is better than multiple simple queries and file reads. Invest time in understanding predicates and composition.

3. **Err on more information**: When in doubt about what the user wants, provide more results rather than fewer. Run multiple query interpretations if ambiguous.

4. **Always ask before bulk changes**: "I found 45 files with unknown type 'book'. Should I add this type to your schema?"

5. **Use the schema as source of truth**: If something isn't in the schema, it won't be indexed or queryable. Guide users to define their types and traits.

6. **Prefer structured queries over search**: Use raven_query before falling back to raven_search.

7. **Check before creating**: Use raven_backlinks or raven_search to see if something already exists before creating duplicates.

8. **Respect user's organization**: Look at existing default_path settings to understand where different types of content belong.

9. **Reindex after schema changes**: If you add types or traits, run raven_reindex(full=True) so all files are re-parsed with the new schema.

## Example Conversations

**User**: "Find open todos from my experiment meetings"
- Compose query: trait:todo value:todo within:meeting refs:[[projects/experiments]]
- If unclear which project, also try: trait:todo value:todo within:meeting content:"experiment"
- Consolidate and present results

**User**: "What do I have due this week?"
- Use raven_query(query_string="trait:due value:this-week")
- Summarize results for user

**User**: "Show me highlights from the books I'm reading"
- Use raven_query(query_string="trait:highlight on:{object:book .status:reading}")
- If no results, check: raven_schema(subcommand="type book") to verify status field exists

**User**: "Tasks related to the website project"
- Try multiple interpretations:
  - trait:todo refs:[[projects/website]] (todos that reference it)
  - trait:todo within:[[projects/website]] (todos inside it)
- Consolidate results from both

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

**User**: "What happened yesterday?"
- raven_date(date="yesterday")
- Summarize: daily note content, items due, meetings

**User**: "Delete the old bifrost project"
- raven_backlinks(target="projects/old-bifrost")  # ALWAYS check references first
- Ask: "This is referenced by 5 pages. Are you sure you want to delete it?"
- Wait for explicit confirmation, then: raven_delete(object_id="projects/old-bifrost")

**User**: "Meetings where we discussed the API"
- Try: object:meeting content:"API"
- Or: object:meeting refs:[[projects/api]] if there's an API project

**User**: "Overdue items assigned to Freya"
- Use: trait:due value:past refs:[[people/freya]]
`
}
