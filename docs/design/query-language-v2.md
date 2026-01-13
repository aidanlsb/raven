# Query Language v2 — Design Spec

This document specifies the redesigned Raven query language. Key changes from v1:

1. **Operator-based equality**: `==` instead of `:` for value comparison
2. **String matching operators**: `~=`, `^=`, `$=`, `=~` for contains/starts/ends/regex
3. **Pipeline operator**: `|>` separates selection from post-processing
4. **Computed aggregations**: Assign results to named values for filtering/sorting
5. **Extended navigation**: `child()`, `ancestors()`, `descendants()` functions
6. **No shorthands**: All syntax is explicit (no `has:due` → `has:{trait:due}`)

---

## Query Types

| Type | Syntax | Returns |
|------|--------|---------|
| Object query | `object:<type>` | Objects of that type |
| Trait query | `trait:<name>` | Traits of that name |

---

## Operators

| Operator | Meaning | Context |
|----------|---------|---------|
| `:` | Type specification | `object:project`, `trait:due` |
| `==` | Equals | `.status==active` |
| `!=` | Not equals | `.status!=done` |
| `>` | Greater than | `.priority>5` |
| `<` | Less than | `.created<2025-01-01` |
| `>=` | Greater or equal | `.priority>=3` |
| `<=` | Less or equal | `.created<=2025-12-31` |
| `~=` | Contains (string) | `.name~="website"` |
| `^=` | Starts with | `.name^="My"` |
| `$=` | Ends with | `.name$=".md"` |
| `=~` | Regex match | `.name=~/^web.*api$/` |
| `=` | Assignment | After `\|>` only |
| `.` | Field access | `.status`, `_.fieldname` |
| `!` | Negation | `!has:{...}`, `!.archived==true` |
| `\|` | Boolean OR | Between predicates |
| `\|>` | Pipeline | Transition to post-processing |
| `{...}` | Sub-query | `has:{trait:todo}` |
| `[[...]]` | Direct reference | `refs:[[people/freya]]` |
| `(...)` | Grouping / function args | `(.a==1 \| .b==2)`, `count(...)` |
| `*` | Wildcard (exists) | `.email==*` |

---

## Object Query Predicates

### Field Predicates

| Syntax | Meaning |
|--------|---------|
| `.field==value` | Field equals value |
| `.field!=value` | Field not equals value |
| `.field>value` | Field greater than |
| `.field<value` | Field less than |
| `.field>=value` | Field greater or equal |
| `.field<=value` | Field less or equal |
| `.field~="val"` | Field contains substring |
| `.field^="val"` | Field starts with |
| `.field$="val"` | Field ends with |
| `.field=~/pat/` | Field matches regex |
| `.field==*` | Field exists |
| `.field!=*` | Field does not exist |

For array fields, `.field==value` matches if the array contains the value.

### Trait Predicates

| Syntax | Meaning |
|--------|---------|
| `has:{trait:... ...}` | Has direct trait matching sub-query |
| `!has:{trait:... ...}` | Does not have direct trait matching |
| `contains:{trait:... ...}` | Has trait in subtree matching sub-query |
| `!contains:{trait:... ...}` | No trait in subtree matching |

### Hierarchy Predicates

| Syntax | Meaning |
|--------|---------|
| `parent:{object:... ...}` | Direct parent matches sub-query |
| `parent:[[target]]` | Direct parent is specific object |
| `ancestor:{object:... ...}` | Some ancestor matches sub-query |
| `ancestor:[[target]]` | Specific object is an ancestor |
| `child:{object:... ...}` | Has direct child matching sub-query |
| `child:[[target]]` | Specific object is a direct child |
| `descendant:{object:... ...}` | Has descendant matching sub-query |
| `descendant:[[target]]` | Specific object is a descendant |

### Reference Predicates

| Syntax | Meaning |
|--------|---------|
| `refs:{object:... ...}` | References object matching sub-query |
| `refs:[[target]]` | References specific object |
| `refd:{object:... ...}` | Referenced by object matching sub-query |
| `refd:[[target]]` | Referenced by specific object |

### Content Predicate

| Syntax | Meaning |
|--------|---------|
| `content:"term"` | Full-text search on content |
| `!content:"term"` | Content does not contain term |

---

## Trait Query Predicates

### Value Predicates

| Syntax | Meaning |
|--------|---------|
| `value==val` | Value equals |
| `value!=val` | Value not equals |
| `value>val` | Value greater than |
| `value<val` | Value less than |
| `value>=val` | Value greater or equal |
| `value<=val` | Value less or equal |

### Association Predicates

| Syntax | Meaning |
|--------|---------|
| `on:{object:... ...}` | Direct parent object matches sub-query |
| `on:[[target]]` | Direct parent is specific object |
| `within:{object:... ...}` | Some ancestor object matches sub-query |
| `within:[[target]]` | Specific object is an ancestor |

### Co-location Predicate

| Syntax | Meaning |
|--------|---------|
| `at:{trait:... ...}` | Co-located with trait matching sub-query |
| `!at:{trait:... ...}` | Not co-located with matching trait |

### Reference Predicate

| Syntax | Meaning |
|--------|---------|
| `refs:{object:... ...}` | Trait line references object matching sub-query |
| `refs:[[target]]` | Trait line references specific object |

### Content Predicate

| Syntax | Meaning |
|--------|---------|
| `content:"term"` | Line contains term |
| `!content:"term"` | Line does not contain term |

---

## Boolean Composition

