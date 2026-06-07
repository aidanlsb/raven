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
type:project has(trait:todo .value==todo)
```

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
