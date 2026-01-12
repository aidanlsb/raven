# Raven Agent Guide

This guide helps AI agents effectively use Raven to manage a user's knowledge base.

## Getting Started

When first interacting with a Raven vault, follow this discovery sequence:

1. **Understand the schema**: `raven_schema(subcommand="types")` and `raven_schema(subcommand="traits")`
2. **Get vault overview**: `raven_stats()` to see object counts and structure
3. **Check saved queries**: `raven_query(list=true)` to see pre-defined queries
4. **Discover workflows**: `raven_workflow_list()` to find available workflows

You can also fetch the `raven://schema/current` MCP resource for the complete schema.yaml.

## Core Concepts

**Raven** is a plain-markdown knowledge system with:
- **Types**: Schema definitions for what things are (e.g., `person`, `project`, `book`) — defined in `schema.yaml`
- **Objects**: Instances of types — each file declares its type in frontmatter (e.g., `people/freya.md` is an object of type `person`)
- **Traits**: Inline annotations on content (`@due`, `@priority`, `@highlight`)
- **References**: Wiki-style links between notes (`[[people/freya]]`)
- **Schema**: User-defined in `schema.yaml` — types and traits must be defined here to be queryable

### File Format Quick Reference

**Frontmatter** (YAML at top of file):
```markdown
---
type: project
status: active
owner: "[[people/freya]]"
---
```

**Embedded objects** (typed heading):
```markdown
## Weekly Standup
::meeting(time=09:00, attendees=[[[people/freya]]])
```

**Traits** (inline annotations):
```markdown
- @due(2026-01-15) Send proposal to [[clients/acme]]
- @priority(high) Review the API design
- @highlight This insight is important
```

**References** (wiki-style links):
```markdown
[[people/freya]]              Full path
[[freya]]                     Short reference (if unambiguous)
[[people/freya|Freya]]        With display text
```

### Reference Resolution

When using object IDs in tool calls:
- **Full path**: `people/freya` — always works
- **Short reference**: `freya` — works if unambiguous (only one object matches)
- **With .md extension**: `people/freya.md` — also works

If a short reference is ambiguous, Raven returns an `ambiguous_reference` error listing all matches. Ask the user which one they meant, or use the full path.

## Querying: The Raven Query Language

The query language is powerful and can answer complex questions in a single query. Master it to serve users efficiently.

### Query Types

**Object queries** return objects of a type:
```
object:<type> [predicates...]
```

**Trait queries** return trait instances:
```
trait:<name> [predicates...]
```

### All Predicates

**For object queries:**
- `.field:value` — Field equals value (`.status:active`, `.priority:high`)
- `.field:"value with spaces"` — Quoted value for spaces/special chars
- `.field:<value` — Field less than value (`.priority:<5`)
- `.field:>value` — Field greater than value (`.count:>10`)
- `.field:<=value` — Field less than or equal to value
- `.field:>=value` — Field greater than or equal to value
- `.field:*` — Field exists (has any value)
- `!.field:value` — Field does NOT equal value
- `!.field:*` — Field does NOT exist
- `has:trait` — Has trait directly on object (`has:due`, `has:priority`)
- `has:{trait:X ...}` — Has trait matching sub-query (`has:{trait:due value:past}`)
- `contains:trait` — Has trait anywhere in subtree (self or nested sections)
- `contains:{trait:X ...}` — Has matching trait anywhere in subtree
- `refs:[[target]]` — References specific target (`refs:[[people/freya]]`)
- `refs:{object:X ...}` — References objects matching sub-query
- `refd:[[source]]` — Referenced by specific source (inverse of `refs:`)
- `refd:{object:X ...}` — Referenced by objects matching sub-query
- `parent:type` — Direct parent is type (`parent:date` for meetings in daily notes)
- `parent:{object:X ...}` — Direct parent matches sub-query
- `parent:[[target]]` — Direct parent is specific object
- `ancestor:type` — Any ancestor is type
- `ancestor:{object:X ...}` — Any ancestor matches sub-query
- `ancestor:[[target]]` — Specific object is an ancestor
- `child:type` — Has direct child of type
- `child:{object:X ...}` — Has direct child matching sub-query
- `descendant:type` — Has descendant of type at any depth
- `descendant:{object:X ...}` — Has descendant matching sub-query
- `content:"term"` — Full-text search on object content

