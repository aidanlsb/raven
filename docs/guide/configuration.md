# Configuration Guide

This page focuses only on Raven's core configuration files:
- machine-level config in `~/.config/raven/config.toml`
- vault-level config in `raven.yaml`
- a brief handoff to `schema.yaml` (covered in depth separately)

Out of scope here:
- detailed schema design patterns (`schema-intro.md` and `reference/schema.md`)
- command tutorials (`cli-basics.md` and `cli-advanced.md`)

For complete syntax reference, use:
- `reference/vault-config.md`
- `reference/schema.md`

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
Use `reference/schema.md` for now; we can add a dedicated schema guide section next.

## Related docs

- `getting-started.md` for first-run setup flow
- `templates.md` for template lifecycle setup and usage
- `schema-intro.md` for guide-level schema setup
- `reference/vault-config.md` for complete `raven.yaml` options
- `reference/schema.md` for full schema rules
