# `schema.yaml` (reference)

`schema.yaml` defines:
- **types** (frontmatter objects and embedded objects)
- **traits** (inline `@trait` annotations)

## Top-level shape

```yaml
version: 2

types:
  person:
    default_path: people/
    fields:
      name: { type: string, required: true }
      email: { type: string }
      alias: { type: string }   # optional, enables [[alias]] resolution

traits:
  due:
    type: date
  priority:
    type: enum
    values: [low, medium, high]
    default: medium
  highlight:
    type: boolean
```

## Types

Type definition keys:
- `default_path`: where `rvn new <type> ...` creates files
- `fields`: field definitions for frontmatter (and for `::type(...)` embedded declarations)
- `template`: optional template (file path or inline template content)

### Field types

- `string`
- `number`
- `date` (YYYY-MM-DD)
- `datetime` (RFC3339-ish, e.g. `2026-01-10T09:00`)
- `enum` / `enum[]`
- `bool` / `bool[]`
- `string[]`
- `date[]`
- `ref` / `ref[]` (with `target: <type>`)

### Field properties

Common:
- `required: true|false`
- `default: <value>`

For enums:
- `values: [...]`

For refs:
- `target: <type>`

For numbers:
- `min`, `max`

## Traits

Traits are single-valued annotations in content:
- `@name` for booleans
- `@name(value)` for valued traits

Trait types:
- `string`
- `date`
- `datetime`
- `enum`
- `bool` / `boolean`

## Templates

Types can specify `template:`. Template variables:
- `{{title}}`, `{{slug}}`, `{{type}}`
- `{{date}}`, `{{datetime}}`, `{{year}}`, `{{month}}`, `{{day}}`, `{{weekday}}`
- `{{field.X}}` (values passed via `rvn new ... --field X=value`)