**For trait queries:**
- `value:X` — Trait value equals X (`value:past`, `value:high`, `value:todo`)
- `value:<X` — Trait value less than X (`value:<2025-01-01`)
- `value:>X` — Trait value greater than X (`value:>5`)
- `value:<=X` — Trait value less than or equal to X
- `value:>=X` — Trait value greater than or equal to X
- `!value:X` — Trait value does NOT equal X
- `on:type` — Trait's direct parent object is type (`on:meeting`, `on:book`)
- `on:{object:X ...}` — Direct parent matches sub-query
- `on:[[target]]` — Direct parent is specific object
- `within:type` — Trait is inside an object of type (any ancestor)
- `within:{object:X ...}` — Inside object matching sub-query
- `within:[[target]]` — Inside specific object
- `refs:[[target]]` — Trait's line references target
- `refs:{object:X ...}` — Trait's line references matching objects
- `at:trait` — Co-located with another trait (same line)
- `at:{trait:X ...}` — Co-located with trait matching sub-query
- `content:"term"` — Trait's line contains term

**Boolean operators:**
- Space between predicates = AND
- `|` = OR (use parentheses for grouping)
- `!` = NOT (prefix)
- `(...)` = grouping

**Special date values for `trait:due`:**
- `value:past` — Before today
- `value:today` — Today
- `value:tomorrow` — Tomorrow
- `value:this-week` — This week
- `value:next-week` — Next week

**Sorting and Grouping:**

Queries can include `sort:` and `group:` clauses to control result ordering and grouping.

- `sort:_.value` — Sort by trait's value
- `sort:_.fieldname` — Sort by object's field
- `sort:_.parent` — Sort by parent object ID
- `sort:_.parent.fieldname` — Sort by field on parent
- `sort:{trait:X}` — Sort by related trait's value
- `sort:min:{trait:X}` — Sort by minimum value (for multiple matches)
- `sort:max:{trait:X}` — Sort by maximum value
- `sort:_.value:desc` — Sort descending
- `group:_.parent` — Group by parent object
- `group:_.refs:type` — Group by referenced object of type

**Limiting results:**
- `limit:N` — Return at most N results

**Examples with sort/group/limit:**
```
trait:todo sort:_.value                     # Sort todos by their value
trait:due sort:_.value:desc                 # Sort due dates descending
object:project sort:_.status                # Sort projects by status
trait:todo group:_.parent                   # Group todos by parent object
trait:todo group:_.refs:project sort:{trait:due}  # Group by project, sort by due
trait:due limit:10                          # Get at most 10 due items
object:project .status:active limit:5       # Get 5 active projects
```

### Query Composition: Translating Requests to Queries

When a user asks a question, decompose it into query components:

1. **What am I looking for?** → `trait:X` or `object:X`
2. **What value/state?** → `value:X` or `.field:X`
3. **Where is it located?** → `within:X`, `on:X`, `parent:X`, `ancestor:X`
4. **What does it reference?** → `refs:[[X]]` or `refs:{object:X ...}`

**Example decomposition:**

User: "Find open todos from meetings about the growth project"

1. What? → `trait:todo` (looking for todo traits)
2. What value? → `value:todo` (open/incomplete)
3. Where? → `within:meeting` (inside meeting objects)
4. References? → `refs:[[projects/growth]]` (mentions the project)

Query: `trait:todo value:todo within:meeting refs:[[projects/growth]]`

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
- Todos that reference the project: `trait:todo refs:[[projects/website]]`
- Todos inside the project file: `trait:todo within:[[projects/website]]`
- Todos in meetings about the project: `trait:todo within:{object:meeting refs:[[projects/website]]}`

Run the most likely interpretation first. If results seem incomplete, try variations.

### Complex Query Examples

