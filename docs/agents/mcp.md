# MCP Reference

Raven exposes a compact MCP surface via `rvn serve`.

The MCP surface is exactly three tools:
- `raven_discover`
- `raven_describe`
- `raven_invoke`

Earlier per-command `raven_*` tools have been removed. Use `raven_invoke` with a registry command ID instead.

## Recommended Setup

Install Raven into a supported MCP client config:

```bash
rvn mcp install --client codex --vault-path /path/to/vault
rvn mcp install --client claude-desktop --vault-path /path/to/vault
```

Supported clients:
- `codex`
- `claude-code`
- `claude-desktop`
- `cursor`

Examples:

```bash
rvn mcp install --client codex --vault-path /path/to/vault
rvn mcp install --client claude-code --vault-path /path/to/vault
rvn mcp install --client cursor --vault-path /path/to/vault
rvn mcp status
```

If your client is unsupported, generate the config snippet manually:

```bash
rvn mcp show --vault-path /path/to/vault
```

For Codex, `rvn mcp show --client codex` prints the TOML snippet for `~/.codex/config.toml`.

Start the server directly with:

```bash
rvn serve --vault-path /path/to/vault
```

## Vault Resolution and Strict Mode

Every vault-scoped `raven_invoke` call and vault-scoped resource read resolves a
vault in this priority order:

1. Per-call `vault_path` (explicit path)
2. Per-call `vault` (configured vault name)
3. Server-pinned vault (`rvn serve --vault-path` / `--vault`)
4. Ambient global state: active vault (`rvn vault use`), then default vault

Sources 1–3 are *explicit*. Source 4 is *ambient* — it depends on mutable global
state, so an agent that omits an explicit vault can silently operate on whatever
vault was left active. The resolved vault is always reported in
`meta.vault_context` (see below).

### Guarding against silent wrong-vault operations

- **`vault_context` is always present** on vault-scoped results. Inspect
  `meta.vault_context.source` to confirm which vault was used and how it was
  chosen.
- **`VAULT_FALLBACK` warning:** in the default mode, when a write command
  resolves its vault from ambient state (`active_vault`/`default_vault`) while
  more than one vault is configured, the response carries a `VAULT_FALLBACK`
  warning. Pass an explicit `vault`/`vault_path` to silence it and be sure of
  the target.
- **Post-init selection guard:** after `init` creates an additional vault without
  activating it, an ambient call that would resolve to the previously active/default
  vault fails with `VAULT_AMBIGUOUS`. Activate the new vault with `vault use`, or pass
  an explicit `vault`/`vault_path` on each call until the user chooses.
- **Strict vault mode:** start the server with `rvn serve --strict-vault` (or set
  `[mcp] strict_vault = true` in `config.toml`) to require an explicit vault for
  every vault-scoped call. When no explicit `vault`/`vault_path` is given and the
  server has no pinned vault, the call fails with the stable error code
  `VAULT_AMBIGUOUS` instead of falling back to ambient state. Single-vault users
  who pin a vault (e.g. via `rvn mcp install --vault-path`) are unaffected.

An explicit `--strict-vault` flag always overrides the config value; pass
`--strict-vault=false` to force-disable it even when the config enables it.

## MCP Resources

Raven exposes MCP resources that agents can fetch:

| URI | Name | Description |
|-----|------|-------------|
| `raven://guide/index` | Agent Guide Index | Overview of available agent guide topics |
| `raven://schema/current` | Current Schema | The vault's verbatim `schema.yaml` defining types and traits |
| `raven://queries/saved` | Saved Queries | Saved queries from `raven.yaml`, same shape as the `query_saved_list` command's `data` |
| `raven://vault/agent-instructions` | Agent Instructions | Vault-root `AGENTS.md` when present |

Additional topic resources are available under `raven://guide/<topic>`.

Vault-scoped resource content is produced by the same shared services that back the equivalent commands, so a resource read never drifts from the command output: `raven://queries/saved` mirrors `query_saved_list` (each entry carries `name`, `query`, `args`, `description`, and `options` when set), and `raven://schema/current` returns the raw on-disk `schema.yaml`.

Vault-scoped resources use stable URIs. On `resources/read`, `raven://schema/current`, `raven://queries/saved`, and `raven://vault/agent-instructions` also accept optional `vault` or `vault_path` params to target a different vault for that read. Do not pass both. `resources/list` accepts the same optional `vault`/`vault_path` params, so list and read stay consistent — the list reflects (and reports) the same vault a read with identical params would target.

Both `resources/list` and `resources/read` include a `vault_context` object in the result for vault-scoped requests, mirroring `meta.vault_context` on tool results, so multi-vault sessions can always confirm which vault was used. In strict vault mode (see above), a vault-scoped `resources/read` without an explicit vault fails with `VAULT_AMBIGUOUS`.

Example:

