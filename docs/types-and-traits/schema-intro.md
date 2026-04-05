# Schema Introduction

This is a guide-level introduction to `schema.yaml`.

Goal: make your first safe schema changes without getting lost in full reference details.

Out of scope:
- exhaustive field/trait rules (use `types-and-traits/schema.md`)

## What `schema.yaml` controls

`schema.yaml` defines your vault's data model:
- **types**: what objects are (for example `project`, `person`, `book`)
- **fields**: validated frontmatter keys per type
- **traits**: inline annotations like `@due(2026-02-01)` or `@highlight`

When Raven indexes your notes, schema definitions determine what becomes structured, queryable data. Only types, fields, and traits defined in `schema.yaml` are indexed — undefined frontmatter keys trigger validation warnings, and undefined traits are treated as plain text.

This means the schema is the bridge between your markdown files and Raven's query engine. If something isn't in the schema, you can't query it structurally.

## Validation levels

Raven validates your schema and data at two levels:

| Command | What it checks |
|---------|---------------|
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

Good descriptions focus on intent and constraints, not just restating the field name. Agents use these descriptions to understand your domain model when creating or querying objects.

## Start from the default schema

After `rvn init`, your schema already includes:
- built-in types (`page`, `section`, `date`)
- starter types (`person`, `project`)
- starter traits (`due`, `todo`, `priority`)

Read your current schema first:

```bash
rvn schema types
rvn schema traits
rvn schema type project
```

## First safe customization (recommended)

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

- **Making fields required too early**  
  Start optional, backfill data, then make required.

- **Using a trait that is not defined**  
  Add it in schema first, or it will not be indexed/queryable.

- **Assuming references auto-resolve to any text**  
  Use explicit object IDs when learning (`[[book/the-mythical-man-month]]`).

## What to read next

- `types-and-traits/schema.md` for complete schema format and evolution rules
- `types-and-traits/templates.md` for end-to-end template file + schema lifecycle
- Use `rvn help <command>` for the full `rvn schema ...` command set
- `using-your-vault/configuration.md` for `config.toml` and `raven.yaml` setup
