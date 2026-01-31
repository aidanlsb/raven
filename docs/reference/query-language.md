# Query Language

> **Shell Tip:** Always wrap queries in single quotes to prevent shell interpretation of special characters like `(`, `)`, `|`, and `!`. See the [CLI Guide](../guide/cli.md#shell-quoting) for details.

## Core Principles

1. **Two query types**: object queries and trait queries
2. **Single return type**: every query (including nested queries) returns exactly one type
3. **Boolean composition**: predicates can be combined with AND (space), OR (`|`), and NOT (`!`)
4. **Nested queries**: structural predicates accept nested `object:` / `trait:` queries directly (no `{...}` syntax)

## Syntax Conventions

| Element | Syntax | Example |
|---------|--------|---------|
| Field access | `.` prefix | `.status==active` |
| Equality | `==` | `.status==active`, `.value==past` |
| Not equals | `!=` | `.status!=done` |
| Comparison | `<`, `>`, `<=`, `>=` | `.priority>5` |
| Substring | `contains()` | `contains(.name, "website")` |
| Starts with | `startswith()` | `startswith(.name, "My")` |
| Ends with | `endswith()` | `endswith(.name, ".md")` |
| Regex | `matches()` | `matches(.name, "^api.*$")` |
| Membership (scalar) | `in()` | `in(.status, [active,backlog])`, `in(.value, [past,today])` |
| Array any/all/none | `any()` / `all()` / `none()` | `any(.tags, _ == "urgent")` |
| Presence | `exists()` | `exists(.email)`, `!exists(.email)` |
| References | `[[...]]` | `[[people/freya]]` |
| Raw string | `r"..."` | `matches(.path, r"C:\Users\.*")` |

## Query Types

### Object Query

Returns objects of a single type.

```
object:<type> [<predicates>...]
```

**Examples:**

```
object:project
object:project .status==active
object:meeting has(trait:due .value==past)
object:project encloses(trait:todo .value==todo)
```

### Trait Query

Returns traits of a single name.

```
trait:<name> [<predicates>...]
```

**Examples:**

```
trait:due
trait:due .value==past
trait:highlight on(object:book .status==reading)
```

---

## Object Query Predicates

### Field-Based

Filter by object frontmatter fields. Fields use dot prefix.

| Predicate | Meaning |
|-----------|---------|
| `.field==value` | Field equals value |
| `.field!=value` | Field does NOT equal value |
| `exists(.field)` | Field exists (has a value) |
| `.field>value` | Field is greater than value |
| `.field<value` | Field is less than value |
| `.field>=value` | Field is greater or equal |
| `.field<=value` | Field is less or equal |

**Examples:**

```
object:project .status==active
object:project .title=="My Project"
object:person exists(.email)
object:person !exists(.email)
object:project .priority>5
```

### String Matching Functions

| Function | Meaning |
|----------|---------|
| `contains(.field, "str")` | Field contains substring |
| `startswith(.field, "str")` | Field starts with |
| `endswith(.field, "str")` | Field ends with |
| `matches(.field, "pattern")` | Field matches regex |

All string functions are **case-insensitive by default**. Add `true` as third argument for case-sensitive:

```
contains(.name, "API", true)
```

### Array Field Matching

For array fields, use quantifier functions (`any`, `all`, `none`). The `_` symbol represents the current element being tested.

**Examples:**

```
object:project any(.tags, _ == "urgent")
object:project all(.tags, startswith(_, "feature-"))
object:project none(.tags, _ == "deprecated")
```

### Structural Predicates (Function Form)

| Predicate | Meaning |
|-----------|---------|
| `has(trait:...)` | Object has matching trait directly on the object |
| `encloses(trait:...)` | Object has matching trait on self or any descendant object |
| `parent(object:...)` | Direct parent matches |
| `ancestor(object:...)` | Some ancestor matches |
| `child(object:...)` | Has direct child matching |
| `descendant(object:...)` | Has descendant matching |
| `refs([[target]])` / `refs(object:...)` | Outgoing references |
| `refd([[source]])` / `refd(object:...)` / `refd(trait:...)` | Incoming references |
| `content("term")` | Full-text search over object content |

**Examples:**

```
object:project has(trait:due)
object:project encloses(trait:todo .value==todo)
object:meeting parent(object:date)
object:meeting refs([[projects/website]])
object:meeting refs(object:project .status==active)
object:project refd(object:meeting)
object:person content("colleague")
```

---

## Trait Query Predicates

### Value-Based

| Predicate | Meaning |
|-----------|---------|
| `.value==val` | Value equals val |
| `.value!=val` | Value does NOT equal val |
| `.value>val` | Value greater than |
| `.value<val` | Value less than |
| `.value>=val` | Value greater or equal |
| `.value<=val` | Value less or equal |

For string matching on values, use `contains()`, `startswith()`, `endswith()`, or `matches()` with `.value` as the first argument.

### Structural Predicates (Function Form)

| Predicate | Meaning |
|-----------|---------|
| `on(object:...)` / `on([[target]])` | Trait is directly on object |
| `within(object:...)` / `within([[target]])` | Trait is within object subtree |
| `at(trait:...)` | Co-located traits (same file+line) |
| `refs([[target]])` / `refs(object:...)` | Line references target |
| `content("term")` | Line content contains term |

**Examples:**

```
trait:due on(object:meeting)
trait:todo within(object:project .status==active)
trait:due at(trait:todo)
trait:due refs([[people/freya]])
trait:todo content("refactor")
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
object:project .status==active has(trait:due)
object:project (.status==active | .status==backlog) !.archived==true
object:meeting (has(trait:due .value==past) | has(trait:remind .value==past))
```

