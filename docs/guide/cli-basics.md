# CLI Basics

Use this guide for first-day Raven CLI usage.

Goal: work comfortably with everyday commands without learning advanced workflows yet.

Out of scope:
- bulk operations (`--stdin`, `--apply`, `--confirm` pipelines)
- deep schema changes
- workflow automation

## Daily loop

### Open today's note

```bash
rvn daily
```

### Capture quickly

```bash
rvn add "Idea to revisit tomorrow"
rvn add "@highlight Useful insight from standup"
```

### Open a specific object or note

```bash
rvn open projects/onboarding
rvn open 2026-02-15
```

## Create structured content

### Create typed objects

```bash
rvn new person "Freya"
rvn new project "Onboarding"
```

If a type has a `name_field`, the title auto-populates it.

### Update frontmatter fields

```bash
rvn set projects/onboarding status=active
rvn set people/freya email=freya@example.com
```

### Add structured lines to notes

```bash
rvn add "Discussed [[projects/onboarding]] with [[people/freya]]"
rvn add "@status(todo) Draft onboarding checklist"
```

## Query basics

### Query objects

```bash
rvn query 'object:project'
rvn query 'object:project .status==active'
```

### Query traits

```bash
rvn query 'trait:highlight'
rvn query 'trait:status .value==todo'
```

### Query with references

```bash
rvn query 'trait:highlight refs([[projects/onboarding]])'
```

## Read and validate

```bash
rvn read projects/onboarding.md
rvn check
```

Use `rvn check` when things look wrong (missing refs, unknown fields, undefined traits).

## Shell quoting

Always wrap queries in single quotes so your shell does not reinterpret special characters:

```bash
# Good
rvn query 'trait:status refs([[projects/onboarding]])'

# Risky
rvn query trait:status refs([[projects/onboarding]])
```

Single quotes protect characters like `(`, `)`, `|`, and `!`.

## What to use next

- `templates.md` for type/daily template setup and lifecycle
- `cli-advanced.md` for bulk changes, schema ops, and automation patterns
- `reference/cli.md` for exact command flags and complete syntax