```
# Overdue items assigned to a person
trait:due value:past refs:[[people/freya]]

# Highlights from books currently being read
trait:highlight on:{object:book .status:reading}

# Todos in meetings that reference active projects
trait:todo within:meeting refs:{object:project .status:active}

# Meetings in daily notes that mention a specific person
object:meeting parent:date refs:[[people/thor]]

# Projects that have any incomplete todos (anywhere in document)
object:project contains:{trait:todo value:todo}

# Tasks due this week on active projects
trait:due value:this-week within:{object:project .status:active}

# Items referencing either of two people
trait:due (refs:[[people/freya]] | refs:[[people/thor]])

# Sections inside a specific project
object:section ancestor:[[projects/website]]

# Meetings without any due items
object:meeting !has:due

# Active projects that reference a specific company
object:project .status:active refs:[[companies/acme]]
```

### Query vs. Search: When to Use Which

| Use `raven_query` when... | Use `raven_search` when... |
|---------------------------|----------------------------|
| You know the type/trait you want | You're doing free-text discovery |
| You need to filter by fields | You don't know the structure |
| You need relationship predicates | User asks "find mentions of X" |
| You want structured results | You want relevance-ranked results |

**Note:** `raven_query` supports `content:"term"` for text search within typed queries. Use this when you want to combine text search with type/field filtering.

### Query Strategy

1. **Understand the schema first** if unsure what types/traits exist:
   ```
   raven_schema(subcommand="types")
   raven_schema(subcommand="traits")
   ```

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

6. **Use backlinks sparingly** — `refs:` predicate in queries is usually sufficient. Use `raven_backlinks` only when you need ALL incoming references to an object.

