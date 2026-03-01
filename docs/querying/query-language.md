# Query Language

> **Shell Tip:** Wrap query strings in single quotes to avoid shell interpretation of `(`, `)`, `|`, and `!`.
> This page is a reference and decision guide for Raven Query Language (RQL).

## Start Here

### Choose the Right Tool

| Use this | When you want |
|----------|---------------|
| `rvn query` | Structured filtering by type/trait, field values, hierarchy, and references |
| `rvn search` | Free-text discovery when you do not know the structure yet |
| `rvn backlinks` | All incoming references to one specific target |
| `rvn outlinks` | All outgoing references from one specific target |
| `rvn read` | Full file content after you already identified relevant objects |

### Choose Query Type

| Query type | Returns | Best for |
|------------|---------|----------|
| `object:<type> ...` | Objects | Find files/sections by frontmatter fields and structural relationships |
| `trait:<name> ...` | Trait instances | Find inline annotations (`@todo`, `@due`, etc.) and surrounding content|

Core rules:
1. Every query returns exactly one kind of result (objects or traits).
2. Queries can nest arbitrarily, e.g. `object:project has(trait:...)`.
3. Boolean composition is `AND` (space), `OR` (`|`), and `NOT` (`!`).

## Query Shapes

### Object Query

```text
object:<type> [predicates...]
```

Examples:

```text
object:project
object:project .status==active
object:meeting has(trait:due .value<today)
object:project encloses(trait:todo .value==todo)
```

### Trait Query

```text
trait:<name> [predicates...]
```

Examples:

```text
trait:due
trait:due .value<today
trait:highlight on(object:book .status==reading)
```

## Syntax Building Blocks

| Element | Syntax | Example |
|---------|--------|---------|
| Field access | `.` prefix | `.status==active`, `.value<today` |
| Equality / inequality | `==`, `!=` | `.status!=done` |
| Comparison | `<`, `>`, `<=`, `>=` | `.priority>5` |
| Presence | `exists(.field)` | `exists(.email)`, `!exists(.email)` |
| Scalar membership | `in(.field, [a,b])` | `in(.status, [active,backlog])` |
| Array quantifiers | `any()` / `all()` / `none()` | `any(.tags, _ == "urgent")` |
| String functions | `contains()`, `startswith()`, `endswith()`, `matches()` | `contains(.name, "website")` |
| References | `[[...]]` | `[[people/freya]]` |
| Raw string | `r"..."` | `matches(.path, r"C:\Users\.*")` |

Notes:
- `.field==*` / `!.field==*` are not supported. Use `exists(.field)` / `!exists(.field)`.
- String functions are case-insensitive by default. Pass `true` as a third argument for case-sensitive matching.
- `matches()` accepts either a quoted pattern or regex literal (`/pattern/`).

## Object Query Predicates

### Field Predicates

| Predicate | Meaning |
|-----------|---------|
| `.field==value` | Field equals value |
| `.field!=value` | Field does not equal value |
| `.field>value`, `.field<value` | Numeric/date comparison |
| `.field>=value`, `.field<=value` | Inclusive comparison |
| `exists(.field)` | Field has a value |
| `in(.field, [a,b,c])` | Field matches any listed scalar value |

Examples:

```text
object:project .status==active
object:project .title=="Website Redesign"
object:person exists(.email)
object:person !exists(.email)
object:project in(.status, [active,paused])
```

For `ref` and `ref[]` fields (from `schema.yaml`), comparison values are resolved as reference targets, including unbracketed shorthand such as `.company==cursor`.

### String Matching

| Function | Meaning |
|----------|---------|
| `contains(.field, "str")` | Substring match |
| `startswith(.field, "str")` | Prefix match |
| `endswith(.field, "str")` | Suffix match |
| `matches(.field, "pattern")` / `matches(.field, /pattern/)` | Regex match |

Case-sensitive example:

```text
contains(.name, "API", true)
```

### Array Predicates

Use quantifiers for array fields. `_` represents the current array element.

```text
object:project any(.tags, _ == "urgent")
object:project all(.tags, startswith(_, "feature-"))
object:project none(.tags, _ == "deprecated")
```