```json
{
  "uri": "raven://schema/current",
  "vault": "work"
}
```

## Vault Agent Instructions

Put a file named `AGENTS.md` at the vault root to give agents vault-specific operating guidance. Raven exposes this file through `raven://vault/agent-instructions` when it exists, so MCP clients can fetch the same instructions alongside the schema and saved queries.

Use `AGENTS.md` for durable rules that are specific to the vault, such as preferred traits, task formatting conventions, naming patterns, or safety constraints. Keep it concise and operational; agents should treat it as guidance for working in the vault, not as a replacement for `schema.yaml` or `raven.yaml`.

## Compact Tool Surface

The MCP surface is intentionally compact:

- `raven_discover` lists all discoverable commands with compact metadata.
- `raven_describe` returns the strict invocation contract for one command.
- `raven_invoke` executes a registry command with validation and policy checks.

Successful responses put their primary homogeneous collection in `data.items`.
This includes `raven_discover`, query/search windows, docs search, ambiguous
`resolve` candidates, import outcomes, and bulk preview/apply rows. Empty
collections are `[]`, not `null`. Specialized or secondary collections retain
descriptive keys, such as `ids`, `issues`, `sections`, `items_by_target`, and
`items_by_source`; ambiguity failures retain candidate IDs in
`error.details.matches`.

### Discovery Flow

Use this sequence:

1. `raven_discover` to fetch the full command catalog.
2. `raven_describe(command="...")` to fetch the strict argument contract and command guidance.
3. `raven_invoke(command="...", args={...})` to execute.

Example:

```json
{
  "command": "query",
  "args": {
    "query_string": "type:project .status==active",
    "limit": 20
  }
}
```

### `raven_invoke` Wrapper Rules

Command arguments must be nested under `args`.

```json
{
  "command": "read",
  "args": {
    "path": "project/website.md",
    "raw": true
  }
}
```

Top-level keys are reserved for the invoke envelope only:
- `command`
- `args`
- `vault`
- `vault_path`
- `schema_hash`
- `strict_schema`

Use `vault` to target a configured vault name for a single call, or `vault_path` to target an explicit vault directory for a single call. Do not pass both in the same invocation.

Passing command-specific parameters beside `command` fails with `INVALID_ARGS`.

## Available Tools

This tool list is generated from the command registry and should stay in sync with `internal/mcp/tools.go`.

<!-- BEGIN MCP TOOL LIST -->
| Tool | Description |
|------|-------------|
| `raven_describe` | Fetch the compact invocation contract for one Raven command. |
| `raven_discover` | List all discoverable Raven commands with compact metadata. |
| `raven_invoke` | Invoke any registry command with strict typed validation and policy checks (command args must be nested inside args). |
<!-- END MCP TOOL LIST -->

## Command IDs

`raven_invoke` operates on canonical command IDs from the registry, for example:
- `read`
- `search`
- `query`
- `new`
- `add`
- `set`
- `schema`
- `schema_add_type`

Use canonical registry command IDs with `raven_describe` and `raven_invoke`.

`raven_describe` returns both a short `summary` and a fuller `description` from the command registry. Use `description` for command-specific syntax guidance, such as Raven query language examples for `query`.

## Parameter Conventions

### Positional CLI args become `args` fields

CLI:

```text
rvn new person "Freya"
```

MCP:

```json
{
  "command": "new",
  "args": {
    "type": "person",
    "title": "Freya"
  }
}
```

### Key-value flags become JSON objects or arrays

Repeatable `--flag key=value` patterns are passed under `args`.

Example:

```json
{
  "command": "new",
  "args": {
    "type": "person",
    "title": "Freya",
    "field": {
      "email": "freya@asgard.realm",
      "role": "engineer"
    }
  }
}
```

### Repeatable string flags use arrays

Example bulk apply preview:

```json
{
  "command": "query",
  "args": {
    "query_string": "trait:todo .value==todo",
    "apply": ["update done"]
  }
}
```

### Saved query inputs

Saved queries still use the `query` command. Pass the saved query name as `query_string` and optional inputs under `inputs`.

```json
{
  "command": "query",
  "args": {
    "query_string": "project-todos",
    "inputs": {
      "project": "project/raven"
    }
  }
}
```

### Saved query management

Use the dedicated saved-query commands to inspect or update definitions.

```json
{
  "command": "query_saved_set",
  "args": {
    "name": "project-todos",
    "query_string": "trait:todo refs([[{{args.project}}]])",
    "arg": ["project"],
    "description": "Todos linked to a project"
  }
}
```

### Vault config management

Use dedicated vault-config commands for supported `raven.yaml` settings instead of raw file edits.

```json
{
  "command": "vault_config_auto_reindex_set",
  "args": {
    "value": false
  }
}
```

```json
{
  "command": "vault_config_protected_prefixes_add",
  "args": {
    "prefix": "private"
  }
}
```

