# Query Logic

## Core Principles

1. **Two query types**: Object queries and trait queries
2. **Single return type**: Every query (including sub-queries) returns exactly one type of thing
3. **Boolean composition**: Predicates can be combined with AND, OR, NOT
4. **Recursive sub-queries**: Some predicates contain sub-queries that follow all the same rules

## Syntax Conventions

| Element | Syntax | Example |
|---------|--------|---------|
| Field access | `.` prefix | `.status:active` |
| Subquery | `{...}` curly braces | `has:{trait:due}` |
| Grouping | `(...)` parentheses | `(.status:active \| .status:done)` |
| References | `[[...]]` double brackets | `[[people/freya]]` |

## Query Types

### Object Query

Returns objects of a single type.

```
object:<type> [<predicates>...]
```

**Examples:**
```
object:meeting
object:project .status:active
object:meeting has:{trait:due value:past} ancestor:{object:date}
```

### Trait Query

Returns traits of a single name.

```
trait:<name> [<predicates>...]
```

**Examples:**
```
trait:due
trait:due value:past
trait:highlight on:{object:book .status:reading}
```

---

## Object Query Predicates

### Field-Based

Filter by object frontmatter fields. Fields use dot prefix to distinguish from keywords.

| Predicate | Meaning |
|-----------|---------|
| `.<field>:<value>` | Field equals/contains value |
| `.<field>:"value with spaces"` | Field equals quoted string (supports spaces) |
| `.<field>:*` | Field exists (has any value) |
| `!.<field>:<value>` | Field does NOT equal/contain value |
| `!.<field>:*` | Field does NOT exist (missing) |

For array fields, `.<field>:<value>` matches if the array contains the value.

Use double quotes for values containing spaces or special characters.

**Examples:**
```
object:project .status:active
object:project .title:"My Project"           # quoted string with spaces
object:book .author:"J.R.R. Tolkien"         # quoted string with punctuation
object:project .status:"in progress"         # status with space
object:person .email:*
object:person !.email:*
object:project !.status:done
object:project .tags:urgent          # array contains "urgent"
object:project .tags:urgent .tags:frontend   # has both
```

### Trait-Based (`has:`)

Filter by whether object contains matching traits. The predicate contains a trait sub-query.

| Predicate | Meaning |
|-----------|---------|
| `has:{trait:<name> ...}` | Has trait matching sub-query |
| `!has:{trait:<name> ...}` | Does NOT have trait matching sub-query |

**Shorthand:** `has:<trait>` expands to `has:{trait:<trait>}`

**Examples:**
```
object:meeting has:due
object:meeting has:{trait:due}
object:meeting has:{trait:due value:past}
object:meeting !has:{trait:due value:past}
object:meeting has:{trait:due !value:past}
object:project (has:due | has:remind)
```

**Semantic distinctions:**
- `has:{trait:due value:past}` — Has a due trait with value=past
- `has:{trait:due !value:past}` — Has a due trait with value≠past
- `!has:{trait:due value:past}` — Does NOT have any due trait with value=past
- `!has:due` — Does NOT have any due trait

### Parent (Direct)

Filter by whether object's direct parent matches an object sub-query or a specific object.

| Predicate | Meaning |
|-----------|---------|
| `parent:{object:<type> ...}` | Direct parent matches sub-query |
| `parent:[[target]]` | Direct parent is the specified object |
| `!parent:{object:<type> ...}` | Direct parent does NOT match sub-query |

**Shorthand:** `parent:<type>` expands to `parent:{object:<type>}`

**Direct target:** `parent:[[target]]` checks if the parent is a specific known object. The target is resolved using standard reference resolution (short names work if unambiguous).

**Examples:**
```
object:meeting parent:date
object:meeting parent:{object:date}
object:section parent:{object:project .status:active}
object:section parent:[[projects/website]]     # sections whose parent is this specific project
object:section parent:[[website]]              # short reference (if unambiguous)
```

### Ancestor (Any Depth)

Filter by whether any ancestor matches an object sub-query or is a specific object.

| Predicate | Meaning |
|-----------|---------|
| `ancestor:{object:<type> ...}` | Some ancestor matches sub-query |
| `ancestor:[[target]]` | The specified object is an ancestor |
| `!ancestor:{object:<type> ...}` | No ancestor matches sub-query |

**Shorthand:** `ancestor:<type>` expands to `ancestor:{object:<type>}`

