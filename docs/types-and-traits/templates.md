# Templates Guide

Use templates when you want new notes to start with consistent, reusable content.

This guide covers:
- template file lifecycle (create/update/list/delete)
- schema templates (shared file definitions in `schema.yaml`)
- type template bindings (which template IDs a type can use)
- core-type templates for built-ins like `date`

Goal: set up complete template workflows with both:
- `rvn template ...` for file lifecycle
- `rvn schema ... template ...` for schema bindings/defaults

Out of scope:
- full command/flag reference (see `reference/cli.md`)
- full schema format reference (see `types-and-traits/schema.md`)

## How templates work

Templates are file-backed only.

- Template files live under `directories.template` in `raven.yaml` (default: `templates/`).
- `rvn template write` creates or updates template files.
- `rvn template delete` moves template files to `.trash/` (and blocks by default if schema definitions still reference the file).
- Template definitions live in `schema.yaml` under top-level `templates:`.
- Types opt into template IDs via `types.<type>.templates`.
- A type can set `default_template`; if unset, creation proceeds without a template.
- Built-in core types (for example `date`) are configured with `rvn schema core ... template ...`.

Example schema:

```yaml
templates:
  meeting_standard:
    file: templates/meeting/standard.md

types:
  meeting:
    templates: [meeting_standard]
    default_template: meeting_standard
```

Core-type template bindings (for built-ins like `date`) are managed via
`rvn schema core ... template ...` commands rather than under a user-defined
`types.<name>` block.

## Quick start: type template

Use this flow when you want structure for a type like `meeting`.

### 1) Confirm the type exists

```bash
rvn schema type meeting
```

### 2) Create/update a template file

```bash
rvn template write meeting/standard.md --content "# Meeting Notes

## Agenda

## Notes

## Action Items"
```

### 3) Register the schema template

```bash
rvn schema template set meeting_standard --file templates/meeting/standard.md
```

### 4) Bind it to the type and set default

```bash
rvn schema type meeting template set meeting_standard
rvn schema type meeting template default meeting_standard
```

### 5) Smoke test object creation

```bash
rvn new meeting "Weekly Standup"
```

## Quick start: daily template (`date` core type)

### 1) Create/update a daily template file

```bash
rvn template write daily.md --content "# Daily Note

## Morning

## Afternoon

## Evening"
```

### 2) Register and bind it to core type `date`

```bash
rvn schema template set daily_default --file templates/daily.md
rvn schema core date template set daily_default
rvn schema core date template default daily_default
```

### 3) Create/open a daily note

```bash
rvn daily tomorrow
```

## Command patterns: file lifecycle

- `rvn template list`
  List template files under `directories.template`.
- `rvn template write <path> --content "<markdown>"`
  Create or update a template file (full file replacement).
- `rvn template delete <path>`
  Delete a template file (moves to `.trash/`; blocked if schema templates still reference it).
- `rvn template delete <path> --force`
  Force delete even if schema template definitions still reference the file.

## Command patterns: schema lifecycle

- `rvn schema template list`
  List all schema templates.
- `rvn schema template get <template_id>`
  Show one template definition.
- `rvn schema template set <template_id> --file <path>`
  Create or update a template definition.
- `rvn schema template remove <template_id>`
  Remove a template definition (blocked if still bound to any type).
- `rvn schema type <type_name> template list`
  List template IDs bound to a type.
- `rvn schema type <type_name> template set <template_id>`
  Bind a template ID to a type.
- `rvn schema type <type_name> template remove <template_id>`
  Unbind a template ID from a type.
- `rvn schema type <type_name> template default <template_id>`
  Set default template for a type.
- `rvn schema type <type_name> template default --clear`
  Clear default template for a type.
- `rvn schema core <core_type> template list|set|remove|default`
  Manage template bindings/default for built-in core types (for example `date`).

## Important behavior

- If a type has no `default_template`, `rvn new` creates the object without template content.
- `--template <template_id>` on `rvn new` can override the default for that create call.
- Template content is static file content at creation time.
- Template file lifecycle and schema binding lifecycle are intentionally separate.

## Troubleshooting

- `template file must be under directories.template ...`
  Move the file under your configured template directory or update `directories.template`.
- `template file not found ...`
  Create the file with `rvn template write ...`, then run `rvn schema template set ... --file ...`.
- `template '<id>' is still referenced by ...`
  Unbind from types first using `rvn schema type <type> template remove <id>`.
- `template file "<path>" is referenced by schema templates: ...`
  Remove schema template definitions first (`rvn schema template remove <id>`) or use `rvn template delete ... --force`.

## Related docs

- `reference/cli.md` for exact `schema template` and `schema type ... template` command details
- `types-and-traits/schema.md` for `templates`, `types.<type>.templates`, and `default_template`
- `reference/vault-config.md` for `directories.template`
- `getting-started/configuration.md` for practical vault setup
