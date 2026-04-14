# Query Language Quick Reference

## Query shapes

- Object query: `type:<type> [predicates...]`
- Trait query: `trait:<name> [predicates...]`

Examples:

```text
type:project .status==active
trait:due .value<today
```

## Core predicate families

- Field/value predicates:
  - Equality and inequality: `.field==value`, `.field!=value`
  - Comparisons: `.field<value`, `.field<=value`, `.field>value`, `.field>=value`
  - Presence: `exists(.field)`, `!exists(.field)`
  - Scalar membership: `in(.field, [a,b,c])`
- String helpers:
  - `contains(.field, "text")`
  - `startswith(.field, "prefix")`
  - `endswith(.field, "suffix")`
  - `matches(.field, "pattern")` or `matches(.field, /pattern/)`
- Array quantifiers:
  - `any(.tags, _ == "urgent")`
  - `all(.tags, startswith(_, "feature-"))`
  - `none(.tags, _ == "deprecated")`
- Structural predicates (type query):
  - `has(trait:...)`, `encloses(trait:...)`
  - `parent(...)`, `ancestor(...)`, `child(...)`, `descendant(...)`
  - `refs(...)`, `refd(...)`
  - `content("term")`
- Structural predicates (trait query):
  - `on(...)`, `within(...)`, `at(trait:...)`
  - `refs(...)`
  - `content("term")`

## Boolean composition

- `!pred` (NOT), highest precedence
- `pred1 pred2` (AND), middle precedence
- `pred1 | pred2` (OR), lowest precedence
- Use parentheses to force grouping

Example:

```text
type:project (.status==active | .status==backlog) !.archived==true
```

## Relative date values

- Supported date keywords in comparisons: `today`, `tomorrow`, `yesterday`

Example:

```text
trait:due .value<=today
```

## Saved query inputs

- Declare placeholders in query: `{{args.name}}`
- Declare matching `args:` in `raven.yaml` query definition.
- Invoke by position or `key=value` inputs.

## Apply support by query kind

- Object queries support `--apply "set ..."`, `add`, `delete`, `move`.
- Trait queries support only `--apply "update <new_value>"`.
- All apply flows preview first; add `--confirm` to execute.