| Operator | Syntax | Precedence |
|----------|--------|------------|
| NOT | `!` prefix | Highest |
| AND | space (implicit) | Middle |
| OR | `\|` | Lowest |
| Grouping | `(...)` | Explicit |

Example:
```
object:project (.status==active | .status==backlog) !.archived==true
```

---

## Pipeline (`|>`)

Separates selection (predicates) from post-processing (aggregation, filtering, sorting, limiting).

```
<query> |> <processing steps>
```

Processing steps are space-separated and execute in order.

---

## Post-Processing: Current Result Reference

| Syntax | Meaning |
|--------|---------|
| `_` | The current result being processed |
| `_.field` | Field on current result |

---

## Post-Processing: Navigation Functions

**Return single object** — usable in predicates:

| Function | Returns |
|----------|---------|
| `parent(_)` | Direct parent of current result |
| `child(_)` | Direct child of current result |

**Return sets** — usable only in aggregation functions:

| Function | Returns |
|----------|---------|
| `ancestors(_)` | All ancestors of current result |
| `descendants(_)` | All descendants of current result |
| `refs(_)` | Objects referenced by current result |
| `refd(_)` | Objects that reference current result |

---

## Post-Processing: Aggregation Functions

| Syntax | Meaning |
|--------|---------|
| `count({subquery})` | Count matching items |
| `count(refs(_))` | Count outgoing references |
| `count(refd(_))` | Count incoming references |
| `count(ancestors(_))` | Count ancestors |
| `count(descendants(_))` | Count descendants |
| `min({subquery})` | Minimum value of matching traits |
| `max({subquery})` | Maximum value of matching traits |
| `sum({subquery})` | Sum of values of matching traits |

### Assignment

Computed values are assigned to names for use in subsequent steps:

```
name = count({trait:todo})
```

Sub-queries can use `_` to relate to current result:

```
count({object:meeting ancestor:_})       # meetings inside current
count({object:project parent:parent(_)}) # sibling projects
```

---

## Post-Processing: Transformation Functions

| Syntax | Meaning |
|--------|---------|
| `filter(expr)` | Keep results where expr is true |
| `sort(field, dir)` | Order by field; dir is `asc` or `desc` |
| `limit(n)` | Cap results at n |

### Filter Expressions

Filter expressions can reference computed values and use comparison operators:

```
filter(todos > 5)
filter(overdue >= 1)
filter(meetings == 0)
```

### Sort Expressions

Sort can use fields or computed values:

```
sort(.name, asc)
sort(todos, desc)
sort(overdue, desc)
```

---

## Full Examples

```
# Simple selection
object:project .status==active

# String matching
object:project .name~="api" .name$="-service"

# Regex matching
object:project .name=~/^web-.*-api$/

# Boolean logic
object:project (.status==active | .status==backlog) !.archived==true

# With sub-query
object:meeting has:{trait:due value==past}

# Trait query
trait:todo value==todo within:{object:project .status==active}

# Simple pipeline
object:project .status==active |> sort(.name, asc) limit(10)

# Aggregation with filtering
object:project .status==active |>
  todos = count({trait:todo value==todo ancestor:_})
  overdue = count({trait:due value==past ancestor:_})
  filter(todos > 0)
  sort(overdue, desc)
  limit(10)

# Complex: projects referenced by yesterday's note with meeting counts
object:project refd:{object:date .date==yesterday} |>
  meetings = count({object:meeting has:{trait:todo} ancestor:_})
  filter(meetings >= 2)
  sort(meetings, desc)

# Sibling project meetings
object:project .status==active |>
  sibling_meetings = count({object:meeting ancestor:{object:project parent:parent(_)}})
  sort(sibling_meetings, desc)

# Reference counting
object:person |>
  mentions = count(refd(_))
  projects = count({object:project refs:_})
  sort(mentions, desc)
  limit(20)
```

---

## Migration from v1

### Syntax Changes

| v1 Syntax | v2 Syntax |
|-----------|-----------|
| `.status:active` | `.status==active` |
| `.priority:>5` | `.priority>5` |
| `.priority:>=5` | `.priority>=5` |
| `value:past` | `value==past` |
| `has:due` | `has:{trait:due}` |
| `parent:date` | `parent:{object:date}` |
| `on:meeting` | `on:{object:meeting}` |
| `sort:_.value` | `\|> sort(_.value, asc)` |
| `sort:min:{trait:due within:_}` | `\|> min_due = min({trait:due within:_}) sort(min_due, asc)` |
| `group:_.parent` | (grouping redesign TBD) |
| `limit:10` | `\|> limit(10)` |

### Removed Shorthands

All shorthands are removed for consistency:

- `has:due` → `has:{trait:due}`
- `contains:todo` → `contains:{trait:todo}`
- `parent:date` → `parent:{object:date}`
- `ancestor:project` → `ancestor:{object:project}`
- `child:meeting` → `child:{object:meeting}`
- `descendant:section` → `descendant:{object:section}`
- `on:meeting` → `on:{object:meeting}`
- `within:project` → `within:{object:project}`
- `at:todo` → `at:{trait:todo}`
- `refd:meeting` → `refd:{object:meeting}`

### New Capabilities

1. **String matching**: `~=`, `^=`, `$=`, `=~` operators
2. **Pipeline post-processing**: `|>` for clean separation
3. **Computed aggregations**: Named values with `=`
4. **Filter on computed values**: `filter(todos > 5)`
5. **Extended navigation**: `ancestors(_)`, `descendants(_)` sets
