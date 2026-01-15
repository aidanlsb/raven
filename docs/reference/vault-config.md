# `raven.yaml` Reference

`raven.yaml` controls vault behavior (as opposed to structure in `schema.yaml`). It lives at the root of your vault.

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
  objects: objects/      # Root for typed objects
  pages: pages/          # Root for untyped pages

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
    query: "trait:due value==past"
    description: "Overdue items"
  active-projects:
    query: "object:project .status==active"
    description: "Active projects"

# Workflows (prompt templates)
workflows:
  meeting-prep:
    file: workflows/meeting-prep.yaml
  research:
    description: Research a topic
    inputs:
      question:
        type: string
        required: true
    context:
      results:
        search: "{{inputs.question}}"
    prompt: |
      Answer this question based on my notes:
      {{inputs.question}}

      ## Relevant notes
      {{context.results}}
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

| Property | Type | Description |
|----------|------|-------------|
| `objects` | string | Root directory for typed objects |
| `pages` | string | Root directory for untyped pages |

```yaml
directories:
  objects: objects/
  pages: pages/
```

**How it works:**

- Type `default_path` values are relative to `objects`
- Object IDs strip the directory prefix for shorter references
- Example: `objects/people/freya.md` → ID is `people/freya`

**Without directories configured:**

```
vault/
├── people/
│   └── freya.md          # ID: people/freya
├── projects/
│   └── website.md        # ID: projects/website
└── random-note.md        # ID: random-note
```

**With directories configured:**

```
vault/
├── objects/
│   ├── people/
│   │   └── freya.md      # ID: people/freya
│   └── projects/
│       └── website.md    # ID: projects/website
└── pages/
    └── random-note.md    # ID: random-note
```

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
    query: "trait:due value==past"
    description: "Items past their due date"

  active-projects:
    query: "object:project .status==active"
    description: "Projects with status active"

  reading-list:
    query: "trait:toread"
    description: "Books and articles to read"
```

**Usage:**

```bash
rvn query overdue              # Run the saved query
rvn query --list               # List all saved queries
rvn query add new-query "..."  # Add via CLI
rvn query remove old-query     # Remove via CLI
```

See `reference/query-language.md` for query syntax.

---

### `workflows`

Define reusable prompt templates for agents.

Workflows can be defined inline or reference external files:

**Inline definition:**

```yaml
workflows:
  meeting-prep:
    description: Prepare for a meeting
    inputs:
      meeting_id:
        type: ref
        target: meeting
        required: true
    context:
      meeting:
        read: "{{inputs.meeting_id}}"
      mentions:
        backlinks: "{{inputs.meeting_id}}"
    prompt: |
      Prepare me for this meeting.

      ## Meeting
      {{context.meeting}}

      ## Mentions
      {{context.mentions}}
```

**File reference:**

```yaml
workflows:
  meeting-prep:
    file: workflows/meeting-prep.yaml
```

See `reference/workflows.md` for complete workflow documentation.

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
    query: "trait:due value==past"
    description: "Items past their due date"

  this-week:
    query: "trait:due value==this-week"
    description: "Items due this week"

  active-projects:
    query: "object:project .status==active"
    description: "Projects with status active"
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

[vaults]
work = "/path/to/work-notes"
personal = "/path/to/personal-notes"
```

Vault config (`raven.yaml`) is for per-vault behavior:

```yaml
daily_directory: journal
auto_reindex: true
```
