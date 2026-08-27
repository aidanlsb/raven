# Raven Query Cheatsheet

Quick reference for common Raven Query Language (RQL) patterns.

## Query shapes

- `type:<type> [predicates...]`
- `section [predicates...]`
- `trait:<name> [predicates...]`
- `link [predicates...]`

## Predicates

- Field comparisons: `.field == value`, `.field != value`, `.field < value`, `.field >= value`
- Trait value: `.value == value` (for `trait:...` queries)
- Array membership:
  - `.field == value` (works for arrays)
  - `any(.field, _ == value)` (explicit)
- List membership: `oneof(.field, [a, b, c])` (this is set membership, NOT the scope predicate `in(...)`)
- String matching: `includes(.field, "text")`, `startswith(...)`, `endswith(...)`, `matches(...)`
- Text search: `content("phrase")`
- References:
  - `refs([[target]])` (objects/traits that reference target)
  - `refs(type:project .status==active)`
  - `links(.ext==pdf)` (type/section/trait has a matching non-Raven outgoing link)
  - Upward scope (on `trait:`/`section`) — direct: `in([[target]])`, `in(type:...)`, `in(section ...)`
  - Upward scope (on `trait:`/`section`), plus source scope on `link` — recursive: `within([[target]])`, `within(type:...)`, `within(section ...)`
  - Downward scope (on `type:`/`section`): `has(section ...)`, `has(trait:...)`, `contains(section ...)`, `contains(trait:...)`
  - Same-line trait match (on `trait:` only): `at(trait:priority .value==high)`

**Scope is root-dependent, and traits attach to the nearest section.** Lead with the forgiving forms: `type:project contains(trait:todo ...)` (not `has`) and `trait:todo within(type:project)` (not `in`). A `@todo` under `## Tasks` is not directly on the project object, so `has`/`in` (direct-only) usually return nothing.

`links(...)` and the bare `link` root share `.source_id`, `.source_type`,
`.file_path`, `.line`, `.position_start`, `.position_end`, `.raw_target`,
`.display`, `.is_image`, `.scheme`, `.ext`, and `.normalized_key`. The root
returns edge rows; `links(...)` filters type/section/trait source rows.

## Sub-queries

Nest queries inside predicates to filter by related objects or traits:

- `refs(type:project .status == active)`
- `within(type:meeting refs([[project/raven]]))`
- `has(trait:due .value < today)`
- `in(type:project .status == active)`

## Boolean logic

Combine predicates with boolean operators:

- Space between predicates = AND
- `|` = OR (use parentheses for grouping)
- `!` = NOT (prefix)
- `(...)` = grouping

Example:
- `trait:todo (.value == todo | .value == doing) !refs([[project/legacy]])`

If a question is vague, clarify the intended result kind first. If two
interpretations are both plausible, run small bounded samples and explain the
difference instead of returning several unbounded result sets.

## Examples

- Open todos for a project page:
  - `trait:todo within([[project/raven]]) .value != done`
- Todos referencing a project:
  - `trait:todo refs([[project/raven]]) .value != done`
- Open todos in briefs:
  - `trait:todo .value == todo within(type:brief)`
- Real traits, not prose mentions:
  - `trait:todo .value == todo`
- Text mentions of a token when structure is unknown:
  - `search "@todo pricing"`
- Todos under a topic section:
  - `trait:todo .value == todo within(section includes(.title, "pricing"))`
- Path-scoped pages with open todos:
  - `type:page matches(.path, "^pages/work/") contains(trait:todo .value == todo)`
- Due tomorrow:
  - `trait:due .value == tomorrow`
- Meetings with an attendee:
  - `type:meeting .attendees == [[person/freya]]`
- Active projects:
  - `type:project .status == active`
- PDF links from projects:
  - `link .ext == pdf within(type:project)`

## Related topics

- `raven://guide/querying`
- `raven://guide/query-at-scale`
- `raven://guide/examples`
