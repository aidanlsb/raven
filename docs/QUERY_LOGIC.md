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
| `.<field>:*` | Field exists (has any value) |
| `!.<field>:<value>` | Field does NOT equal/contain value |
| `!.<field>:*` | Field does NOT exist (missing) |

For array fields, `.<field>:<value>` matches if the array contains the value.

**Examples:**
```
object:project .status:active
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

Filter by whether object's direct parent matches an object sub-query.

| Predicate | Meaning |
|-----------|---------|
| `parent:{object:<type> ...}` | Direct parent matches sub-query |
| `!parent:{object:<type> ...}` | Direct parent does NOT match sub-query |

**Shorthand:** `parent:<type>` expands to `parent:{object:<type>}`

**Examples:**
```
object:meeting parent:date
object:meeting parent:{object:date}
object:section parent:{object:project .status:active}
```

### Ancestor (Any Depth)

Filter by whether any ancestor matches an object sub-query.

| Predicate | Meaning |
|-----------|---------|
| `ancestor:{object:<type> ...}` | Some ancestor matches sub-query |
| `!ancestor:{object:<type> ...}` | No ancestor matches sub-query |

**Shorthand:** `ancestor:<type>` expands to `ancestor:{object:<type>}`

**Examples:**
```
object:meeting ancestor:date
object:meeting ancestor:{object:date}
object:topic ancestor:{object:meeting ancestor:date}
```

### Child (Direct)

Filter by whether object has at least one direct child matching an object sub-query.

| Predicate | Meaning |
|-----------|---------|
| `child:{object:<type> ...}` | Has child matching sub-query |
| `!child:{object:<type> ...}` | No child matches sub-query |

**Shorthand:** `child:<type>` expands to `child:{object:<type>}`

**Examples:**
```
object:meeting child:topic
object:meeting child:{object:topic}
object:date child:{object:meeting has:due}
```

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

---

## Trait Query Predicates

### Value-Based

Filter by trait value.

| Predicate | Meaning |
|-----------|---------|
| `value:<val>` | Value equals val |
| `!value:<val>` | Value NOT equals val |

**Examples:**
```
trait:due value:past
trait:due !value:past
trait:status value:todo
trait:priority value:high
```

### Source

Filter by where the trait appears. If omitted, both sources are included.

| Predicate | Meaning |
|-----------|---------|
| `source:inline` | Only inline traits (`@trait(value)` in content) |
| `source:frontmatter` | Only frontmatter traits (in YAML header) |

**Examples:**
```
trait:due source:inline
trait:due value:past source:frontmatter
```

### Object Association (Direct) (`on:`)

Filter by the object the trait is directly associated with.

| Predicate | Meaning |
|-----------|---------|
| `on:{object:<type> ...}` | Direct parent object matches sub-query |
| `!on:{object:<type> ...}` | Direct parent object does NOT match sub-query |

**Shorthand:** `on:<type>` expands to `on:{object:<type>}`

**Examples:**
```
trait:due on:meeting
trait:due on:{object:meeting}
trait:highlight on:{object:book .status:reading}
trait:due value:past on:{object:project .status:active}
```

### Object Association (Any Ancestor) (`within:`)

Filter by whether any ancestor object matches a sub-query.

| Predicate | Meaning |
|-----------|---------|
| `within:{object:<type> ...}` | Some ancestor object matches sub-query |
| `!within:{object:<type> ...}` | No ancestor object matches sub-query |

**Shorthand:** `within:<type>` expands to `within:{object:<type>}`

**Examples:**
```
trait:highlight within:date
trait:highlight within:{object:date}
trait:due within:{object:project .status:active}
trait:due (within:book | within:article)
```

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

**Trait sub-query** (inside `has:{...}`)
```
trait:<name> [<trait-predicates>...]
```

**Object sub-query** (inside `on:{...}`, `within:{...}`, `parent:{...}`, `ancestor:{...}`, `child:{...}`, `refs:{...}`)
```
object:<type> [<object-predicates>...]
```

**Shorthand:** For simple type/trait-only sub-queries, omit the braces:
- `has:due` → `has:{trait:due}`
- `parent:date` → `parent:{object:date}`
- `on:meeting` → `on:{object:meeting}`

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

# All overdue items (frontmatter + inline)
trait:due value:past

# Overdue items directly on active projects
trait:due value:past on:{object:project .status:active}

# Overdue items anywhere within active projects
trait:due value:past within:{object:project .status:active}

# Highlights in books or articles that are being read
trait:highlight (on:{object:book .status:reading} | on:{object:article .status:reading})

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
```

---

## Design Decisions

1. **No cross-type unions**: A query returns one type. Use separate queries and combine results if needed.
2. **Direct vs ancestor**: Separate predicates for direct parent (`parent:`, `on:`) vs any ancestor (`ancestor:`, `within:`).
3. **Trait sources unified**: `trait:due` returns both frontmatter and inline traits.
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
