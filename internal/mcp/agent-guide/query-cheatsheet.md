# Raven Query Cheatsheet

Quick reference for common Raven Query Language (RQL) patterns.

## Query shapes

- `object:<type> [predicates...]`
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
  - `refs(object:project .status==active)`
  - Trait location: `within([[target]])`, `within(object:...)`, `on([[target]])`, `on(object:...)`
  - Object hierarchy: `parent(...)`, `ancestor(...)`, `child(...)`, `descendant(...)`

## Sub-queries

Nest queries inside predicates to filter by related objects or traits:

- `refs(object:project .status == active)`
- `within(object:meeting refs([[projects/raven]]))`
- `has(trait:due .value < today)`
- `on(object:project .status == active)`

## Boolean logic

Combine predicates with boolean operators:

- Space between predicates = AND
- `|` = OR (use parentheses for grouping)
- `!` = NOT (prefix)
- `(...)` = grouping

Example:
- `trait:todo (.value == todo | .value == doing) !refs([[projects/legacy]])`

This can be very useful to provide lots of information to the user. If a question is vague, err on the side of running a few different versions of a query that could match the description and returning all the results to the user.

## Examples

- Open todos for a project page:
  - `trait:todo within([[projects/raven]]) .value != done`
- Todos referencing a project:
  - `trait:todo refs([[projects/raven]]) .value != done`
- Due tomorrow:
  - `trait:due .value == tomorrow`
- Meetings with an attendee:
  - `object:meeting .attendees == [[people/freya]]`
- Active projects:
  - `object:project .status == active`
