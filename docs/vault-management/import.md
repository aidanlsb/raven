# Import

`rvn import` loads objects from JSON into your vault. Use it to migrate from another tool, sync from an external source, or create many objects from structured data.

## Quick start

Pipe a JSON array into `rvn import` with a target type:

```bash
echo '[{"name": "Freya", "email": "freya@example.com"}, {"name": "Thor"}]' | rvn import person --dry-run
```

The `--dry-run` flag previews what would be created. Remove it to apply immediately:

```bash
echo '[{"name": "Freya", "email": "freya@example.com"}, {"name": "Thor"}]' | rvn import person
```

Read from a file instead of stdin:

```bash
rvn import person --file contacts.json
```

## Field mapping

When external field names don't match your schema, use `--map` to translate them:

```bash
echo '[{"full_name": "Freya", "mail": "freya@example.com"}]' \
  | rvn import person --map full_name=name --map mail=email
```

Each `--map` flag maps one external key to a schema field: `external_key=schema_field`.

## Mapping files

For complex or reusable mappings, use a YAML mapping file:

```yaml
# mappings/contacts.yaml
type: person
map:
  full_name: name
  mail: email
  org: company
```

```bash
rvn import --mapping mappings/contacts.yaml --file contacts.json
```

Mapping files use `map` for external-to-schema field mappings. They can also set
`key` and `content_field`, matching the equivalent CLI flags.

### Heterogeneous imports

When the input contains multiple types, use a `type_field` to tell Raven which field determines the type:

```yaml
# mappings/mixed.yaml
type_field: kind
types:
  contact:
    type: person
    map:
      full_name: name
  proj:
    type: project
    map:
      title: name
```

```bash
rvn import --mapping mappings/mixed.yaml --file mixed-data.json
```

Each entry under `types` uses `type` for the Raven target type and can set its
own `map`, `key`, and `content_field`.

## Import modes

By default, `rvn import` upserts: it creates new objects and updates existing ones (matched by the type's `name_field`).

| Flag | Behavior |
|------|----------|
| *(default)* | Upsert: create or update |
| `--create-only` | Only create new objects, skip existing |
| `--update-only` | Only update existing objects, skip new |

Change the match key with `--key`:

```bash
rvn import person --file contacts.json --key email
```

This matches existing objects by their `email` field instead of the default `name_field`.

## Content field

Use `--content-field` to populate the markdown body from a JSON field:

```bash
echo '[{"name": "Freya", "bio": "Project lead and architect."}]' \
  | rvn import person --content-field bio
```

The `bio` value becomes the page body content below the frontmatter.

## Protected and excluded paths

Like every Raven write command, `rvn import` refuses to create or modify objects
in protected prefixes (for example `templates/`, `.raven/`, or any
`protected_prefixes` you configure) or in excluded paths. Any item that resolves
to such a path is reported as a per-item error, and the rest of the import
continues.

## Preview and apply

Imports apply immediately unless you pass `--dry-run`:

```bash
# Preview
rvn import person --file contacts.json --dry-run

# Apply changes immediately
rvn import person --file contacts.json
```

The dry-run preview shows which objects would be created, updated, or skipped.

## Examples

### Import contacts from a CRM export

```bash
rvn import person \
  --file crm-export.json \
  --map full_name=name \
  --map primary_email=email \
  --map organization=company
```

### Import tasks from a project management tool

```bash
rvn import note \
  --file tasks.json \
  --map summary=title \
  --map description=content \
  --content-field description \
  --create-only
```

### Dry-run to check mappings

Always preview first when working with unfamiliar data:

```bash
rvn import person --file unknown-format.json --dry-run
```

Review the output, add `--map` flags as needed, and then rerun without `--dry-run` to apply.

## Related docs

- [Bulk operations](bulk-operations.md): query-driven changes with `--apply`
  and `--ids`.
- [Common commands](../using-your-vault/common-commands.md): `rvn upsert`,
  `rvn set`, and other editing commands.
- [Schema reference](../types-and-traits/schema.md): field types and validation.
- [Documentation map](../getting-started/documentation-map.md): every topic.
