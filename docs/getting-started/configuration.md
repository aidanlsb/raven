# Configuration Guide

This page focuses only on Raven's core configuration files:
- machine-level config in `~/.config/raven/config.toml`
- vault-level config in `raven.yaml`
- a brief handoff to `schema.yaml` (covered in depth separately)

Out of scope here:
- detailed schema design patterns (`types-and-traits/schema-intro.md` and `types-and-traits/schema.md`)

## Configuration layers at a glance

| File | Scope | What it controls |
|------|-------|------------------|
| `~/.config/raven/config.toml` | your machine | which vault is default, how files open in your editor |
| `~/.config/raven/state.toml` | your machine | active vault selection (`rvn vault use`) |
| `raven.yaml` | one vault | operational behavior inside that vault |
| `schema.yaml` | one vault | data model for objects, fields, and traits |

Rule of thumb:
- if a setting should apply across all vaults on your machine, put it in `config.toml`
- if a setting should travel with a specific vault, put it in `raven.yaml` or `schema.yaml`

## Machine-level config: `config.toml`

`config.toml` is your personal Raven environment. It is where you define:
- `default_vault`
- optional `state_file` override for runtime state location
- editor settings (`editor`, `editor_mode`)
- named vaults in `[vaults]`

### Recommended baseline

```toml
default_vault = "work"
state_file = "state.toml"
editor = "code"
editor_mode = "auto"

[vaults]
work = "/Users/you/work-notes"
personal = "/Users/you/personal-notes"

[ui]
accent = "39"
code_theme = "monokai"
```

### Keys that matter most

- `default_vault`
  Vault name Raven uses when no explicit vault is provided.

- `state_file`
  Optional path to `state.toml`. If relative, it resolves relative to `config.toml`'s directory.

- `editor`
  Editor command used by open/edit actions.

- `editor_mode`
  How Raven launches the editor:
  - `auto` (default)
  - `terminal`
  - `gui`

- `[ui]`
  Optional terminal styling settings:
  - `accent`: accent color for headings/highlights
  - `code_theme`: markdown code block theme (for Glamour rendering)

- `[vaults]`
  Name-to-path mapping for your known vaults.

## Vault behavior config: `raven.yaml`

`raven.yaml` defines how a vault behaves day to day. Start by deciding:
1. where daily notes live
2. where quick capture appends
3. whether to enforce directory roots
4. whether to keep operational shortcuts (saved queries/workflows) in config

For a first setup, configure daily notes, capture, and auto-reindex first. Other sections are optional.

### A practical starting point

```yaml
auto_reindex: true

directories:
  daily: daily/
  object: object/
  page: page/
  workflow: workflows/
  template: templates/

capture:
  destination: daily
  heading: "## Captured"
  timestamp: false
```

### Key `raven.yaml` sections

#### Daily notes

```yaml
directories:
  daily: daily/
daily_template: templates/daily.md
```

- `directories.daily` sets where daily notes are stored.
- `daily_template` sets the format for newly created daily notes.

#### Capture behavior

```yaml
capture:
  destination: daily
  heading: "## Captured"
  timestamp: false
```

- `destination`: `"daily"` or a specific file path
- `heading`: optional heading under which captures are appended
- `timestamp`: whether entries are prefixed with time

#### Index behavior

```yaml
auto_reindex: true
```

- `true` keeps the index in sync automatically after edits.
- `false` gives manual control if you prefer explicit indexing.

#### Directory layout

```yaml
directories:
  daily: daily/
  object: object/
  page: page/
  workflow: workflows/
  template: templates/
```

- `daily`: root for daily notes
- `object`: root for typed objects
- `page`: root for untyped pages
- `workflow`: root for workflow definition files
- `template`: root for template files

Use this when you want strict and predictable project structure.

#### Deletion behavior

```yaml
deletion:
  behavior: trash
  trash_dir: .trash
```

- `behavior`: `trash` (safe default) or `permanent`
- `trash_dir`: where deleted files are moved when trash mode is enabled

#### Saved operational config

```yaml
queries:
  overdue:
    query: "trait:due .value==past"
    description: "Items past due"

workflows:
  meeting-prep:
    file: workflows/meeting-prep.yaml
```

- `queries` keeps shared saved-query names in the vault
- `workflows` registers reusable workflow definitions

## Vault structure config: `schema.yaml` (next section)

`schema.yaml` is deep enough to treat separately. In short, it controls:
- types
- fields and validation rules
- traits
- type templates

This guide keeps schema details out of scope so configuration stays focused.
See `types-and-traits/schema.md` for full schema rules.

---

# `raven.yaml` Reference

`raven.yaml` controls vault behavior (as opposed to structure in `schema.yaml`). It lives at the root of your vault.
This section is lookup-oriented and is not required for the first-session note -> structure -> query loop.

## Complete Example

```yaml
# Directory organization
directories:
  daily: daily/         # Root for daily notes
  object: object/       # Root for typed objects
  page: page/           # Root for untyped pages
  workflow: workflows/  # Root for workflow definition files
  template: templates/  # Root for template files

# Template for daily notes (file path)
daily_template: templates/daily.md

# Auto-reindex after CLI operations (default: true)
auto_reindex: true

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
    query: "object:project has(trait:status .value==in_progress)"
    description: "Projects marked in progress"

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

### `directories.daily`

Root directory where daily notes are stored.

| Type | Default |
|------|---------|
| string | `"daily/"` |

```yaml
directories:
  daily: journal/
```

Daily notes are created as `<directories.daily>/YYYY-MM-DD.md`.

`daily_directory` is no longer supported. Use `directories.daily`.

---

### `daily_template`

Template file path for new daily notes.

| Type | Default |
|------|---------|
| string | (none) |

**File-based template:**

```yaml
daily_template: templates/daily.md
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
| `daily` | string | Root directory for daily notes |
| `object` | string | Root directory for typed objects |
| `page` | string | Root directory for untyped pages |
| `workflow` | string | Root directory for workflow definition files |
| `template` | string | Root directory for template files |

```yaml
directories:
  daily: daily/
  object: object/
  page: page/
  workflow: workflows/
  template: templates/
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
    query: "object:project has(trait:status .value==in_progress)"
    description: "Projects marked in progress"

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

See `querying/query-language.md` for query syntax.

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

See `workflows/workflows.md` for complete workflow documentation.

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

# Directory settings
directories:
  daily: daily/
  object: object/
  page: page/
  workflow: workflows/
  template: templates/

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
    query: "object:project has(trait:status .value==in_progress)"
    description: "Projects marked in progress"

# Workflows registry
workflows:
  onboard:
    file: workflows/onboard.yaml
```

---

## Configuration Precedence

Raven uses multiple configuration sources:

1. **`~/.config/raven/config.toml`** — Global settings (default vault, editor, optional `state_file`)
2. **`~/.config/raven/state.toml`** — Mutable runtime state (for example `active_vault`)
3. **`raven.yaml`** — Vault-specific settings (this file)
4. **Command-line flags** — Override for single commands

Global config (`config.toml`) is for cross-vault settings:

```toml
default_vault = "work"
state_file = "state.toml"
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
directories:
  daily: journal/
auto_reindex: true
```