```json
{
  "command": "vault_config_exclude_add",
  "args": {
    "pattern": ".cursor/"
  }
}
```

```json
{
  "command": "vault_config_directories_set",
  "args": {
    "daily": "journal",
    "type": "type",
    "template": "templates/custom"
  }
}
```

```json
{
  "command": "vault_config_directories_set",
  "args": {
    "assets": "assets"
  }
}
```

```json
{
  "command": "vault_config_capture_set",
  "args": {
    "destination": "inbox.md",
    "heading": "## Captured"
  }
}
```

```json
{
  "command": "vault_config_deletion_set",
  "args": {
    "behavior": "permanent",
    "trash-dir": "archive/trash"
  }
}
```

## Common Patterns

### Read and search

```json
{
  "command": "search",
  "args": {
    "query": "meeting notes",
    "type": "meeting"
  }
}
```

```json
{
  "command": "read",
  "args": {
    "path": "project/website.md",
    "raw": true,
    "start_line": 10,
    "end_line": 40
  }
}
```

### Asset references

Use `backlinks` to find notes that reference a vault-local asset. Use `move` instead of shell `mv` so Markdown links/images and wikilinks are rewritten.

```json
{
  "command": "backlinks",
  "args": {
    "target": "assets/pdfs/paper.pdf"
  }
}
```

```json
{
  "command": "move",
  "args": {
    "source": "assets/downloads/paper.pdf",
    "destination": "assets/pdfs/paper.pdf"
  }
}
```

### Create and enrich an object

```json
{
  "command": "new",
  "args": {
    "type": "project",
    "title": "Website Redesign"
  }
}
```

Then append content:

```json
{
  "command": "add",
  "args": {
    "text": "## Notes\n- Kickoff next week",
    "to": "project/website-redesign.md"
  }
}
```

### Schema inspection

```json
{
  "command": "schema",
  "args": {
    "subcommand": "type",
    "name": "person"
  }
}
```

### Preview/apply flow

Single-object writes (`set`, `add`, `update`, `edit`, and single-object
`delete`/`move`) apply immediately. The call below writes on the first
invocation:

```json
{
  "command": "edit",
  "args": {
    "path": "project/website.md",
    "old_str": "Status: draft",
    "new_str": "Status: published"
  }
}
```

Pass `dry-run` to preview a single-object write without applying it:

```json
{
  "command": "edit",
  "args": {
    "path": "project/website.md",
    "old_str": "Status: draft",
    "new_str": "Status: published",
    "dry-run": true
  }
}
```

High-blast-radius operations stay preview-first and require `confirm` to apply:
bulk writes (`stdin`), `query` with `apply`, `schema rename`, and `check` fixes.

```json
{
  "command": "delete",
  "args": {
    "stdin": true,
    "confirm": true
  }
}
```

Because single-object writes apply on the first call, only invoke them when the
user's intent is clear. For `delete`/`move`, check backlinks or read the object
first—or pass `dry-run`—when the impact is not already obvious.

### Confirming a write happened

Every mutating command reports a uniform phase in `meta.mutation.phase`:

```json
{
  "ok": true,
  "meta": { "mutation": { "phase": "applied" } }
}
```

- `applied` — the change was written to the vault.
- `preview` — nothing was written yet (a dry run, an unconfirmed high-blast-radius
  operation, or a write blocked pending confirmation).

Branch on this field instead of command-specific `data` fields (`data.status`,
`data.preview`, …). It is present on every mutating command and omitted on
read-only commands and on failures. `query` with `apply` reports the phase of the
write it delegates to.

## Best Practices

1. Check the schema before creating or mutating typed items.
2. Prefer `query` over `search` when the structure is known.
3. Use raw `read` ranges before building string replacements for `edit`.
4. Use `edit` only for content markdown files; use dedicated commands for `raven.yaml`, `schema.yaml`, and templates.
5. Single-object writes apply immediately (`dry-run` to preview); bulk and query-driven mutations stay preview-first and need `confirm`. Read `meta.mutation.phase` (`applied`/`preview`) to confirm whether a write happened.
6. Use `move` for asset relocation so references and the asset index stay correct.
7. Reindex after schema-level structural changes or out-of-band asset file changes when required.
8. Treat `raven_describe` as the authority for argument shape.

## Related Resources

- `raven://guide/quickstart`
- `raven://guide/getting-started`
- `raven://guide/response-contract`
- `raven://guide/write-patterns`
- `raven://guide/key-flows`

## Related Docs

- `querying/query-language.md` — RQL syntax for `query` commands
- `vault-management/bulk-operations.md` — `--apply` and `--ids` patterns for bulk changes
- `using-your-vault/assets.md` — asset organization, linking, and checks
- `using-your-vault/common-commands.md` — full command surface (read, search, edit, check, etc.)