## Key Workflows

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
   raven_query(query_string="trait:due value:past", apply="set status=overdue")
   
   # Apply changes after user confirmation
   raven_query(query_string="trait:due value:past", apply="set status=overdue", confirm=true)
   ```

2. Supported bulk operations:
   - `set field=value` — Update frontmatter fields
   - `delete` — Delete matching objects
   - `add <text>` — Append text to matching files
   - `move <dir/>` — Move to directory (destination must end with `/`)

3. Alternative: Use `--ids` to get IDs for piping:
   ```
   raven_query(query_string="object:project .status:archived", ids=true)
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
   raven_query_add(name="urgent", query_string="trait:due value:this-week|past", 
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
       query: "object:task .owner:{{inputs.person_id}}"  # Also substituted
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

## Error Handling

When tools return errors, here's how to handle them:

| Error Type | Meaning | What to Do |
|------------|---------|------------|
| `validation_error` | Invalid input or missing required fields | Check `retry_with` in response for corrected call template. Ask user for missing values. |
| `not_found` | Object or file doesn't exist | Verify the path/reference. Offer to create it. |
| `ambiguous_reference` | Short reference matches multiple objects | Show user the matches, ask which one they meant. Use full path. |
| `data_integrity` | Operation blocked to protect data | Explain the safety concern to user, ask for confirmation. |
| `parse_error` | YAML/markdown syntax error | Read the file, identify the syntax issue, offer to fix it. |

**Validation error recovery:**

When `raven_new` or `raven_set` fails due to missing required fields, the response includes a `retry_with` template showing exactly what call to make with the missing fields filled in. Use this to ask the user for the missing values.

## Issue Types Reference

When `raven_check` returns issues, here's how to fix them:

**Errors (must fix):**

| Issue Type | Meaning | Fix Command |
|------------|---------|-------------|
| `unknown_type` | File uses a type not in schema | `raven_schema_add_type(name="book")` |
| `missing_reference` | Link to non-existent page | `raven_new(type="person", title="Freya")` |
| `unknown_frontmatter_key` | Field not defined for type | `raven_schema_add_field(type_name="person", field_name="company")` |
| `missing_required_field` | Required field not set | `raven_set(object_id="...", fields={"name": "..."})` |
| `missing_required_trait` | Required trait not set | `raven_set(object_id="...", fields={"due": "2025-02-01"})` |
| `invalid_enum_value` | Value not in allowed list | `raven_set(object_id="...", fields={"status": "done"})` |
| `wrong_target_type` | Ref field points to wrong type | Update the reference to point to correct type |
| `invalid_date_format` | Date/datetime value malformed | Fix to YYYY-MM-DD format |
| `duplicate_object_id` | Multiple objects share same ID | Rename one of the duplicates |
| `parse_error` | YAML frontmatter or syntax error | Fix the malformed syntax |
| `ambiguous_reference` | Reference matches multiple objects | Use full path: `[[people/freya]]` |
| `missing_target_type` | Ref field's target type doesn't exist | Add the target type to schema |
| `duplicate_alias` | Multiple objects use same alias | Rename one of the aliases |
| `alias_collision` | Alias conflicts with object ID/short name | Rename the alias |

**Warnings (optional to fix):**

| Issue Type | Meaning | Fix Suggestion |
|------------|---------|----------------|
| `undefined_trait` | Trait not in schema | `raven_schema_add_trait(name="toread", type="boolean")` |
| `unused_type` | Type defined but never used | Remove from schema or create an instance |
| `unused_trait` | Trait defined but never used | Remove from schema or use it |
| `stale_index` | Index needs reindexing | `raven_reindex()` |
| `short_ref_could_be_full_path` | Short ref could be clearer | Consider using full path |
| `id_collision` | Short name matches multiple objects | Use full paths in references |
| `self_referential_required` | Type has required ref to itself | Make field optional or add default |

**Using issue types for filtering:**

```
# Focus on actionable errors only
raven_check(issues="missing_reference,unknown_type,missing_required_field")

# Skip noisy schema warnings during cleanup
raven_check(exclude="unused_type,unused_trait,short_ref_could_be_full_path")

# Just errors, no warnings
raven_check(errors_only=true)
```

## Best Practices

1. **Always use Raven commands instead of shell commands**: Raven commands maintain index consistency and update references automatically.

   | Task | Use This | NOT This |
   |------|----------|----------|
   | Move/rename files | `raven_move` | `mv` |
   | Delete files | `raven_delete` | `rm` |
   | Create typed objects | `raven_new` | Writing files directly |
   | Update frontmatter | `raven_set` | Manual file edits |
   | Edit content | `raven_edit` | `sed`, `awk`, etc. |
   | Read vault files | `raven_read` | `cat`, `head`, etc. |

   **Why this matters:**
   - `raven_move` updates all references to the moved file automatically
   - `raven_delete` warns about backlinks and moves files to `.trash/`
   - `raven_new` applies templates and validates against the schema
   - `raven_set` validates field values and triggers reindexing

2. **Master the query language**: A single well-crafted query is better than multiple simple queries and file reads. Invest time in understanding predicates and composition.

3. **Err on more information**: When in doubt about what the user wants, provide more results rather than fewer. Run multiple query interpretations if ambiguous.

4. **Always ask before bulk changes**: "I found 45 files with unknown type 'book'. Should I add this type to your schema?"

5. **Preview before applying**: Operations like `raven_edit`, `raven_query --apply`, and bulk operations preview by default. Changes are NOT applied unless `confirm=true`.

6. **Use the schema as source of truth**: If something isn't in the schema, it won't be indexed or queryable. Guide users to define their types and traits.

7. **Prefer structured queries over search**: Use `raven_query` with the query language before falling back to `raven_search`.

8. **Check before creating**: Use `raven_backlinks` or `raven_search` to see if something already exists before creating duplicates.

9. **Respect user's organization**: Look at existing `default_path` settings to understand where different types of content belong.

10. **Reindex after schema changes**: If you add types or traits, run `raven_reindex(full=true)` so all files are re-parsed with the new schema.

11. **Check for workflows proactively**: When a user asks for complex analysis, check `raven_workflow_list()` first — there may be a workflow designed for their request.

## Example Conversations

**User**: "Find open todos from my experiment meetings"
```
→ Compose query: trait:todo value:todo within:meeting refs:[[projects/experiments]]
→ If unclear which project, also try: trait:todo value:todo within:meeting content:"experiment"
→ Consolidate and present results
```

**User**: "What do I have due this week?"
```
→ raven_query(query_string="trait:due value:this-week")
→ Summarize results for user
```

**User**: "Show me highlights from the books I'm reading"
```
→ raven_query(query_string="trait:highlight on:{object:book .status:reading}")
→ If no results, check: raven_schema(subcommand="type book") to verify status field exists
```

**User**: "Tasks related to the website project"
```
→ Try multiple interpretations:
  - trait:todo refs:[[projects/website]] (todos that reference it)
  - trait:todo within:[[projects/website]] (todos inside it)
→ Consolidate results from both
```

**User**: "Add a new person for my colleague Thor Odinson"
```
→ raven_schema(subcommand="type person")  # Check required fields and name_field
→ If name_field: name is set:
    raven_new(type="person", title="Thor Odinson")  # name auto-populated
→ If no name_field:
    raven_new(type="person", title="Thor Odinson", field={"name": "Thor Odinson"})
```

**User**: "My vault has a lot of broken links, can you help fix them?"
```
→ raven_check(issues="missing_reference")  # Focus on broken links
→ Review summary, explain to user
→ "I see 2798 missing references. The most-referenced missing pages are:
    - 'bifrost-bridge' (referenced 15 times)
    - 'Baldur' (referenced 12 times)
   Would you like me to create pages for the most common ones? What type should they be?"
→ Create pages based on user input
```

**User**: "I just created some new projects, make sure they're set up correctly"
```
→ raven_check(type="project")  # Validate all project objects
→ Report any issues: "All 5 projects are valid" or "2 projects have issues: ..."
→ Offer to fix any problems found
```

**User**: "Check if my due dates are formatted correctly"
```
→ raven_check(trait="due")  # Validate all @due trait usages
→ Report: "Found 3 invalid date formats: ..." or "All 42 due dates are valid"
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

**User**: "What happened yesterday?"
```
→ raven_date(date="yesterday")
→ Summarize: daily note content, items that were due, any meetings
```

**User**: "Open the cursor company page"
```
→ raven_open(reference="cursor")
→ "Opening companies/cursor.md"
```

**User**: "Delete the old bifrost project"
```
→ raven_backlinks(target="projects/old-bifrost")  # ALWAYS check for references first
→ "Before I delete projects/old-bifrost, I want to let you know it's referenced by 
   5 other pages. Deleting it will create broken links. 
   Should I proceed, or would you like to update those references first?"
→ Wait for explicit user confirmation
→ Only after user says yes: raven_delete(object_id="projects/old-bifrost")
→ "Done. The file has been moved to .trash/ and can be recovered if needed."
```

**User**: "Run the meeting prep workflow for my 1:1 with Freya"
```
→ raven_workflow_list()  # Check if meeting-prep exists
→ raven_workflow_render(name="meeting-prep", input={"person_id": "people/freya"})
→ Use the rendered prompt and context to provide a comprehensive meeting prep
```

**User**: "I want to save a query for finding all my reading list items"
```
→ raven_query_add(name="reading-list", 
                  query_string="trait:toread", 
                  description="Books and articles to read")
→ "Created saved query 'reading-list'. You can now run it with 'rvn query reading-list'"
```

**User**: "Show me pages that need to be organized"
```
→ raven_untyped()
→ "I found 15 pages without explicit types. Here are the most recent:
   - inbox/random-note.md
   - ideas/app-concept.md
   Would you like to assign types to any of these?"
```

**User**: "Meetings where we discussed the API"
```
→ Try: object:meeting content:"API"
→ Or: object:meeting refs:[[projects/api]] if there's an API project
```

**User**: "Overdue items assigned to Freya"
```
→ trait:due value:past refs:[[people/freya]]
```

**User**: "Show my todos sorted by due date"
```
→ trait:todo sort:{trait:due}
→ Or sort by value: trait:todo sort:_.value
```

**User**: "Which projects are mentioned in meetings?"
```
→ object:project refd:{object:meeting}
→ This uses the refd: predicate to find projects referenced by meetings
```

**User**: "Find high-priority items that are also due soon"
```
→ trait:due at:{trait:priority value:high}
→ Uses at: to find traits co-located on the same line
```

**User**: "Group my todos by project"
```
→ trait:todo group:_.refs:project
→ Groups todos by which project they reference
```

**User**: "Sort projects by their earliest due date"
```
→ object:project sort:min:{trait:due}
→ Uses min: aggregation to find the earliest due date on each project
```
