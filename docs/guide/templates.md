# Templates Guide

Use templates when you want new notes to start with a consistent structure.

This guide covers both:
- type templates (for `rvn new <type> ...`)
- daily templates (for `rvn daily`)

Goal: set up templates safely with the `rvn template ...` command family.

Out of scope:
- full command-flag reference (use `reference/cli.md`)
- full schema reference (use `reference/schema.md`)

## How templates work

Templates in Raven are file-backed only.

- A type template is stored in `schema.yaml` as `types.<type>.template: <file-path>`.
- A daily template is stored in `raven.yaml` as `daily_template: <file-path>`.
- Template files must live under `directories.template` in `raven.yaml` (default: `templates/`).

Example config:

```yaml
directories:
  template: templates/
daily_template: templates/daily.md
```

## Quick start: type template

Use this flow when you want structure for a type like `meeting`.

### 1) Check the type exists

```bash
rvn schema type meeting
```

### 2) Scaffold and bind a template file

```bash
rvn template scaffold type meeting
```

Default file path: `templates/meeting.md`.

### 3) Write template content

```bash
rvn template write type meeting --content '# {{title}}

**Date:** {{date}}

## Agenda

## Notes

## Action Items
'
```

### 4) Preview variable substitution

```bash
rvn template render type meeting --title "Weekly Standup"
```

### 5) Smoke test object creation

```bash
rvn new meeting "Weekly Standup"
```

## Quick start: daily template

### 1) Scaffold and bind daily template

```bash
rvn template scaffold daily
```

Default file path: `templates/daily.md`.

### 2) Write daily template content

```bash
rvn template write daily --content '# {{weekday}}, {{date}}

## Morning

## Afternoon

## Evening
'
```

### 3) Preview for a specific date

```bash
rvn template render daily --date tomorrow
```

### 4) Create/open a daily note

```bash
rvn daily tomorrow
```

## Command patterns

- `rvn template list`  
  Show all configured template bindings.

- `rvn template get type <type_name>` / `rvn template get daily`  
  Show binding + resolved file content.

- `rvn template set ... --file <path>`  
  Bind an existing template file (must already exist).

- `rvn template scaffold ...`  
  Create a template file and bind it in one step.

- `rvn template write ... --content "..."`  
  Replace content in the currently bound file.

- `rvn template remove ...`  
  Remove binding; add `--delete-file` to also delete the file.

## Template variables

Supported variables:

| Variable | Meaning |
|----------|---------|
| `{{title}}` | object title (or date string for daily templates) |
| `{{slug}}` | slugified title |
| `{{type}}` | type name |
| `{{date}}` | date (`YYYY-MM-DD`) |
| `{{datetime}}` | datetime (`YYYY-MM-DDTHH:MM`) |
| `{{year}}` | year |
| `{{month}}` | month (`01`-`12`) |
| `{{day}}` | day (`01`-`31`) |
| `{{weekday}}` | weekday name |
| `{{field.<name>}}` | value passed with `--field` during object creation |

For daily templates, date-based variables are usually the most useful.

## Reusing and deleting template files safely

Multiple targets can point to the same file. For example, two types can share a common template.

If you run:

```bash
rvn template remove type meeting --delete-file
```

Raven checks whether other template bindings still reference that file. If they do, deletion is blocked unless you pass `--force`.

Recommended safe flow:
1. `rvn template list`
2. remove or repoint other bindings
3. rerun remove with `--delete-file`

## Troubleshooting

- `template file must be under directories.template ...`  
  Move the file under your configured template directory or update `directories.template`.

- `template file not found ...`  
  Create it first (`rvn template scaffold ...`) or fix the path.

- `inline template content is not supported`  
  Use file references in `schema.yaml` / `raven.yaml`; do not store inline template bodies in config.

## Related docs

- `reference/cli.md` for exact `rvn template` flags
- `reference/vault-config.md` for `directories.template` and `daily_template`
- `reference/schema.md` for `types.<type>.template`
- `configuration.md` for practical vault setup
