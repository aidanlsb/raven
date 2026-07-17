# Query Language Quick Reference

## Query roots

- Object query: `type:<type> [predicates...]`
- Section query: `section [predicates...]`
- Trait query: `trait:<name> [predicates...]`
- Asset query: `asset [predicates...]`

Examples:

```text
type:project .status==active
section .title==Tasks
trait:due .value<today
asset .extension==pdf
```

Every query returns exactly one result kind: objects, sections, traits, or assets. Use `rvn schema`, `rvn schema type <name>`, and `rvn schema trait <name>` to verify local names before writing specific predicates.

## Predicate-by-root capability matrix

Scope/structural predicates are root-dependent. Predicates rejected for a root produce a validation error.

| Predicate / family | `type:<t>` | `section` | `trait:<name>` | `asset` |
|--------------------|:----------:|:---------:|:--------------:|:-------:|
| Field compares, `exists(.field)` | yes | yes (built-in fields) | yes (`.value`) | yes |
| `oneof(.field, [...])` | yes | yes | yes (`.value`) | yes |
| String funcs (`includes`/`startswith`/`endswith`/`matches`) | yes | yes (non-numeric) | yes (`.value`) | yes |
| `any`/`all`/`none` | yes (array fields) | no | yes (array `.value`) | no |
| `has(...)` (direct downward) | yes | yes | no | no |
| `contains(...)` (recursive downward) | yes | yes | no | no |
| `in(...)` (direct upward scope) | no | yes | yes | no |
| `within(...)` (recursive upward scope) | no | yes | yes | no |
| `at(trait:...)` | no | no | yes | no |
| `refs(...)` | yes | yes | yes | no |
| `refd(...)` | yes | yes | no | yes |
| `content("term")` | yes | yes | yes | no |

Scope shortcuts: downward (`has`/`contains`) live on container roots (`type:`/`section`); upward (`in`/`within`) live on contained roots (`trait:`/`section`). `has`/`in` are direct-only; `contains`/`within` are recursive.

Traits attach to the nearest section, so lead with the forgiving forms: use `type:project contains(trait:todo ...)` (not `has`) and `trait:todo within(type:project)` (not `in`). `in(...)` is containment scope, not set membership — for value-in-a-set use `oneof(.field, [...])`.

## Scalar predicates

- Equality and inequality: `.field==value`, `.field!=value`
- Comparisons: `.field<value`, `.field<=value`, `.field>value`, `.field>=value`
- Presence: `exists(.field)`, `!exists(.field)`
- Scalar membership: `oneof(.field, [a,b,"quoted",[[target]]])`

Values can be bare identifiers, quoted strings, or wikilink references. `.field==*` is not supported; use `exists(.field)`.

## String predicates

- `includes(.field, "text")`
- `startswith(.field, "prefix")`
- `endswith(.field, "suffix")`
- `matches(.field, "pattern")` or `matches(.field, /pattern/)`

String functions are case-insensitive by default. Add `true` as the third argument for case-sensitive matching, for example `includes(.name, "API", true)`.

Use string predicates on scalar string-like type fields, trait `.value`, and string asset fields. For array fields, use `any()`/`all()`/`none()` with `_`.

## Array predicates

Array quantifiers apply to type-query array fields:

```text
type:project any(.tags, _ == "urgent")
type:project all(.tags, startswith(_, "feature-"))
type:project none(.tags, _ == "deprecated")
```

Element predicates support `_ == value`, `_ != value`, comparisons, string functions, boolean composition, and wikilink values for `ref[]` fields.

## Type-query predicates

Type queries support field predicates plus:

- `has(trait:...)`: matching trait directly on the object
- `has(section...)`: matching section directly under the object
- `contains(trait:...)`: matching trait recursively in the section tree
- `contains(section...)`: matching section recursively in the section tree
- `refs(...)`: object references a target or matching type query
- `refd(...)`: object is referenced by a target, matching type query, or matching trait query
- `content("term")`: full-text content search within objects

Scope predicates accept nested type/section queries, wikilinks, or unambiguous target shorthands:

```text
section within(type:project)
type:meeting refs([[project/website]])
type:project refd(type:meeting)
type:project contains(trait:todo .value==todo)
```

`has()` matches traits/sections directly on the object; `contains()` searches the whole section tree. Because traits attach to the nearest section, a `@todo` under a `## Tasks` heading is not directly on the object — `type:project has(trait:todo)` usually returns nothing, so prefer `type:project contains(trait:todo .value==todo)`.

## Trait-query predicates

Trait queries support `.value` predicates plus:

- `in(...)`: trait is directly on a matching object or section scope
- `within(...)`: trait is within a matching object or section scope
- `at(trait:...)`: trait is co-located with a matching trait on the same line
- `refs(...)`: trait line references a target or matching type query
- `content("term")`: term appears in the trait line
- `any(.value, ...)`, `all(.value, ...)`, `none(.value, ...)`: element predicates for array-valued traits

Examples:

```text
trait:todo .value==todo within(type:project .status==active)
trait:due at(trait:todo)
trait:todo refs([[person/freya]])
trait:tags any(.value, _ == "raven")
trait:reviewers any(.value, _ == [[person/freya]])
```

`in(...)` matches only the direct scope; `within(...)` matches any ancestor scope. Prefer `within(type:project ...)` for inline traits that live under a heading. `in(...)` is scope containment, not set membership — for "value is one of a set" use `oneof(.value, [...])`.

`refd(...)`, `has(...)`, downward scope predicates, and arbitrary fields other than `.value` are not valid on trait queries.

## Asset-query predicates

Asset queries use derived metadata fields:

- `.id`: stable asset ID, currently the same as `.file_path`
- `.file_path`: vault-relative asset path
- `.filename`: basename including extension
- `.extension`: lowercase extension without the dot
- `.media_type`: MIME type derived from extension when known
- `.size_bytes`: file size in bytes

Examples:

```text
asset .extension==pdf
asset oneof(.extension, [jpg,jpeg,png,webp,gif,svg])
asset startswith(.media_type, "image/")
asset startswith(.file_path, "assets/screenshots/")
asset .size_bytes>1048576
asset refd(type:project .status==active)
asset refd(trait:todo .value==todo)
```

Asset queries support scalar predicates, string predicates on string asset fields, boolean composition, and `refd(...)`. Assets do not support `refs(...)`, `has(...)`, `content(...)`, scope predicates, or array predicates.

## Boolean composition

- `!pred` (NOT), highest precedence
- `pred1 pred2` (AND), middle precedence
- `pred1 | pred2` (OR), lowest precedence
- Use parentheses to force grouping

Example:

```text
type:project (.status==active | .status==backlog) !.archived==true
```

## Dates

Supported relative date keywords in date/date-time comparisons: `today`, `tomorrow`, `yesterday`.

Examples:

```text
trait:due .value<=today
type:date .date>=2026-05-01 .date<=today
```

## Saved query inputs

- Declare placeholders in query text: `{{args.name}}`
- Declare matching inputs with `rvn query saved set <name> '<rql>' --arg name --json`
- Invoke by position or `key=value` inputs

## Apply support by query kind

- Object queries support `--apply "set ..."`, `add`, `delete`, and `move`.
- Trait queries support only `--apply "update <new_value>"`.
- Section and asset queries do not support `--apply`.
- All apply flows preview first; add `--confirm` to execute.
