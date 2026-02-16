# Schema Introduction

This is a guide-level introduction to `schema.yaml`.

Goal: make your first safe schema changes without getting lost in full reference details.

Out of scope:
- exhaustive field/trait rules (use `reference/schema.md`)
- every schema command variant (use `reference/cli.md`)

## What `schema.yaml` controls

`schema.yaml` defines your vault's data model:
- **types**: what objects are (for example `project`, `person`, `book`)
- **fields**: validated frontmatter keys per type
- **traits**: inline annotations like `@due(2026-02-01)` or `@highlight`

When Raven indexes your notes, schema definitions determine what becomes structured/queryable data.

## Start from the default schema

After `rvn init`, your schema already includes:
- built-in types (`page`, `section`, `date`)
- starter types (`person`, `project`)
- starter traits (including `due`, `priority`, `status`, `highlight`)

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
rvn schema add type book --name-field title --default-path books/
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
rvn add "@toread Read chapter 1 of [[books/the-mythical-man-month]]"
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
  Use explicit object IDs when learning (`[[books/the-mythical-man-month]]`).

## What to read next

- `reference/schema.md` for complete schema format and evolution rules
- `reference/cli.md` for the full `rvn schema ...` command set
- `configuration.md` for `config.toml` and `raven.yaml` setup

