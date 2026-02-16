# `raven.yaml` Reference

`raven.yaml` controls vault behavior (as opposed to structure in `schema.yaml`). It lives at the root of your vault.
If you are deciding *what* to configure first, start with `guide/configuration.md` and return here for full option details.
This page is lookup-oriented and is not required for the first-session note -> structure -> query loop.

## Complete Example

```yaml
# Where daily notes are stored
daily_directory: daily

# Template for daily notes (path or inline content)
daily_template: templates/daily.md

# Auto-reindex after CLI operations (default: true)
auto_reindex: true

# Directory organization (optional)
directories:
  object: object/        # Root for typed objects
  page: page/            # Root for untyped pages
  workflow: workflows/   # Root for workflow definition files

# Quick capture settings
capture:
  destination: daily      # "daily" or file path like "inbox.md"
  heading: "## Captured"  # Optional heading to append under
  timestamp: false        # Prefix with HH:MM

# Deletion settings
deletion:
  behavior: trash         # "trash" or "permanent"
  trash_dir: .trash       # Where trashed files go

# Saved queries
queries:
  overdue:
    query: "trait:due .value==past"
    description: "Overdue items"
  active-projects:
    query: "object:project .status==active"
    description: "Active projects"

# Workflows registry (file references only)
workflows:
  meeting-prep:
    file: workflows/meeting-prep.yaml
  research:
    file: workflows/research.yaml

  # Workflow run checkpoint retention
  runs:
    storage_path: .raven/workflow-runs
    auto_prune: true
    keep_completed_for_days: 7
    keep_failed_for_days: 14
    keep_awaiting_for_days: 30
    max_runs: 1000
    preserve_latest_per_workflow: 5

# Additional protected/system prefixes (additive).
# Critical protected paths are enforced automatically (.raven/, .trash/, .git/, raven.yaml, schema.yaml).
# protected_prefixes:
#   - templates/
#   - private/
```

---

## Configuration Options

### `daily_directory`

Directory where daily notes are stored.

| Type | Default |
|------|---------|
| string | `"daily"` |

```yaml
daily_directory: journal
```

Daily notes are created as `<daily_directory>/YYYY-MM-DD.md`.

---

### `daily_template`

Template for new daily notes. Can be a file path or inline content.

| Type | Default |
|------|---------|
| string | (none) |

**File-based template:**

```yaml
daily_template: templates/daily.md
```

**Inline template:**

```yaml
daily_template: |
  # {{weekday}}, {{date}}
  
  ## Morning
  
  ## Afternoon
  
  ## Evening
```

**Available variables:**

| Variable | Example |
|----------|---------|
| `{{date}}` | `2026-01-10` |
| `{{weekday}}` | `Friday` |
| `{{year}}` | `2026` |
| `{{month}}` | `01` |
| `{{day}}` | `10` |

---

### `auto_reindex`

Automatically reindex after CLI operations that modify files.

| Type | Default |
|------|---------|
| boolean | `true` |

```yaml
auto_reindex: true
```

When enabled, commands like `rvn add`, `rvn new`, `rvn set`, and `rvn edit` automatically update the index. Disable if you prefer manual reindexing with `rvn reindex`.

---

### `directories`

Configure directory organization for the vault. When set, typed objects are nested under one root, untyped pages under another.

Uses singular keys (`object`, `page`) to encourage singular directory names, which leads to more natural reference syntax like `[[person/freya]]` instead of `[[people/freya]]`.

| Property | Type | Description |
|----------|------|-------------|
| `object` | string | Root directory for typed objects |
| `page` | string | Root directory for untyped pages |

```yaml
directories:
  object: object/
  page: page/
```

**How it works:**

- Type `default_path` values are relative to `object`
- Object IDs strip the directory prefix for shorter references
- Example: `object/person/freya.md` → ID is `person/freya`

**Without directories configured:**

```
vault/
├── person/
│   └── freya.md          # ID: person/freya
├── project/
│   └── website.md        # ID: project/website
└── random-note.md        # ID: random-note
```

**With directories configured:**

```
vault/
├── object/
│   ├── person/
│   │   └── freya.md      # ID: person/freya
│   └── project/
│       └── website.md    # ID: project/website
└── page/
    └── random-note.md    # ID: random-note
```

**Backwards compatibility:** The old plural keys (`objects`, `pages`) are still supported but deprecated. New vaults should use singular keys.

---

### `capture`

Configure quick capture behavior for `rvn add`.

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `destination` | string | `"daily"` | Where to append captures |
| `heading` | string | (none) | Heading to append under |
| `timestamp` | boolean | `false` | Prefix with current time |

```yaml
capture:
  destination: daily
  heading: "## Captured"
  timestamp: true
```

**Destination options:**

- `"daily"` — Append to today's daily note
- A file path like `"inbox.md"` — Append to that specific file

**Heading behavior:**

- If specified, captures are appended under this heading
- The heading is created at the end of the file if it doesn't exist
- If not specified, captures are appended to the end of the file

**Timestamp format:**

When enabled, entries are prefixed with `HH:MM`:

```markdown
- 14:30 Quick thought
- 15:45 Another note
```

---

### `deletion`

Configure file deletion behavior for `rvn delete`.

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `behavior` | string | `"trash"` | `"trash"` or `"permanent"` |
| `trash_dir` | string | `".trash"` | Directory for trashed files |

