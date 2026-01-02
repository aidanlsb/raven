# Raven Agent Guide

This guide helps AI agents effectively use Raven to manage a user's knowledge base.

## Core Concepts

**Raven** is a plain-markdown knowledge system with:
- **Types**: Define what objects ARE (person, project, book) — declared in frontmatter
- **Traits**: Annotations on content (@due, @priority, @highlight) — inline or frontmatter
- **References**: Wiki-style links between notes ([[people/alice]])
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
   - "I see 14 undefined types. The most common are: book (45 files), recipe (12 files)..."
   - "Would you like me to add these to your schema?"
4. Execute fix commands based on user confirmation
```

### 2. Creating Content

When users want to create notes:

```
1. Use raven_new for typed objects:
   raven_new(type="person", title="Alice Chen", field={"email": "alice@example.com"})
   
2. Use raven_add for quick capture:
   raven_add(text="@due(tomorrow) Follow up with Alice")
   
3. If a required field is missing, ask the user for the value
```

### 3. Querying

When users ask about their data:

```
1. Use raven_trait for trait-based queries:
   raven_trait(trait_type="due", value="today")  # What's due today?
   raven_trait(trait_type="due", value="past")   # What's overdue?

2. Use raven_query for saved queries:
   raven_query(query_name="tasks")  # Run the "tasks" query

3. Use raven_search for full-text search:
   raven_search(query="meeting notes")

4. Use raven_backlinks to find what references something:
   raven_backlinks(target="people/alice")
```

### 4. Schema Discovery

When you need to understand the vault structure:

```
1. Use raven_schema to see available types and traits:
   raven_schema(subcommand="types")   # List all types
   raven_schema(subcommand="traits")  # List all traits
   raven_schema(subcommand="type person")  # Details about person type

2. Check raven_query with list=true to see saved queries
```

### 5. Editing Content

When users want to modify existing notes:

```
1. Use raven_set for frontmatter changes:
   raven_set(object_id="people/alice", fields={"email": "new@example.com"})

2. Use raven_edit for content changes (requires unique string match):
   raven_edit(path="projects/website.md", old_str="Status: active", new_str="Status: completed", confirm=true)

3. Use raven_read first to understand the file content
```

## Issue Types Reference

When `raven_check` returns issues, here's how to fix them:

| Issue Type | Meaning | Fix Command |
|------------|---------|-------------|
| `unknown_type` | File uses a type not in schema | `raven_schema_add_type(name="book")` |
| `missing_reference` | Link to non-existent page | `raven_new(type="person", title="Alice")` |
| `undefined_trait` | Trait not in schema | `raven_schema_add_trait(name="toread", type="boolean")` |
| `unknown_frontmatter_key` | Field not defined for type | `raven_schema_add_field(type_name="person", field_name="company")` |
| `missing_required_field` | Required field not set | `raven_set(object_id="...", fields={"name": "..."})` |
| `missing_required_trait` | Required trait not set | `raven_set(object_id="...", fields={"due": "2025-02-01"})` |
| `invalid_enum_value` | Value not in allowed list | `raven_set(object_id="...", fields={"status": "done"})` |

### 6. Reindexing

After bulk operations or schema changes:

```
1. Use raven_reindex to rebuild the index:
   raven_reindex()

2. This is needed after:
   - Adding new types or traits to the schema
   - Bulk file operations outside of Raven
   - If queries return stale results
```

## Best Practices

1. **Always ask before bulk changes**: "I found 45 files with unknown type 'book'. Should I add this type to your schema?"

2. **Use the schema as source of truth**: If something isn't in the schema, it won't be indexed or queryable. Guide users to define their types and traits.

3. **Prefer structured queries over search**: Use `raven_trait`, `raven_type`, `raven_query` before falling back to `raven_search`.

4. **Check before creating**: Use `raven_backlinks` or `raven_search` to see if something already exists before creating duplicates.

5. **Respect user's organization**: Look at existing `default_path` settings to understand where different types of content belong.

6. **Reindex after schema changes**: If you add types or traits, run `raven_reindex` so they become queryable.

## Example Conversations

**User**: "What do I have due this week?"
```
→ raven_trait(trait_type="due", value="this-week")
→ Summarize results for user
```

**User**: "Add a new person for my colleague Bob Smith"
```
→ raven_schema(subcommand="type person")  # Check required fields
→ raven_new(type="person", title="Bob Smith", field={"name": "Bob Smith"})
```

**User**: "My vault has a lot of broken links, can you help fix them?"
```
→ raven_check()
→ Review summary, explain to user
→ "I see 2798 missing references. The most-referenced missing pages are:
    - 'consumer subs' (referenced 15 times)
    - 'Daniel Sternberg' (referenced 12 times)
   Would you like me to create pages for the most common ones? What type should they be?"
→ Create pages based on user input
```

**User**: "Create a project for the website redesign"
```
→ raven_schema(subcommand="type project")  # Check fields/traits
→ raven_new(type="project", title="Website Redesign")
→ "Created projects/website-redesign.md. Would you like to set any fields like client or due date?"
```
