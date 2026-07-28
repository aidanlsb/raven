# Query Language Quick Reference

## Query roots

- Object query: `type:<type> [predicates...]`
- Section query: `section [predicates...]`
- Trait query: `trait:<name> [predicates...]`
- Link query: `link [predicates...]`

Examples:

```text
type:project .status==active
section .title==Tasks
trait:due .value<today
link .ext==pdf within(type:project)
```

Every query returns exactly one result kind: objects, sections, traits, or
outgoing link edges. Use `rvn schema`, `rvn schema type <name>`, and `rvn schema
trait <name>` to verify local names before writing specific predicates.

## Predicate-by-root capability matrix

Scope/structural predicates are root-dependent. Predicates rejected for a root produce a validation error.

| Predicate / family | `type:<t>` | `section` | `trait:<name>` | `link` |
|--------------------|:----------:|:---------:|:--------------:|:------:|
| Field compares, `exists(.field)` | yes | yes (built-in fields) | yes (`.value`) | compares only; `exists()` is rejected |
| `oneof(.field, [...])` | yes | yes | yes (`.value`) | yes |
| String funcs (`includes`/`startswith`/`endswith`/`matches`) | yes | yes (non-numeric) | yes (`.value`) | yes (string fields) |
| `any`/`all`/`none` | yes (array fields) | no | yes (array `.value`) | no |
| `has(...)` (direct downward) | yes | yes | no | no |
| `contains(...)` (recursive downward) | yes | yes | no | no |
| `in(...)` (direct upward scope) | no | yes | yes | no |
| `within(...)` (recursive upward/source scope) | no | yes | yes | yes |
| `at(trait:...)` | no | no | yes | no |
| `refs(...)` | yes | yes | yes | no |
| `links(...)` | yes | yes | yes | no |
| `refd(...)` | yes | yes | no | no |
| `content("term")` | yes | yes | yes | no |

Scope shortcuts: downward (`has`/`contains`) live on container roots (`type:`/`section`); upward (`in`/`within`) live on contained roots (`trait:`/`section`). `has`/`in` are direct-only; `contains`/`within` are recursive.

Traits attach to the nearest section, so lead with the forgiving forms: use `type:project contains(trait:todo ...)` (not `has`) and `trait:todo within(type:project)` (not `in`). `in(...)` is containment scope, not set membership — for value-in-a-set use `oneof(.field, [...])`.

For both `refs(...)` and `links(...)`, type roots inspect the whole file,
section roots inspect the complete section subtree, and trait roots inspect
only the trait's source line. The bare `link` root supports neither predicate.

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

Use string predicates on scalar string-like type fields, trait `.value`, and
the string-valued shared link fields. For array fields, use
`any()`/`all()`/`none()` with `_`.

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
- `links(...)`: object has an outgoing non-Raven link matching the shared link fields
- `refd(...)`: object is referenced by a target, matching type query, or matching trait query
- `content("term")`: full-text content search within objects

Scope predicates accept nested type/section queries, wikilinks, or target
shorthands. Prefer canonical object IDs in direct targets; short forms are
resolution sugar and can become ambiguous as the vault grows:

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
- `links(...)`: trait line contains an outgoing non-Raven link matching the shared link fields
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

## Outgoing link predicates

Use `links(...)` on type, section, or trait roots to filter source entities by
outgoing non-Raven file/URL links:

```text
type:project links(.ext==pdf)
section links(.scheme==url)
trait:todo links(.is_image==true)
```

`links(...)` and the bare `link` root share exactly these filter fields:
`.source_id`, `.source_type`, `.file_path`, `.line`, `.position_start`,
`.position_end`, `.raw_target`, `.display`, `.is_image`, `.scheme`, `.ext`,
and `.normalized_key`. There is no inverse `linkd()` predicate.

For `.normalized_key`, URLs lowercase the host and strip only default ports
(`:80` for HTTP and `:443` for HTTPS), preserving path case, query, trailing
slash, and fragment. Files inside the vault use vault-relative POSIX keys;
absolute targets outside the vault remain absolute.
Equality on `.normalized_key` and `.raw_target` is case-sensitive; other string
link fields retain case-insensitive equality. Link columns are always present,
so `exists()`/`!exists()` are rejected; use `.ext==""` to match an empty
extension.

## Link-query predicates

The bare `link` root returns one indexed outgoing Markdown link/image edge per
row. Result fields are `.source_id`, `.source_type`, `.file_path`, `.line`,
`.position_start`, `.position_end`, `.raw_target`, `.display`, `.is_image`,
`.scheme`, `.ext`, and `.normalized_key`.

```text
link .ext==pdf within(type:project)
link .is_image==true within(section .title==Resources)
link .scheme==url includes(.display, "documentation")
```

All result fields use the same predicate grammar as `links(...)`; line and
position fields compare numerically. `within(type:...)` filters the source file
object; `within(section ...)` filters by the indexed link line in a matching
section subtree. Link rows are outgoing-only and do not support `in()`,
`refs()`, `links()`, `refd()`, `content()`, structural containment, or arrays.
`link --ids` projects `source_id` once per matching edge, and link queries do
not support `--apply`. Use `trait:<name> links(...)` to find trait lines with
matching links, not `link within(trait:<name>)`.

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
- Section queries support only `move`; link queries do not support `--apply`.
- All apply flows preview first; add `--confirm` to execute.
