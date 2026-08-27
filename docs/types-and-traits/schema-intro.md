# Schema introduction

This is a guide-level introduction to `schema.yaml`.

Goal: make your first safe schema changes without getting lost in the full reference.

Out of scope:
- exhaustive field/trait rules (use the [Schema reference](schema.md))

## What `schema.yaml` controls

`schema.yaml` defines your vault's data model:
- **types**: what objects are (for example `project`, `person`, `book`)
- **fields**: validated frontmatter keys per type
- **traits**: inline annotations like `@due(2026-02-01)` or `@highlight`

When Raven indexes your notes, only types, fields, and traits defined in `schema.yaml` become structured, queryable data. Unknown frontmatter keys are validation errors. Undefined traits are treated as plain text.

If a type, field, or trait is not in the schema, you cannot query it as structure.

Non-Markdown files are not schema objects. Link to them with standard Markdown;
Raven indexes those outgoing link edges separately.

## Validation levels

Raven validates schema and data at three points:

| Command | What it checks |
|---------|---------------|
| `rvn new` / mutation commands | Validate new values while writing |
| `rvn schema validate` | Internal consistency of `schema.yaml` (valid types, valid enum values, ref targets exist, etc.) |
| `rvn check` | Vault files against the schema (unknown types, missing required fields, broken references, undefined traits) |

Run `rvn schema validate` after editing the schema itself. Run `rvn check` to find data issues in your vault files.

## Descriptions for humans and agents

Add `description` to types and fields to give context to both humans and AI agents:

```yaml
types:
  experiment:
    description: Controlled product change with hypothesis and measured outcome
    fields:
      hypothesis:
        type: string
        description: Falsifiable statement of expected behavior change
```

Describe intent and constraints. Repeating the field name wastes the slot. Agents read these descriptions when creating or querying objects.

## Start from the default schema

After `rvn init`, your schema already includes:
- built-in types (`page`, `section`, `date`)
- starter types (`person`, `project`)
- starter traits (`due`, `todo`, `priority`)

Note: sections are queried with the `section` query keyword (`rvn query "section .title==Tasks"`), not `type:section`.

Read your current schema first:

```bash
rvn schema types
rvn schema traits
rvn schema type project
```

## First safe customization

Add one type and one trait before attempting bigger model changes.

### 1) Add a new type

```bash
rvn schema add type book --name-field title --default-path book/
rvn schema add field book author --type string
rvn schema add field book status --type enum --values planned,reading,done
```

### 2) Add a new trait

```bash
rvn schema add trait toread --type bool
```

### 3) Use the new model immediately

```bash
rvn new book "The Mythical Man-Month" --field author="Frederick P. Brooks Jr."
rvn add "@toread Read chapter 1 of [[book/the-mythical-man-month]]"
rvn query 'trait:toread'
```

Success check: `rvn query 'trait:toread'` returns at least one result.

## Safe schema-change loop

Use this loop each time you modify `schema.yaml`:
1. change schema via CLI or manual edit
2. run `rvn schema validate`
3. run `rvn check`
4. run `rvn reindex --full` after major schema changes

This catches type/field/trait issues early and keeps the index aligned with the schema.

## Common first mistakes

- **Making fields required too early.** Start optional, backfill data, then make required.
- **Using a trait that is not defined.** Add it in schema first, or it will not be indexed.
- **Assuming references auto-resolve to any text.** Use explicit object IDs when learning (`[[book/the-mythical-man-month]]`).

## What to read next

- [Schema reference](schema.md) for the complete schema format and evolution
  rules.
- [Templates](templates.md) for template files, definitions, and bindings.
- Use `rvn help <command>` for the full `rvn schema ...` command set
- [Configuration](../using-your-vault/configuration.md) for `config.toml` and
  `raven.yaml`.
- [Documentation map](../getting-started/documentation-map.md) for every topic.
