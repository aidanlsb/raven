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
  - `within([[target]])` (traits/objects within a target object)
  - `within(object:meeting .status==active)`
  - `on([[target]])`, `on(object:...)`, `ancestor(...)`, `child(...)`

## Sub-queries

Nest queries inside predicates to filter by related objects or traits:

- `refs(object:project .status == active)`
- `within(object:meeting refs([[project/raven]]))`
- `has(trait:due .value == past)`
- `on(object:project .status == active)`

## Examples

- Open todos for a project page:
  - `trait:todo within([[project/raven]]) .value != done`
- Todos referencing a project:
  - `trait:todo refs([[project/raven]]) .value != done`
- Due tomorrow:
  - `trait:due .value == tomorrow`
- Meetings with an attendee:
  - `object:meeting .attendees == [[people/freya]]`
- Active projects:
  - `object:project .status == active`