**Direct target:** `ancestor:[[target]]` checks if a specific object appears anywhere in the ancestor chain.

**Examples:**
```
object:meeting ancestor:date
object:meeting ancestor:{object:date}
object:topic ancestor:{object:meeting ancestor:date}
object:section ancestor:[[projects/website]]   # sections anywhere inside this project
```

### Child (Direct)

Filter by whether object has at least one direct child matching an object sub-query or is a specific object.

| Predicate | Meaning |
|-----------|---------|
| `child:{object:<type> ...}` | Has child matching sub-query |
| `child:[[target]]` | The specified object is a direct child |
| `!child:{object:<type> ...}` | No child matches sub-query |

**Shorthand:** `child:<type>` expands to `child:{object:<type>}`

**Direct target:** `child:[[target]]` checks if a specific object is a direct child of the queried object.

**Examples:**
```
object:meeting child:topic
object:meeting child:{object:topic}
object:date child:{object:meeting has:due}
object:date child:[[daily/2025-02-01#standup]]  # dates that have this specific meeting as a child
```

### Descendant (Any Depth)

Filter by whether object has any descendant matching an object sub-query at any depth, or a specific object.

| Predicate | Meaning |
|-----------|---------|
| `descendant:{object:<type> ...}` | Has descendant matching sub-query |
| `descendant:[[target]]` | The specified object is a descendant |
| `!descendant:{object:<type> ...}` | No descendant matches sub-query |

**Shorthand:** `descendant:<type>` expands to `descendant:{object:<type>}`

**Direct target:** `descendant:[[target]]` checks if a specific object appears anywhere in the descendant tree.

**Examples:**
```
object:project descendant:section
object:project descendant:{object:section}
object:date descendant:{object:meeting has:due}
object:project descendant:[[projects/website#tasks]]  # projects that have this section as a descendant
```

### Contains (`contains:`)

Filter by whether object has matching traits anywhere in its subtree (self or any descendant). This is the inverse of `within:` — where `within:` finds traits inside objects, `contains:` finds objects that contain traits.

| Predicate | Meaning |
|-----------|---------|
| `contains:{trait:<name> ...}` | Has trait matching sub-query on self or any descendant |
| `!contains:{trait:<name> ...}` | No matching trait on self or any descendant |

**Shorthand:** `contains:<trait>` expands to `contains:{trait:<trait>}`

**Examples:**
```
object:project contains:todo
object:project contains:{trait:todo}
object:project contains:{trait:todo value:todo}
object:project contains:{trait:priority value:high}
object:date contains:{trait:due value:past}
```

**Semantic distinctions:**
- `has:{trait:todo}` — Has a todo trait **directly on** the object
- `contains:{trait:todo}` — Has a todo trait **anywhere** (self or nested sections/children)
- `contains:{trait:todo value:todo}` — Has an incomplete todo anywhere in subtree
- `!contains:{trait:todo value:done}` — Has no completed todos in subtree

### References (`refs:`)

Filter by what an object references (outgoing links). Use `refs:[[target]]` for a specific target, or `refs:{object:...}` to match targets by a sub-query.

| Predicate | Meaning |
|-----------|---------|
| `refs:[[target]]` | References specific target |
| `refs:{object:<type> ...}` | References objects matching sub-query |
| `!refs:[[target]]` | Does NOT reference target |
| `!refs:{object:<type> ...}` | Does NOT reference any matching objects |

**Examples:**
```
object:meeting refs:[[projects/website]]
object:meeting refs:[[people/freya]]
object:meeting refs:{object:project .status:active}
object:meeting !refs:[[projects/website]]
```

**Note:** For finding all objects that reference a given target (backlinks/incoming links), use `rvn backlinks <target>`. The `refs:` predicate filters by outgoing references within a typed query.

### Content Search (`content:`)

Filter by full-text search on object content. Uses FTS5 for efficient text matching.

| Predicate | Meaning |
|-----------|---------|
| `content:"term"` | Content contains search term(s) |
| `content:"exact phrase"` | Content contains exact phrase |
| `!content:"term"` | Content does NOT contain term |

**Search syntax** (FTS5):
- Simple words: `content:"meeting notes"` (finds pages with both words)
- Exact phrase: `content:"team meeting"` (exact phrase match)
- Prefix: `content:"meet*"` (matches meeting, meetings, etc.)
- Boolean: `content:"meeting AND notes"`, `content:"meeting OR notes"`

