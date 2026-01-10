# `raven.yaml` (reference)

`raven.yaml` controls vault behavior (as opposed to structure in `schema.yaml`).

## Top-level shape

```yaml
daily_directory: daily
daily_template: templates/daily.md  # optional (path or inline content)

auto_reindex: true

directories:
  objects: objects/
  pages: pages/

capture:
  destination: daily      # or a file path like inbox.md
  heading: "## Captured"  # optional
  timestamp: false

deletion:
  behavior: trash         # trash | permanent
  trash_dir: .trash

queries:
  overdue:
    query: "trait:due value:past"
    description: "Overdue items"

workflows:
  meeting-prep:
    file: workflows/meeting-prep.yaml
```

## Saved queries

Saved queries are just named Raven Query Language strings:

```yaml
queries:
  active-projects:
    query: "object:project .status:active"
    description: "Active projects"
```

Run with:

```bash
rvn query active-projects
```

## Workflows

Workflows live under `workflows:` and can be inline or `file:` based.

Reference: `reference/workflows.md`.

## Daily note templates

`daily_template` supports the same basic variables as type templates:
- `{{date}}`, `{{weekday}}`, `{{year}}`, `{{month}}`, `{{day}}`