```yaml
deletion:
  behavior: trash
  trash_dir: .trash
```

**Behavior options:**

- `"trash"` — Move deleted files to the trash directory (recoverable)
- `"permanent"` — Delete files permanently (use with caution)

**Trash directory:**

- Created within the vault when first used
- Typically gitignored
- Files can be recovered by moving them back

---

### `queries`

Define saved queries that can be run with `rvn query <name>`.

| Property | Type | Description |
|----------|------|-------------|
| `query` | string | Query string using Raven query language |
| `description` | string | Human-readable description |

```yaml
queries:
  overdue:
    query: "trait:due .value==past"
    description: "Items past their due date"

  active-projects:
    query: "object:project .status==active"
    description: "Projects with status active"

  reading-list:
    query: "trait:toread"
    description: "Books and articles to read"
```

Saved queries can accept inputs via `{{args.<name>}}` placeholders.
When using `{{args.*}}`, declare `args` explicitly to define accepted inputs and positional order:

```yaml
queries:
  project-todos:
    query: "trait:todo (within([[{{args.project}}]]) | refs([[{{args.project}}]]))"
    args: [project]
    description: "Todos tied to a project"
```

**Usage:**

```bash
rvn query overdue              # Run the saved query
rvn query project-todos projects/raven        # Positional (args order)
rvn query project-todos project=projects/raven # key=value (order independent)
rvn query --list               # List all saved queries
rvn query add new-query "..."  # Add via CLI
rvn query remove old-query     # Remove via CLI
```

See `reference/query-language.md` for query syntax.

---

### `directories.workflow`

Workflow file root under the `directories` config block.

| Type | Default |
|------|---------|
| string | `"workflows/"` |

```yaml
directories:
  workflow: workflows/
```

Workflow files referenced in `workflows.<name>.file` must live under this directory.
Set this when you want a different workflow root (for example `automation/workflows/`).

---

### `workflows`

Define reusable workflow declarations as a name -> file registry.

For agent-friendly creation (without manual YAML edits), use:
- `rvn workflow scaffold <name>`
- `rvn workflow add <name> --file <directories.workflow>/<name>.yaml`
- `rvn workflow validate [name]`

Declarations are file references only:

```yaml
directories:
  workflow: workflows/
workflows:
  meeting-prep:
    file: workflows/meeting-prep.yaml
```

Inline workflow bodies in `raven.yaml` are not supported.

Migration note:
- Move each inline workflow body into its own YAML file under `directories.workflow`
- Keep only `workflows.<name>.file` entries in `raven.yaml`

Legacy top-level workflow keys (`context`, `prompt`, `outputs`) are not supported in workflow v3.

See `reference/workflows.md` for complete workflow documentation.

---

### `workflows.runs`

Configure persisted workflow run checkpoints and retention.

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `storage_path` | string | `.raven/workflow-runs` | Vault-relative directory for run records |
| `auto_prune` | boolean | `true` | Prune records on `workflow run` / `workflow continue` |
| `keep_completed_for_days` | integer | `7` | TTL for completed runs |
| `keep_failed_for_days` | integer | `14` | TTL for failed runs |
| `keep_awaiting_for_days` | integer | `30` | TTL for awaiting-agent runs |
| `max_runs` | integer | `1000` | Hard cap on total stored runs |
| `preserve_latest_per_workflow` | integer | `5` | Keep newest N per workflow when pruning for cap |

```yaml
workflows:
  runs:
    storage_path: .raven/workflow-runs
    auto_prune: true
    keep_completed_for_days: 7
    keep_failed_for_days: 14
    keep_awaiting_for_days: 30
    max_runs: 1000
    preserve_latest_per_workflow: 5
```

---

## Default Configuration

When you run `rvn init`, a default `raven.yaml` is created:

```yaml
# Raven Vault Configuration

# Where daily notes are stored
daily_directory: daily

# Auto-reindex after CLI operations (default: true)
auto_reindex: true

# Saved queries - run with 'rvn query <name>'
queries:
  tasks:
    query: "trait:due"
    description: "All tasks with due dates"

  overdue:
    query: "trait:due .value==past"
    description: "Items past their due date"

  this-week:
    query: "trait:due .value==this-week"
    description: "Items due this week"

  active-projects:
    query: "object:project .status==active"
    description: "Projects with status active"

# Optional directories
# directories:
#   workflow: workflows/

# Workflows registry
workflows:
  onboard:
    file: workflows/onboard.yaml
```

---

## Configuration Precedence

Raven uses multiple configuration sources:

1. **`~/.config/raven/config.toml`** — Global settings (default vault, editor)
2. **`raven.yaml`** — Vault-specific settings (this file)
3. **Command-line flags** — Override for single commands

Global config (`config.toml`) is for cross-vault settings:

```toml
default_vault = "work"
editor = "code"
editor_mode = "auto"

[vaults]
work = "/path/to/work-notes"
personal = "/path/to/personal-notes"
```

`editor_mode` controls how the editor is launched:
- `auto` (default): detect common terminal editors
- `terminal`: always run in the foreground with TTY attached
- `gui`: always run in the background (non-blocking)

Vault config (`raven.yaml`) is for per-vault behavior:

```yaml
daily_directory: journal
auto_reindex: true
```