**Examples:**
```
object:person content:"colleague"
object:project content:"api design"
object:meeting content:"quarterly review"
object:person !content:"contractor"

# Combined with other predicates
object:person .status:active content:"engineer"
object:project has:due content:"deadline"
```

---

## Trait Query Predicates

### Value-Based

Filter by trait value.

| Predicate | Meaning |
|-----------|---------|
| `value:<val>` | Value equals val |
| `value:"val with spaces"` | Value equals quoted string (supports spaces) |
| `!value:<val>` | Value NOT equals val |

Use double quotes for values containing spaces or special characters.

**Examples:**
```
trait:due value:past
trait:due !value:past
trait:status value:todo
trait:status value:"in progress"       # quoted string with spaces
trait:priority value:high
trait:priority value:"very high"       # quoted string
```

### Content Search (`content:`)

Filter by text content on the same line as the trait. Uses substring matching.

| Predicate | Meaning |
|-----------|---------|
| `content:"term"` | Line content contains term |
| `!content:"term"` | Line content does NOT contain term |

**Examples:**
```
trait:todo content:"refactor"
trait:highlight content:"important"
trait:due content:"deadline"
!trait:todo content:"optional"
```

**Combined with other predicates:**
```
trait:todo content:"landing page" value:todo
trait:highlight content:"insight" on:meeting
trait:due content:"urgent" within:project
```

**Note:** Unlike object `content:` which uses FTS5 full-text search, trait `content:` uses simple case-insensitive substring matching on the line where the trait appears. This is sufficient since trait content is a single line.

### Source

Filter by line position. Traits always appear inline in content (not frontmatter).

| Predicate | Meaning |
|-----------|---------|
| `source:inline` | Traits appearing after line 1 |

**Examples:**
```
trait:due source:inline
```

> **Note:** All traits are inline (`@trait(value)` syntax in content). Frontmatter contains type-specific fields, not traits.

### Object Association (Direct) (`on:`)

Filter by the object the trait is directly associated with, either by type or specific object.

| Predicate | Meaning |
|-----------|---------|
| `on:{object:<type> ...}` | Direct parent object matches sub-query |
| `on:[[target]]` | Direct parent object is the specified object |
| `!on:{object:<type> ...}` | Direct parent object does NOT match sub-query |

**Shorthand:** `on:<type>` expands to `on:{object:<type>}`

**Direct target:** `on:[[target]]` checks if the trait's direct parent object is a specific known object.

**Examples:**
```
trait:due on:meeting
trait:due on:{object:meeting}
trait:highlight on:{object:book .status:reading}
trait:due value:past on:{object:project .status:active}
trait:todo on:[[projects/website#tasks]]   # todos directly on this specific section
```

### Object Association (Any Ancestor) (`within:`)

Filter by whether any ancestor object (including direct parent) matches a sub-query or is a specific object.

| Predicate | Meaning |
|-----------|---------|
| `within:{object:<type> ...}` | Some ancestor object matches sub-query |
| `within:[[target]]` | The specified object is an ancestor |
| `!within:{object:<type> ...}` | No ancestor object matches sub-query |

**Shorthand:** `within:<type>` expands to `within:{object:<type>}`

