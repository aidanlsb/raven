# Configuration Guide

This guide covers global and vault-level configuration. 
- global machine config in `config.toml`
- vault-level behavior config in `raven.yaml`

For schema details, see:
- `docs/types-and-traits/schema-intro.md`
- `docs/types-and-traits/schema.md`

## Configuration layers

| File | Scope | Purpose |
|------|-------|---------|
| `~/.config/raven/config.toml` | machine | Global defaults and vault registry |
| `~/.config/raven/state.toml` | machine | Mutable runtime state (`active_vault`) |
| `raven.yaml` (vault root) | per vault | Vault behavior and operational config |

Rule of thumb:
- Put cross-vault machine settings in `config.toml`.
- Put vault behavior that should travel with the vault in `raven.yaml`.
- Put structure and validation in `schema.yaml` (separate docs).

---

## Global config: `config.toml`

`config.toml` controls how Raven resolves vaults and launches your editor.

### Typical example

```toml
default_vault = "work"
state_file = "state.toml"
editor = "cursor"
editor_mode = "auto"

[vaults]
work = "/Users/you/work-notes"
personal = "/Users/you/personal-notes"

[ui]
accent = "39"
code_theme = "monokai"
```

### Keys

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `default_vault` | string | none | Name from `[vaults]` used as fallback when no explicit vault is provided |
| `state_file` | string | `state.toml` next to `config.toml` | Relative paths are resolved relative to the config directory |
| `editor` | string | `$EDITOR` | Used by commands that open files |
| `editor_mode` | string | `auto` behavior in caller logic | One of `auto`, `terminal`, `gui` |
| `[vaults]` | table | empty | Name -> absolute path mapping |
| `[ui].accent` | string | unset | Accent color for styled terminal output. Supports ANSI (`"0"`-`"255"`) or hex (`"#RRGGBB"` / `"#RGB"`). |
| `[ui].code_theme` | string | unset (`monokai` effective default) | Markdown code-block theme (Glamour/Chroma), for example `monokai`, `dracula`, `github` |

### UI options in detail

`[ui]` controls human-facing terminal presentation. It does not affect JSON payloads (`--json`).

#### `[ui].accent`

Purpose:
- Colors section headers, divider labels, and syntax-highlighted Raven markers in CLI output.

Accepted values:
- ANSI color index as string: `"0"` to `"255"` (example: `"39"`).
- Hex color: `"#RRGGBB"` or shorthand `"#RGB"` (example: `"#5fd7ff"` or `"#5cf"`).
- Disable accent explicitly with `"none"`, `"off"`, or `"default"` (in `config.toml`).

Behavior details:
- `#RGB` is normalized internally to `#RRGGBB`.
- If the value is invalid, Raven falls back to its default non-accent style (bold headings, default syntax color).
- `rvn config set --ui-accent` only enforces non-empty input; format validity is evaluated when output is rendered.

Examples:

```bash
rvn config set --ui-accent 39 --json
rvn config set --ui-accent '#5fd7ff' --json
rvn config unset --ui-accent --json
```

#### `[ui].code_theme`

Purpose:
- Selects the Glamour/Chroma theme used for fenced code blocks in rendered markdown output.

Accepted values:
- Any Chroma style name (case-insensitive), for example: `monokai`, `dracula`, `github`, `nord`.

Behavior details:
- Current scope: markdown rendering paths (for example `rvn read` without `--raw` in terminal output).
- Empty or invalid theme values fall back to `monokai`.
- `rvn config set --ui-code-theme` only enforces non-empty input; theme validity is resolved when markdown is rendered.

Examples:

```bash
rvn config set --ui-code-theme dracula --json
rvn config set --ui-code-theme GitHub --json
rvn config unset --ui-code-theme --json
```

#### Combined example

```toml
[ui]
accent = "#5fd7ff"
code_theme = "github"
```

### Legacy compatibility

- `vault` (single string path) is still supported for backward compatibility.
- Prefer `default_vault` + `[vaults]` for new setups.

### Path resolution and overrides

- Config path:
  - `--config` flag wins if provided.
  - Otherwise Raven uses its default config location.
- State path:
  - `--state` flag wins if provided.
  - Else `state_file` from `config.toml`.
  - Else `state.toml` beside `config.toml`.

### Vault resolution order at runtime

For commands that operate on a vault:
1. `--vault-path`
2. `--vault <name>` (resolved via `[vaults]`)
3. `active_vault` from `state.toml`
4. `default_vault` from `config.toml`

If `active_vault` is set but missing from config, Raven falls back to `default_vault` and emits a warning in non-JSON mode.

### Manage global vault config via CLI/MCP

Instead of editing `config.toml` manually, you can manage vault entries directly:

```bash
rvn vault add personal /Users/you/personal-notes --pin --json
rvn vault use personal --json
rvn vault list --json
rvn vault remove personal --clear-default --clear-active --json
```

MCP exposes these as:
- `raven_vault_add`
- `raven_vault_use`
- `raven_vault_pin`
- `raven_vault_list`
- `raven_vault_remove`
- `raven_vault_clear`

### Manage global config fields via CLI/MCP

Use `rvn config` for machine-level config lifecycle and explicit field edits:

```bash
rvn config init --json
rvn config show --json
rvn config set --editor cursor --editor-mode auto --json
rvn config set --ui-accent 39 --ui-code-theme monokai --json
rvn config unset --ui-accent --ui-code-theme --json
```

MCP exposes these as:
- `raven_config`
- `raven_config_show`
- `raven_config_init`
- `raven_config_set`
- `raven_config_unset`

