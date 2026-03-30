# Import

`rvn import` lets you bulk-load objects from external JSON data into your vault. Use it when migrating from another tool, syncing from an external source, or bulk-creating objects from structured data.

## Quick start

Pipe a JSON array into `rvn import` with a target type:

```bash
echo '[{"name": "Freya", "email": "freya@example.com"}, {"name": "Thor"}]' | rvn import person --dry-run
```

The `--dry-run` flag previews what would be created. Remove it and add `--confirm` to apply:

```bash
echo '[{"name": "Freya", "email": "freya@example.com"}, {"name": "Thor"}]' | rvn import person --confirm
```

Read from a file instead of stdin:

```bash
rvn import person --file contacts.json --confirm
```

## Field mapping

When external field names don't match your schema, use `--map` to translate them:

```bash
echo '[{"full_name": "Freya", "mail": "freya@example.com"}]' \
  | rvn import person --map full_name=name --map mail=email --confirm
```

Each `--map` flag maps one external key to a schema field: `external_key=schema_field`.

## Mapping files

For complex or reusable mappings, use a YAML mapping file:

```yaml
# mappings/contacts.yaml
type: person
mappings:
  full_name: name
  mail: email
  org: company
```

```bash
rvn import --mapping mappings/contacts.yaml --file contacts.json --confirm
```

### Heterogeneous imports

When the input contains multiple types, use a `type_field` to tell Raven which field determines the type:

```yaml
# mappings/mixed.yaml
type_field: kind
types:
  contact:
    target_type: person
    mappings:
      full_name: name
  proj:
    target_type: project
    mappings:
      title: name
```

```bash
rvn import --mapping mappings/mixed.yaml --file mixed-data.json --confirm
```

## Import modes

By default, `rvn import` upserts: it creates new objects and updates existing ones (matched by the type's `name_field`).

| Flag | Behavior |
|------|----------|
| *(default)* | Upsert — create or update |
| `--create-only` | Only create new objects, skip existing |
| `--update-only` | Only update existing objects, skip new |

Change the match key with `--key`:

```bash
rvn import person --file contacts.json --key email --confirm
```

This matches existing objects by their `email` field instead of the default `name_field`.

## Content field

Use `--content-field` to populate the markdown body from a JSON field:

```bash
echo '[{"name": "Freya", "bio": "Project lead and architect."}]' \
  | rvn import person --content-field bio --confirm
```

The `bio` value becomes the page body content below the frontmatter.

## Preview and apply

All imports preview by default. Use `--dry-run` for an explicit preview, and `--confirm` to apply:

```bash
# Preview (default behavior)
rvn import person --file contacts.json

# Explicit preview
rvn import person --file contacts.json --dry-run

# Apply changes
rvn import person --file contacts.json --confirm
```

The preview shows which objects would be created, updated, or skipped.

## Examples

### Import contacts from a CRM export

```bash
rvn import person \
  --file crm-export.json \
  --map full_name=name \
  --map primary_email=email \
  --map organization=company \
  --confirm
```

### Import tasks from a project management tool

```bash
rvn import note \
  --file tasks.json \
  --map summary=title \
  --map description=content \
  --content-field description \
  --create-only \
  --confirm
```

### Dry-run to check mappings

Always preview first when working with unfamiliar data:

```bash
rvn import person --file unknown-format.json --dry-run
```

Review the output, add `--map` flags as needed, and re-run until the preview looks right.

## Related docs

- `vault-management/bulk-operations.md` — query-driven bulk changes with `--apply` and `--ids`
- `using-your-vault/common-commands.md` — `rvn upsert`, `rvn set`, and other editing commands
- `types-and-traits/schema.md` — field types and validation rules
