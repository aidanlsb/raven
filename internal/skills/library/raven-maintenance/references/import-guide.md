# Import Guide

## Simple import (single type, no mapping)

```bash
echo '[{"name": "Freya", "email": "freya@asgard.realm"}]' | rvn import person --json
```

Fields in the JSON that match schema field names are mapped automatically.

## Field mapping with --map

```bash
rvn import person --file contacts.json --map full_name=name --map mail=email --json
```

Maps `full_name` in the JSON to the `name` schema field, `mail` to `email`.

## YAML mapping file (homogeneous)

```yaml
type: person
key: name
map:
  full_name: name
  mail: email
```

```bash
rvn import --mapping contacts.yaml --file contacts.json --dry-run --json
```

The `key` field determines how existing objects are matched for upsert behavior.

## YAML mapping file (heterogeneous / mixed types)

```yaml
type_field: kind
types:
  contact:
    type: person
    key: name
    map:
      full_name: name
  task:
    type: project
    map:
      title: name
```

Each input record's `kind` field determines which type mapping applies.

## Import modes

- Default: upsert (create new, update existing)
- `--create-only`: skip records that match existing objects
- `--update-only`: skip records that don't match existing objects

## Recommended flow

```bash
# 1. Dry run to see what would happen
rvn import person --file contacts.json --dry-run --json

# 2. Review the preview output

# 3. Apply
rvn import person --file contacts.json --confirm --json

# 4. Verify
rvn check --type person --json
```
