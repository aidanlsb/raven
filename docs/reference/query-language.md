# Query Language

> **Shell Tip:** Always wrap queries in single quotes to prevent shell interpretation of special characters like `{}`, `|`, and `!`. See the [CLI Guide](../guide/cli.md#shell-quoting) for details.

## Core Principles

1. **Two query types**: Object queries and trait queries
2. **Single return type**: Every query (including sub-queries) returns exactly one type
3. **Boolean composition**: Predicates can be combined with AND, OR, NOT
4. **Recursive sub-queries**: Predicates can contain sub-queries that follow all the same rules
5. **Pipeline processing**: Use `|>` to chain post-processing operations

## Syntax Conventions

| Element | Syntax | Example |
|---------|--------|---------|
| Field access | `.` prefix | `.status==active` |
| Equality | `==` | `.status==active`, `value==past` |
| Not equals | `!=` | `.status!=done` |
| Comparison | `<`, `>`, `<=`, `>=` | `.priority>5` |
| Contains | `includes()` | `includes(.name, "website")` |
| Starts with | `startswith()` | `startswith(.name, "My")` |
| Ends with | `endswith()` | `endswith(.name, ".md")` |
| Regex | `matches()` | `matches(.name, "^api.*$")` |
| Array any | `any()` | `any(.tags, _ == "urgent")` |
| Array all | `all()` | `all(.tags, startswith(_, "feat-"))` |
| Subquery | `{...}` | `has:{trait:due}` |
| Pipeline | `\|>` | `\|> sort(.name) limit(10)` |
| Grouping | `(...)` | `(.status==active \| .status==done)` |
| References | `[[...]]` | `[[people/freya]]` |
| Null check | `isnull()`, `notnull()` | `notnull(.email)` |
| Raw string | `r"..."` | `matches(.path, r"C:\Users\.*")` |

## Query Types

### Object Query

Returns objects of a single type.

```
object:<type> [<predicates>...] [|> <pipeline>]
```

**Examples:**
```
object:project
object:project .status==active
object:meeting has:{trait:due value==past}
object:project .status==active |> sort(.name, asc) limit(10)
```

### Trait Query

Returns traits of a single name.

```
trait:<name> [<predicates>...] [|> <pipeline>]
```

**Examples:**
```
trait:due
trait:due value==past
trait:highlight on:{object:book .status==reading}
```

---

## Object Query Predicates

### Field-Based

Filter by object frontmatter fields. Fields use dot prefix.

| Predicate | Meaning |
|-----------|---------|
| `.field==value` | Field equals value |
| `.field!=value` | Field does NOT equal value |
| `notnull(.field)` | Field exists (has any value) |
| `isnull(.field)` | Field does NOT exist |
| `.field>value` | Field is greater than value |
| `.field<value` | Field is less than value |
| `.field>=value` | Field is greater or equal |
| `.field<=value` | Field is less or equal |

**Examples:**
```
object:project .status==active
object:project .title=="My Project"
object:person notnull(.email)
object:person isnull(.email)
object:project .priority>5
object:project .created>=2025-01-01
```

### String Matching Functions

For string matching, use function-style predicates:

| Function | Meaning |
|----------|---------|
| `includes(.field, "str")` | Field contains substring |
| `startswith(.field, "str")` | Field starts with |
| `endswith(.field, "str")` | Field ends with |
| `matches(.field, "pattern")` | Field matches regex |

All string functions are **case-insensitive by default**. Add `true` as third argument for case-sensitive:
```
includes(.name, "API", true)   # case-sensitive
```

Use raw strings (`r"..."`) for regex patterns to avoid escaping:
```
matches(.path, r"C:\Users\.*")
```

**Examples:**
```
object:project includes(.name, "website")
object:project startswith(.name, "api-")
object:project endswith(.name, "-service")
object:project matches(.name, "^web-.*-api$")
```

### Array Field Matching

For array fields, use quantifier functions:

| Function | Meaning |
|----------|---------|
| `any(.field, predicate)` | Any element matches predicate |
| `all(.field, predicate)` | All elements match predicate |
| `none(.field, predicate)` | No element matches predicate |

The `_` symbol represents the current element being tested.

**Examples:**
```
object:project any(.tags, _ == "urgent")
object:project all(.tags, startswith(_, "feature-"))
object:project none(.tags, _ == "deprecated")
object:project any(.tags, _ == "urgent" | _ == "critical")
object:project any(.tags, includes(_, "api"))
```

### Trait-Based (`has:`)

Filter by whether object contains matching traits.

| Predicate | Meaning |
|-----------|---------|
| `has:{trait:<name> ...}` | Has trait matching sub-query |
| `!has:{trait:<name> ...}` | Does NOT have trait matching |

**Examples:**
```
object:project has:{trait:due}
object:meeting has:{trait:due value==past}
object:meeting !has:{trait:due value==past}
```

### Hierarchy Predicates

| Predicate | Meaning |
|-----------|---------|
| `parent:{object:<type> ...}` | Direct parent matches sub-query |
| `parent:[[target]]` | Direct parent is specific object |
| `ancestor:{object:<type> ...}` | Some ancestor matches |
| `ancestor:[[target]]` | Specific object is an ancestor |
| `child:{object:<type> ...}` | Has child matching |
| `child:[[target]]` | Specific object is a child |
| `descendant:{object:<type> ...}` | Has descendant matching |
| `descendant:[[target]]` | Specific object is a descendant |

**Examples:**
```
object:meeting parent:{object:date}
object:meeting ancestor:{object:date}
object:section parent:[[projects/website]]
object:date descendant:{object:meeting has:{trait:due}}
```

### Contains (`contains:`)

Filter by whether object has matching traits anywhere in its subtree.

| Predicate | Meaning |
|-----------|---------|
| `contains:{trait:<name> ...}` | Has trait on self or any descendant |
| `!contains:{trait:<name> ...}` | No matching trait in subtree |

**Examples:**
```
object:project contains:{trait:todo}
object:project contains:{trait:todo value==todo}
object:date contains:{trait:due value==past}
```

### References (`refs:`)

Filter by what an object references (outgoing links).

| Predicate | Meaning |
|-----------|---------|
| `refs:[[target]]` | References specific target |
| `refs:{object:<type> ...}` | References objects matching sub-query |
| `!refs:[[target]]` | Does NOT reference target |

**Examples:**
```
object:meeting refs:[[projects/website]]
object:meeting refs:{object:project .status==active}
```

### Referenced-By (`refd:`)

Filter by what references this object (incoming links/backlinks).

| Predicate | Meaning |
|-----------|---------|
| `refd:[[source]]` | Referenced by specific source |
| `refd:{object:<type> ...}` | Referenced by objects matching sub-query |
| `refd:{trait:<name> ...}` | Referenced by traits matching sub-query |

**Examples:**
```
object:project refd:[[daily/2025-02-01#standup]]
object:person refd:{object:project .status==active}
```

### Content Search (`content:`)

Filter by full-text search on object content.

| Predicate | Meaning |
|-----------|---------|
| `content:"term"` | Content contains search term(s) |
| `!content:"term"` | Content does NOT contain term |

**Examples:**
```
object:person content:"colleague"
object:project content:"api design"
object:project .status==active content:"deadline"
```

---

## Trait Query Predicates

### Value-Based

Filter by trait value.

| Predicate | Meaning |
|-----------|---------|
| `value==val` | Value equals val |
| `value!=val` | Value does NOT equal val |
| `value>val` | Value greater than |
| `value<val` | Value less than |
| `value>=val` | Value greater or equal |
| `value<=val` | Value less or equal |

For string matching on values, use `includes()`, `startswith()`, `endswith()`, or `matches()`.

#### String Matching on Trait Values

String matching in trait queries uses `.value` as the first argument:

| Function | Meaning |
|----------|---------|
| `includes(.value, "str")` | Value contains substring |
| `startswith(.value, "str")` | Value starts with |
| `endswith(.value, "str")` | Value ends with |
| `matches(.value, "pattern")` | Value matches regex |

All string functions are **case-insensitive by default**. Add `true` as third argument for case-sensitive:
```
includes(.value, "API", true)
```

Use raw strings (`r"..."`) for regex patterns to avoid escaping:
```
matches(.value, r"^TODO.*$")
```

**Examples:**
```
trait:due value==past
trait:due !value==past
trait:due value<2025-01-01
trait:status includes(.value, "progress")
trait:tag startswith(.value, "feat-")
trait:note matches(.value, r"^TODO.*$")
```

### Object Association (`on:`, `within:`)

| Predicate | Meaning |
|-----------|---------|
| `on:{object:<type> ...}` | Direct parent object matches |
| `on:[[target]]` | Direct parent is specific object |
| `within:{object:<type> ...}` | Some ancestor matches |
| `within:[[target]]` | Specific object is an ancestor |

**Examples:**
```
trait:due on:{object:meeting}
trait:highlight on:{object:book .status==reading}
trait:todo within:{object:project .status==active}
trait:todo within:[[projects/website]]
```

### Co-location (`at:`)

Filter traits by co-location (same file and line).

| Predicate | Meaning |
|-----------|---------|
| `at:{trait:<name> ...}` | Co-located with trait matching sub-query |
| `!at:{trait:<name> ...}` | NOT co-located with matching trait |

**Examples:**
```
trait:due at:{trait:todo}
trait:priority at:{trait:due value==past}
```

### References (`refs:`)

Filter traits by references on the same line.

| Predicate | Meaning |
|-----------|---------|
| `refs:[[target]]` | Line contains reference to target |
| `refs:{object:<type> ...}` | Line references objects matching sub-query |

**Examples:**
```
trait:due refs:[[people/freya]]
trait:highlight refs:{object:project}
```

### Content Search (`content:`)

Filter by text content on the same line as the trait.

| Predicate | Meaning |
|-----------|---------|
| `content:"term"` | Line content contains term |
| `!content:"term"` | Line does NOT contain term |

**Examples:**
```
trait:todo content:"refactor"
trait:highlight content:"important"
```

---

## Boolean Composition

| Operator | Syntax | Precedence |
|----------|--------|------------|
| NOT | `!` prefix | Highest |
| AND | space (implicit) | Middle |
| OR | `\|` | Lowest |
| Grouping | `( )` | Explicit |

**Examples:**
```
object:project .status==active has:{trait:due}
object:project (.status==active | .status==backlog) !.archived==true
object:meeting (has:{trait:due value==past} | has:{trait:remind value==past})
```

---

## Pipeline (`|>`)

The pipeline operator separates selection (predicates) from post-processing.

```
<query> |> <stage1> <stage2> ...
```

### Available Stages

| Stage | Syntax | Description |
|-------|--------|-------------|
| Assignment | `name = count({...})` | Compute and store a value |
| Filter | `filter(expr)` | Keep results matching expression |
| Sort | `sort(field, asc\|desc)` | Order results |
| Limit | `limit(n)` | Cap results at n |

### Aggregation Functions

| Function | Description |
|----------|-------------|
| `count({subquery})` | Count matching items |
| `count(refs(_))` | Count outgoing references |
| `count(refd(_))` | Count incoming references |
| `count(ancestors(_))` | Count ancestors |
| `count(descendants(_))` | Count descendants |
| `count(parent(_))` | Count parent (0 or 1) |
| `count(child(_))` | Count direct children |
| `min({trait:...})` | Minimum trait value |
| `max({trait:...})` | Maximum trait value |
| `min(.field, {object:...})` | Minimum field value on objects |
| `max(.field, {object:...})` | Maximum field value on objects |
| `sum(.field, {object:...})` | Sum of field values on objects |
| `sum({trait:...})` | Sum of numeric trait values |

**Important**: All subqueries in pipeline assignments **must** contain a `_` reference to connect to the current result. Subqueries without `_` are invalid because they would produce the same value for every result:

```
# ✅ Valid - subquery references _
object:project |> todos = count({trait:todo within:_})

# ❌ Invalid - subquery doesn't reference _, returns same value for all
object:project |> todos = count({trait:todo})
```

The `_` symbol represents the current result being processed.

**Important**: `_` is **strictly typed** - it always represents the exact item being processed:
- In object pipelines: `_` is the object
- In trait pipelines: `_` is the trait

### Self-Reference Compatibility

#### In Object Pipelines (`_` = object)

| Predicate | Supported | Meaning |
|-----------|-----------|---------|
| `ancestor:_` | ✅ | Has this object as ancestor |
| `descendant:_` | ✅ | Has this object as descendant |
| `parent:_` | ✅ | This object is direct parent |
| `child:_` | ✅ | This object is direct child |
| `refs:_` | ✅ | References this object |
| `refd:_` | ✅ | Referenced by this object |
| `on:_` | ✅ | Trait is directly on this object |
| `within:_` | ✅ | Trait is within this object's subtree |

#### In Trait Pipelines (`_` = trait)

| Predicate | Supported | Meaning |
|-----------|-----------|---------|
| `at:_` | ✅ | Co-located (same file+line) |
| `refd:_` | ✅ | Referenced by this trait's line |
| `has:_` | ✅ | Objects that have this trait |
| `contains:_` | ✅ | Objects that contain this trait in subtree |
| `on:_` | ❌ ERROR | `on:` expects an object |
| `within:_` | ❌ ERROR | `within:` expects an object |
| `ancestor:_` | ❌ ERROR | Traits can't be ancestors |
| `descendant:_` | ❌ ERROR | Traits can't be descendants |
| `parent:_` | ❌ ERROR | Traits can't be parents |
| `child:_` | ❌ ERROR | Traits can't be children |
| `refs:_` | ❌ ERROR | Traits can't be referenced via `[[...]]` |

**Note**: `min`, `max`, and `sum` on object queries require a field specifier (`.field`). For trait queries, these operate on the trait's value directly.

### Filter Expressions

Filter expressions compare computed or field values:

```
filter(todos > 0)
filter(overdue >= 1)
filter(.status == active)
```

### Sort Expressions

Sort by field or computed value. Direction is optional and defaults to ascending:

```
sort(.name)           # ascending (default)
sort(.name, asc)      # explicit ascending
sort(todos, desc)     # descending
```

### Pipeline Examples

```
# Simple sort and limit
object:project .status==active |> sort(.name) limit(10)

# Count and filter
object:project |> todos = count({trait:todo value==todo within:_}) filter(todos > 0)

# Full pipeline
object:project .status==active |>
  todos = count({trait:todo value==todo within:_})
  overdue = count({trait:due value==past within:_})
  filter(todos > 0)
  sort(overdue, desc)
  limit(10)

# Reference counting
object:person |>
  mentions = count(refd(_))
  projects = count({object:project refs:_})
  sort(mentions, desc)
  limit(20)
```

---

## Full Examples

```
# Simple queries
object:project .status==active
trait:due value==past

# String matching
object:project includes(.name, "api") endswith(.name, "-service")
object:project matches(.name, r"^web-.*-api$")

# Array matching
object:project any(.tags, _ == "urgent")
object:project all(.tags, startswith(_, "feature-"))

# Boolean logic
object:project (.status==active | .status==backlog) !.archived==true

# With sub-query
object:meeting has:{trait:due value==past}

# Trait query with hierarchy
trait:todo value==todo within:{object:project .status==active}

# Content search
object:project content:"api design"
trait:highlight content:"important"

# References
object:meeting refs:[[people/freya]]
object:project refd:{object:meeting}

# Pipeline with aggregation
object:project .status==active |>
  todos = count({trait:todo value==todo within:_})
  filter(todos > 0)
  sort(todos, desc)
  limit(10)
```

---

## Predicate Reference

### Predicates by Query Type

| Predicate | Object Query | Trait Query |
|-----------|--------------|-------------|
| `.field==value` | ✅ Frontmatter fields | ❌ |
| `value==val` | ❌ | ✅ Trait value |
| `has:{trait:...}` | ✅ Has matching trait | ❌ |
| `contains:{trait:...}` | ✅ Has trait in subtree | ❌ |
| `parent:` | ✅ Direct parent matches | ❌ |
| `ancestor:` | ✅ Some ancestor matches | ❌ |
| `child:` | ✅ Has child matching | ❌ |
| `descendant:` | ✅ Has descendant matching | ❌ |
| `refs:` | ✅ References target | ✅ Line references target |
| `refd:` | ✅ Referenced by source | ❌ |
| `on:` | ❌ | ✅ Direct parent object |
| `within:` | ❌ | ✅ Ancestor object |
| `at:` | ❌ | ✅ Co-located (same line) |
| `content:` | ✅ Full-text search | ✅ Line content |

---

## Design Decisions

1. **No cross-type unions**: A query returns one type only
2. **Explicit subqueries**: All predicates require explicit `{object:...}` or `{trait:...}` syntax
3. **Operator-based syntax**: `==` for equality, comparison operators standalone
4. **Pipeline separation**: `|>` clearly separates selection from processing
5. **Self-reference with `_`**: Pipeline operations use `_` to reference current result
