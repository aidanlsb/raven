# Raven Query Cheatsheet

Quick reference for common Raven Query Language (RQL) patterns.

## Query shapes

- `type:<type> [predicates...]`
- `trait:<name> [predicates...]`

## Predicates

- Field comparisons: `.field == value`, `.field != value`, `.field < value`, `.field >= value`
- Trait value: `.value == value` (for `trait:...` queries)
- Array membership:
  - `.field == value` (works for arrays)
  - `any(.field, _ == value)` (explicit)
- List membership: `in(.field, [a, b, c])`
- Text search: `content("phrase")`
- References:
  - `refs([[target]])` (objects/traits that reference target)
  - `refs(type:project .status==active)`
  - Trait location: `within([[target]])`, `within(type:...)`, `on([[target]])`, `on(type:...)`
  - Object hierarchy: `parent(...)`, `ancestor(...)`, `child(...)`, `descendant(...)`

## Sub-queries

Nest queries inside predicates to filter by related objects or traits:

- `refs(type:project .status == active)`
- `within(type:meeting refs([[project/raven]]))`
- `has(trait:due .value < today)`
- `on(type:project .status == active)`

## Boolean logic

Combine predicates with boolean operators:

- Space between predicates = AND
- `|` = OR (use parentheses for grouping)
- `!` = NOT (prefix)
- `(...)` = grouping

Example:
- `trait:todo (.value == todo | .value == doing) !refs([[project/legacy]])`

This can be very useful to provide lots of information to the user. If a question is vague, err on the side of running a few different versions of a query that could match the description and returning all the results to the user.

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
  - `trait:todo .value == todo within(type:section content("pricing"))`
- Path-scoped pages with open todos:
  - `type:page matches(.path, "^pages/work/") has(trait:todo .value == todo)`
- Due tomorrow:
  - `trait:due .value == tomorrow`
- Meetings with an attendee:
  - `type:meeting .attendees == [[person/freya]]`
- Active projects:
  - `type:project .status == active`