---

## Vault config: `raven.yaml`

`raven.yaml` controls per-vault behavior: directories, auto-reindexing, capture, deletion, saved queries, workflow registration, workflow run retention, and protected paths.

### Practical baseline

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

### Full example (current canonical shape)

```yaml
directories:
  daily: daily/
  object: object/
  page: page/
  workflow: workflows/
  template: templates/

auto_reindex: true

capture:
  destination: daily
  heading: "## Captured"
  timestamp: false

deletion:
  behavior: trash
  trash_dir: .trash

queries:
  overdue:
    query: "trait:due .value==past"
    description: "Items past due"
  project-todos:
    query: "trait:todo refs([[{{args.project}}]])"
    args: [project]
    description: "Todos linked to a project"

workflows:
  meeting-prep:
    file: workflows/meeting-prep.yaml

workflow_runs:
  storage_path: .raven/workflow-runs
  auto_prune: true
  keep_completed_for_days: 7
  keep_failed_for_days: 14
  keep_awaiting_for_days: 30
  max_runs: 1000
  preserve_latest_per_workflow: 5

protected_prefixes:
  - templates/
  - private/
```

## `raven.yaml` reference

### `auto_reindex`

Automatically reindex after CLI commands that modify vault content.

| Type | Default |
|------|---------|
| boolean | `true` |

When disabled, run `rvn reindex` manually.

### `directories`

Directory roots used by Raven.

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `daily` | string | `daily/` | Daily note files are `<daily>/YYYY-MM-DD.md` |
| `object` | string | unset | Root for typed objects |
| `page` | string | unset, but defaults to `object` when `object` is set and `page` is omitted | Root for untyped pages |
| `workflow` | string | `workflows/` | Root for workflow definition files |
| `template` | string | `templates/` | Root for template files referenced by schema |

Behavior notes:
- Paths are normalized as vault-relative paths.
- `workflow` and `template` are normalized with trailing `/`.
- `daily` is normalized and used as a directory name (no trailing `/` in resolved internal value).
- If the entire `directories` block is missing, Raven uses a flat layout (no `object`/`page` roots).

Compatibility notes:
- Singular keys are canonical: `object`, `page`, `workflow`, `template`.
- Legacy plural keys are still accepted: `objects`, `pages`, `workflows`, `templates`.
- If both singular and plural are present, singular wins.
- `daily_directory` is no longer supported and causes a config error.
- Legacy top-level `workflow_directory` is still accepted and mapped to `directories.workflow`.

### `capture`

Quick capture defaults for `rvn add`.

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `destination` | string | `daily` | `"daily"` or a vault-relative file path like `inbox.md` |
| `heading` | string | unset | If set, appends under that heading (creates heading if missing) |
| `timestamp` | boolean | `false` | Prefix each capture with `HH:MM` |

### `deletion`

Behavior for `rvn delete`.

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `behavior` | string | `trash` | `trash` or `permanent` |
| `trash_dir` | string | `.trash` | Vault-relative trash location when `behavior: trash` |

### `queries`

Saved query registry used by `rvn query <name>`.

Each query entry supports:

| Key | Type | Required | Notes |
|-----|------|----------|-------|
| `query` | string | yes | Raven Query Language string |
| `args` | string[] | no | Declares accepted placeholder names and positional order |
| `description` | string | no | Human-readable description |

For parameterized saved queries, use placeholders like `{{args.project}}` and declare `args`.

### `workflows`

Workflow declaration registry (name -> file reference).

```yaml
workflows:
  meeting-prep:
    file: workflows/meeting-prep.yaml
```

Rules:
- Declarations are file references only.
- Inline workflow bodies in `raven.yaml` are not supported.
- Workflow files must live under `directories.workflow` (default `workflows/`).
- The workflow name `runs` is reserved.

### `workflow_runs`

Workflow run checkpoint storage and retention policy.

| Key | Type | Default |
|-----|------|---------|
| `storage_path` | string | `.raven/workflow-runs` |
| `auto_prune` | boolean | `true` |
| `keep_completed_for_days` | integer | `7` |
| `keep_failed_for_days` | integer | `14` |
| `keep_awaiting_for_days` | integer | `30` |
| `max_runs` | integer | `1000` |
| `preserve_latest_per_workflow` | integer | `5` |

Compatibility:
- Legacy `workflows.runs` is still read for backward compatibility.
- Use top-level `workflow_runs` for new config.

### `protected_prefixes`

Additional vault-relative prefixes treated as protected/system-managed by automation features.

| Type | Default |
|------|---------|
| string[] | empty |

This is additive. Raven always protects:
- `.raven/`
- `.trash/`
- `.git/`
- `raven.yaml`
- `schema.yaml`

### `daily_template` (legacy)

`daily_template` remains in the config model for backward compatibility, but daily templating is schema-driven in current Raven. Use `schema.yaml` (`types.date.templates` and `types.date.default_template`) instead.

---

## Defaults from `rvn init`

`rvn init` creates a default `raven.yaml` with:
- `directories.daily`, `directories.object`, `directories.page`, `directories.workflow`, `directories.template`
- `auto_reindex: true`
- starter `queries`
- starter `workflows` entry pointing to `workflows/onboard.yaml`

It also creates the default folders and the onboard workflow file.

---

## What belongs in `schema.yaml` (and not here)

Keep these out of `raven.yaml`:
- type definitions
- field definitions and validation
- trait definitions
- template ID definitions and type template bindings

Use:
- `docs/types-and-traits/schema-intro.md` for concepts
- `docs/types-and-traits/schema.md` for full reference