### Structural Predicates

| Predicate | Meaning |
|-----------|---------|
| `has(trait:...)` | Object has matching trait directly on itself |
| `encloses(trait:...)` | Object has matching trait on self or any descendant |
| `parent(...)` | Direct parent matches query or target |
| `ancestor(...)` | Any ancestor matches query or target |
| `child(...)` | Direct child matches query or target |
| `descendant(...)` | Any descendant matches query or target |
| `refs(...)` | Object references a target or query match |
| `refd(...)` | Object is referenced by a source or query match |
| `content("term")` | Full-text term in object content |

`parent`, `ancestor`, `child`, `descendant`, and `refs` accept:
- Nested query: `parent(object:date)`
- Wikilink: `parent([[daily/2026-01-10]])`
- Target shorthand: `parent(daily/2026-01-10)`

Examples:

```text
object:project has(trait:due)
object:project encloses(trait:todo .value==todo)
object:meeting parent(object:date)
object:meeting refs([[projects/website]])
object:meeting refs(object:project .status==active)
object:project refd(object:meeting)
```

## Trait Query Predicates

### Value Predicates

| Predicate | Meaning |
|-----------|---------|
| `.value==val`, `.value!=val` | Value equality/inequality |
| `.value>val`, `.value<val` | Numeric/date comparison |
| `.value>=val`, `.value<=val` | Inclusive comparison |
| `in(.value, [a,b,c])` | Value is one of listed values |

Date/date-time comparisons also support relative keywords:
- `today`
- `tomorrow`
- `yesterday`

Examples:

```text
trait:due .value<today
trait:due in(.value, [today,tomorrow])
trait:due .value<=2026-03-01
```

### Trait Structural Predicates

| Predicate | Meaning |
|-----------|---------|
| `on(...)` | Trait is directly on matching object |
| `within(...)` | Trait is anywhere within matching object subtree |
| `at(trait:...)` | Co-located with matching trait (same file and line) |
| `refs(...)` | Trait's line references target or query match |
| `content("term")` | Trait's line contains term |

Examples:

```text
trait:due on(object:meeting)
trait:todo within(object:project .status==active)
trait:due at(trait:todo)
trait:due refs([[people/freya]])
trait:todo content("refactor")
```

`refd(...)` is available on object queries, not trait queries.

## Boolean Composition

| Operator | Syntax | Precedence |
|----------|--------|------------|
| NOT | `!pred` | Highest |
| AND | `pred1 pred2` | Middle |
| OR | `pred1 \| pred2` | Lowest |
| Grouping | `( ... )` | Explicit |

Examples:

```text
object:project .status==active has(trait:due)
object:project (.status==active | .status==backlog) !.archived==true
object:meeting (has(trait:due .value<today) | has(trait:remind .value<today))
```

## Running and Applying Queries

### Inspect Results

```bash
rvn query 'object:project .status==active' --json
rvn query 'trait:due .value<today' --ids
rvn query 'object:project refs([[companies/acme]])' --refresh --json
```

### Save and Reuse Queries

```bash
rvn query add overdue 'trait:due .value<today' --json
rvn query overdue --json
rvn query --list --json
```

Saved query placeholders (`{{args.name}}`) must be declared in `raven.yaml` under `queries.<name>.args`.

### Bulk Operations by Query Type

- Object query `--apply` supports: `set`, `add`, `delete`, `move`.
- Trait query `--apply` supports only: `update <new_value>`.
- All `--apply` operations preview by default; use `--confirm` to apply.

Examples:

```bash
# Object query bulk update
rvn query 'object:project has(trait:due .value<today)' --apply 'set status=overdue' --confirm

# Trait query bulk update
rvn query 'trait:todo .value==todo' --apply 'update done' --confirm
```

## Related Docs

- Query-driven bulk changes: `vault-management/bulk-operations.md`
- Queryable field/trait definitions: `types-and-traits/schema.md`
- Hierarchy and object IDs (`#fragment`, sections, embedded objects): `types-and-traits/file-format.md`
- Saved query configuration in `raven.yaml`: `getting-started/configuration.md`
- MCP query tool usage: `agents/mcp.md`
