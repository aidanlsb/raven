# Querying: The Raven Query Language

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
- `.field==value` — Field equals value (`.status==active`, `.priority==high`)
- `.field=="value with spaces"` — Quoted value for spaces/special chars
- `.field<value` — Field less than value (`.priority<5`)
- `.field>value` — Field greater than value (`.count>10`)
- `.field<=value` — Field less than or equal to value
- `.field>=value` — Field greater than or equal to value
- `in(.field, [a,b])` — Field is one of a list of values (scalar membership)
- `.field==*` — Field exists (has any value)
- `!.field==value` — Field does NOT equal value
- `!.field==*` — Field does NOT exist
- `has:{trait:X ...}` — Has trait matching sub-query (`has:{trait:due .value==past}`)
- `contains:{trait:X ...}` — Has matching trait anywhere in subtree
- `refs:[[target]]` — References specific target (`refs:[[people/freya]]`)
- `refs:{object:X ...}` — References objects matching sub-query
- `refd:[[source]]` — Referenced by specific source (inverse of `refs:`)
- `refd:{object:X ...}` — Referenced by objects matching sub-query
- `parent:{object:X ...}` — Direct parent matches sub-query
- `parent:[[target]]` — Direct parent is specific object
- `ancestor:{object:X ...}` — Any ancestor matches sub-query
- `ancestor:[[target]]` — Specific object is an ancestor
- `child:{object:X ...}` — Has direct child matching sub-query
- `child:[[target]]` — Has direct child that is a specific object
- `descendant:{object:X ...}` — Has descendant matching sub-query
- `descendant:[[target]]` — Has descendant that is a specific object
- `content:"term"` — Full-text search on object content

**For trait queries:**
- `.value==X` — Trait value equals X (`.value==past`, `.value==high`, `.value==todo`)
- `.value<X` — Trait value less than X (`.value<2025-01-01`)
- `.value>X` — Trait value greater than X (`.value>5`)
- `.value<=X` — Trait value less than or equal to X
- `.value>=X` — Trait value greater than or equal to X
- `in(.value, [a,b])` — Trait value is one of a list of values (use this for “value in list”)
- `!.value==X` — Trait value does NOT equal X
- `on:{object:X ...}` — Direct parent matches sub-query
- `on:[[target]]` — Direct parent is specific object
- `within:{object:X ...}` — Inside object matching sub-query
- `within:[[target]]` — Inside specific object
- `refs:[[target]]` — Trait's line references target
- `refs:{object:X ...}` — Trait's line references matching objects
- `at:{trait:X ...}` — Co-located with trait matching sub-query
- `content:"term"` — Trait's line contains term

**Boolean operators:**
- Space between predicates = AND
- `|` = OR (use parentheses for grouping)
- `!` = NOT (prefix)
- `(...)` = grouping

**Note on `in()` vs `any()`:**
- Use `in(.value, [a,b])` for **scalar** membership (trait `.value` is scalar).
- Use `any(.tags, _ == "x")` for **array fields** on objects.

**Special date values for `trait:due`:**
- `.value==past` — Before today
- `.value==today` — Today
- `.value==tomorrow` — Tomorrow
- `.value==this-week` — This week
- `.value==next-week` — Next week

**Sorting and Limiting:**

Use pipeline stages for sorting and limiting results.

- `|> sort(.value, asc|desc)` — Sort by trait value
- `|> sort(.field, asc|desc)` — Sort by object field
- `|> limit(N)` — Return at most N results

**Examples with sort/limit:**
```
trait:todo |> sort(.value, asc)             # Sort todos by their value
trait:due |> sort(.value, desc)             # Sort due dates descending
object:project |> sort(.status, asc)        # Sort projects by status
trait:due |> limit(10)                      # Get at most 10 due items
object:project .status==active |> limit(5)  # Get 5 active projects
```

### Query Composition: Translating Requests to Queries

When a user asks a question, decompose it into query components:

1. **What am I looking for?** → `trait:X` or `object:X`
2. **What value/state?** → `.value==X` or `.field==X`
3. **Where is it located?** → `within:{object:X}`, `on:{object:X}`, `parent:{object:X}`, `ancestor:{object:X}`
4. **What does it reference?** → `refs:[[X]]` or `refs:{object:X ...}`

**Example decomposition:**

User: "Find open todos from meetings about the growth project"

1. What? → `trait:todo` (looking for todo traits)
2. What value? → `.value==todo` (open/incomplete)
3. Where? → `within:{object:meeting}` (inside meeting objects)
4. References? → `refs:[[projects/growth]]` (mentions the project)

Query: `trait:todo .value==todo within:{object:meeting} refs:[[projects/growth]]`

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

### Common Pattern: “value is one of these”

Users often mean “open statuses” / “one of these states”. Use `in()`:

```
trait:todo in(.value, [todo,blocked,doing])
trait:todo !in(.value, [done,cancelled])
object:project in(.status, [active,backlog])
```

### Complex Query Examples

```
# Overdue items assigned to a person
trait:due .value==past refs:[[people/freya]]

# Highlights from books currently being read
trait:highlight on:{object:book .status==reading}

# Todos in meetings that reference active projects
trait:todo within:{object:meeting} refs:{object:project .status==active}

# Meetings in daily notes that mention a specific person
object:meeting parent:{object:date} refs:[[people/thor]]

# Projects that have any incomplete todos (anywhere in document)
object:project contains:{trait:todo .value==todo}

# Tasks due this week on active projects
trait:due .value==this-week within:{object:project .status==active}

# Items referencing either of two people
trait:due (refs:[[people/freya]] | refs:[[people/thor]])

# Sections inside a specific project
object:section ancestor:[[projects/website]]

# Meetings without any due items
object:meeting !has:{trait:due}

# Active projects that reference a specific company
object:project .status==active refs:[[companies/acme]]
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