**Direct target:** `within:[[target]]` checks if the trait is anywhere inside a specific object (trait's parent is the object or any descendant of it).

**Examples:**
```
trait:highlight within:date
trait:highlight within:{object:date}
trait:due within:{object:project .status:active}
trait:due (within:book | within:article)
trait:todo within:[[projects/website]]     # todos anywhere inside this project
trait:todo within:[[website]]              # short reference (if unambiguous)
```

### References (`refs:`)

Filter traits by references that appear on the same line as the trait. Use `refs:[[target]]` for a specific target, or `refs:{object:...}` to match targets by a sub-query.

| Predicate | Meaning |
|-----------|---------|
| `refs:[[target]]` | Trait line contains reference to specific target |
| `refs:{object:<type> ...}` | Trait line contains reference to objects matching sub-query |
| `!refs:[[target]]` | Trait line does NOT contain reference to target |
| `!refs:{object:<type> ...}` | Trait line does NOT contain references to any matching objects |

**Examples:**
```
trait:due refs:[[people/freya]]
trait:highlight refs:[[projects/website]]
trait:due refs:{object:person}
trait:due refs:{object:project .status:active}
trait:highlight !refs:[[people/loki]]
```

**Note:** The `refs:` predicate for traits matches references that appear on the same line as the trait annotation. This is useful for finding tasks assigned to specific people (`@due(tomorrow) Send report to [[people/freya]]`) or highlights that reference specific projects.

---

## Boolean Composition

### Operators

| Operator | Syntax | Binding |
|----------|--------|---------|
| NOT | `!` prefix | Tightest |
| AND | space (implicit) | Middle |
| OR | `\|` | Loosest |
| Grouping | `( )` | Explicit precedence |

### Precedence

Standard precedence: NOT > AND > OR

`A | B C` = `A | (B AND C)`

Use parentheses to override: `(A | B) C`

### Examples

Multiple predicates of the same kind are allowed — they're just AND'd together:

```
# Multiple field predicates
object:project .status:active .priority:high

# Multiple has: predicates
object:meeting has:due has:remind

# AND (implicit, space-separated)
object:project .status:active has:due

# OR (with |)
object:project (.status:active | .status:backlog)

# NOT (with !)
object:project !has:deprecated

# Combined
object:project .status:active !has:deprecated
object:project (.status:active | .status:backlog) !has:deprecated
object:meeting (has:{trait:due value:past} | has:{trait:remind value:past})
```

---

## Sub-Query Composition

Predicates can contain sub-queries in curly braces. Each sub-query follows all the same rules.

### Object Query with Trait Sub-Query

```
object:meeting has:{trait:due value:past source:inline}
```

The `has:{...}` contains a full trait query.

### Trait Query with Object Sub-Query

```
trait:highlight on:{object:book .status:reading}
```

The `on:{...}` contains a full object query.

### Deep Nesting

```
trait:due on:{object:project has:{trait:priority value:high}}
```

"Due traits directly on projects that have high-priority traits"

### Combining Direct and Ancestor

```
trait:highlight within:{object:date child:{object:project .status:active}}
```

"Highlights anywhere within daily notes that have an active project as a direct child"

### OR Across Sub-Queries

Each sub-query must return one type. OR happens at the predicate level:

```
# CORRECT: Two sub-queries, OR'd at filter level
trait:highlight (on:{object:book .status:reading} | on:{object:article .status:reading})

# INVALID: Single sub-query returning two types
trait:highlight on:{object:book | object:article}  # ✗ Not allowed
```

---

## Sub-Query Syntax

Inside sub-query curly braces, write full queries with explicit type prefixes:

**Trait sub-query** (inside `has:{...}`, `contains:{...}`)
```
trait:<name> [<trait-predicates>...]
```

**Object sub-query** (inside `on:{...}`, `within:{...}`, `parent:{...}`, `ancestor:{...}`, `child:{...}`, `descendant:{...}`, `refs:{...}`)
```
object:<type> [<object-predicates>...]
```

**Shorthand:** For simple type/trait-only sub-queries, omit the braces:
- `has:due` → `has:{trait:due}`
- `contains:todo` → `contains:{trait:todo}`
- `parent:date` → `parent:{object:date}`
- `descendant:section` → `descendant:{object:section}`
- `on:meeting` → `on:{object:meeting}`

**Direct target:** For hierarchy and association predicates, use `[[target]]` instead of a sub-query to match a specific known object:
- `parent:[[projects/website]]` — parent is this specific project
- `ancestor:[[daily/2025-02-01]]` — this date is an ancestor
- `child:[[daily/2025-02-01#standup]]` — this meeting is a child
- `descendant:[[projects/website#tasks]]` — this section is a descendant
- `on:[[projects/website#tasks]]` — trait's direct parent is this section
- `within:[[projects/website]]` — trait is anywhere inside this project

Short references work if unambiguous (e.g., `within:[[website]]`). Ambiguous references return an error with matching options.

---

## Validation Rules

Recursively validate each query:

1. Query returns exactly one kind of result (objects of a single type, or traits of a single name)
2. Query uses only predicates valid for its query type
3. Sub-queries inside predicates are themselves valid queries
4. Boolean composition is well-formed

---

## Full Examples

```bash
# Active projects
object:project .status:active

# People with email
object:person .email:*

# People without email  
object:person !.email:*

# Meetings with overdue items
object:meeting has:{trait:due value:past}

# Meetings without any due traits
object:meeting !has:due

# Meetings that are direct children of daily notes
object:meeting parent:date

# Meetings anywhere within daily notes (any depth)
object:meeting ancestor:date

# All overdue items
trait:due value:past

# Overdue items directly on active projects
trait:due value:past on:{object:project .status:active}

# Overdue items anywhere within active projects
trait:due value:past within:{object:project .status:active}

# Highlights in books or articles that are being read
trait:highlight (on:{object:book .status:reading} | on:{object:article .status:reading})

# Due items that reference a specific person (e.g., tasks assigned to someone)
trait:due refs:[[people/freya]]

# Highlights that reference any active project
trait:highlight refs:{object:project .status:active}

# Traits with specific content on their line
trait:todo content:"refactor"
trait:highlight content:"important" on:book

# People whose pages mention "colleague"
object:person content:"colleague"

# Active projects with deadline-related content
object:project .status:active content:"deadline"

# Projects with active status OR having high-priority traits, but NOT deprecated
object:project (.status:active | has:{trait:priority value:high}) !has:deprecated

# Due traits on projects that have high-priority traits
trait:due on:{object:project has:{trait:priority value:high}}

# Complex: meetings whose ancestor is a daily note that has a child project with status active
object:meeting ancestor:{object:date child:{object:project .status:active}}

# Meetings that reference a specific person
object:meeting refs:[[people/freya]]

# Meetings that reference any active project
object:meeting refs:{object:project .status:active}

# Meetings that don't reference a specific project
object:meeting !refs:[[projects/website]]

# Projects with any todo anywhere in their hierarchy (self or nested sections)
object:project contains:{trait:todo}

# Active projects with incomplete todos
object:project .status:active contains:{trait:todo value:todo}

# Projects with sections nested inside
object:project descendant:section

# Daily notes that contain overdue items anywhere
object:date contains:{trait:due value:past}

# Difference between has: and contains:
object:project has:{trait:due}              # Due trait directly on project
object:project contains:{trait:due}         # Due trait anywhere (including sections)
```

---

## Design Decisions

1. **No cross-type unions**: A query returns one type. Use separate queries and combine results if needed.
2. **Direct vs deep hierarchy**: Separate predicates for each direction and depth:
   - Up: `parent:` (direct) vs `ancestor:` (any depth)
   - Down: `child:` (direct) vs `descendant:` (any depth)
   - Object→Trait: `has:` (direct) vs `contains:` (subtree)
   - Trait→Object: `on:` (direct) vs `within:` (ancestors)
3. **Traits are inline-only**: `trait:due` returns all inline `@due` annotations in content.
4. **Explicit predicates**: No shorthand value syntax (`=past`), use `value:past` for clarity.
5. **Dot prefix for fields**: `.status:active` distinguishes fields from keywords, avoiding collisions.
6. **Curly braces for sub-queries**: `has:{trait:due}` vs parentheses `()` for boolean grouping.
7. **Explicit sub-query types**: `parent:{object:date}` not `parent:{date}` for consistency.
8. **Unified "matches" semantics**: `.<field>:<value>` means "equals" for single values, "contains" for arrays.
9. **Temporal queries deferred**: Use git history for temporal needs; may add dedicated support later.

---

## Known Gaps (Future Work)

Features identified as valuable but not yet designed:

### Critical

1. **Comparison operators**: Beyond equality for dates and numbers.
   ```
   # Proposed syntax
   trait:due value:<today
   object:project .priority:>3
   ```
   Essential for date-based queries (`before`, `after`, `between`).

3. **Sorting**: Order results by field or trait.
   ```
   # Proposed syntax
   object:project .status:active sort:.updated
   ```
   Essential for practical use.

### Important

4. **Limiting**: Cap result count.
   ```
   # Proposed syntax
   trait:highlight limit:10
   ```

5. **String matching**: Contains, starts-with, regex.
   ```
   # Proposed syntax
   object:project .name:~website
   ```

### Deferred

6. **Aggregations**: COUNT, SUM, GROUP BY — agents can compute these.
7. **Position-based selection**: First, last, nth — not natural for this domain.
8. **Sibling queries**: Covered by composing parent/child predicates.

### Future Extension Points

The syntax has clean extension paths:
- **Operator prefixes** after `:` → `>`, `<`, `>=`, `<=`, `~` (regex), `=` (exact array)
- **Dot notation** for field properties → `.tags.length:>2`, `.tags.all:value`
- **Range syntax** → `.date:2024-01..2024-06` for between
