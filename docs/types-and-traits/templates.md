# Templates Guide

Use templates when you want new notes to start with consistent, reusable content.

This guide covers:
- schema templates (shared file definitions in `schema.yaml`)
- type template bindings (which template IDs a type can use)
- daily templates through the built-in `date` type

Goal: set up templates with the `rvn schema template ...` and `rvn schema type <type> template ...` commands.

Out of scope:
- full command/flag reference (see `reference/cli.md`)
- full schema format reference (see `types-and-traits/schema.md`)

## How templates work

Templates are file-backed only.

- Template definitions live in `schema.yaml` under top-level `templates:`.
- Types opt into template IDs via `types.<type>.templates`.
- A type can set `default_template`; if unset, creation proceeds without a template.
- Daily notes use type `date`, so daily templates are configured via `types.date.templates` and `types.date.default_template`.
- Template files must live under `directories.template` in `raven.yaml` (default: `templates/`).

Example schema:

```yaml
templates:
  meeting_standard:
    file: templates/meeting/standard.md

types:
  meeting:
    templates: [meeting_standard]
    default_template: meeting_standard

  date:
    templates: [daily_default]
    default_template: daily_default
```

## Quick start: type template

Use this flow when you want structure for a type like `meeting`.

### 1) Confirm the type exists

```bash
rvn schema type meeting
```

### 2) Create a template file

```bash
mkdir -p templates/meeting
cat > templates/meeting/standard.md <<'EOF'
# Meeting Notes

## Agenda

## Notes

## Action Items
EOF
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

## Quick start: daily template (`date` type)

### 1) Create a daily template file

```bash
cat > templates/daily.md <<'EOF'
# Daily Note

## Morning

## Afternoon

## Evening
EOF
```

### 2) Register and bind it to `date`

```bash
rvn schema template set daily_default --file templates/daily.md
rvn schema type date template set daily_default
rvn schema type date template default daily_default
```

### 3) Create/open a daily note

```bash
rvn daily tomorrow
```

## Command patterns

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

## Important behavior

- If a type has no `default_template`, `rvn new` creates the object without template content.
- `--template <template_id>` on `rvn new` can override the default for that create call.
- Template content is static file content at creation time.

## Troubleshooting

- `template file must be under directories.template ...`
  Move the file under your configured template directory or update `directories.template`.
- `template file not found ...`
  Create the file first, then run `rvn schema template set ... --file ...`.
- `template '<id>' is still referenced by ...`
  Unbind from types first using `rvn schema type <type> template remove <id>`.

## Related docs

- `reference/cli.md` for exact `schema template` and `schema type ... template` command details
- `types-and-traits/schema.md` for `templates`, `types.<type>.templates`, and `default_template`
- `reference/vault-config.md` for `directories.template`
- `getting-started/configuration.md` for practical vault setup
