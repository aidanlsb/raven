# MCP reference

Raven speaks MCP through `rvn serve`.

There are exactly three tools:
- `raven_discover`
- `raven_describe`
- `raven_invoke`

Earlier per-command `raven_*` tools have been removed. Use `raven_invoke` with a registry command ID instead.

## Recommended setup

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

## Vault resolution

Every vault-scoped `raven_invoke` call and vault-scoped resource read resolves a
vault in this priority order:

1. Per-call `vault_path` (explicit path)
2. Per-call `vault` (configured vault name)
3. In-memory session focus set by `vault focus`
4. Server launch pin (`rvn serve --vault-path` / `--vault`)

If none is present, Raven returns `VAULT_AMBIGUOUS`. MCP never guesses from the
CLI's active or default vault. The resolved vault is always reported in
`meta.vault_context`; see [Vault context](#vault-context).

### Switch vaults within one session

Invoke `vault_focus` through the running MCP server to change the default target
for subsequent vault-scoped calls:

```json
{
  "command": "vault_focus",
  "args": {
    "name": "work"
  }
}
```

Use `"path": "/absolute/path/to/vault"` instead of `name` to focus an
unregistered vault. The directory must exist and contain `raven.yaml`,
`schema.yaml`, or `.raven/`. Do not pass both.

A per-call top-level `vault` or `vault_path` still overrides session focus for
that invocation. Clear focus to return to the server's immutable launch pin:

```json
{
  "command": "vault_focus",
  "args": {
    "clear": true
  }
}
```

If the server started unpinned, clearing focus returns it to unpinned behavior,
so unqualified vault-scoped calls fail with `VAULT_AMBIGUOUS`. Restarting
`rvn serve` also discards session focus.

`rvn vault use` writes CLI `active_vault`, and `rvn vault pin` writes
`default_vault`; MCP intentionally reads neither. Running `rvn vault focus`
directly in a shell validates and describes the target, but cannot alter another
process's memory. Session focus changes only when `vault_focus` is invoked
through `raven_invoke` on the running server.

### Explicit vault requirement

- **`vault_context` is always present** on vault-scoped results. Inspect
  `meta.vault_context.source` to confirm which vault was used and how it was
  chosen.
- **Post-init activation disclosure:** after `init` creates an additional vault,
  it becomes active immediately. Inspect and surface `post_init.active_vault`,
  `previous_active_vault` / `previous_vault`, and `switch_back` before continuing.
  This CLI activation does not focus the MCP session; invoke `vault_focus` or
  pass `vault`/`vault_path` on later vault-scoped calls.
- **`VAULT_AMBIGUOUS`:** when no explicit `vault`/`vault_path` is given and the
  server has neither session focus nor a launch pin, the call fails with this stable error code.
  Single-vault users who pin a vault (for example via
  `rvn mcp install --vault-path`) are unaffected.

## MCP resources

Raven exposes MCP resources that agents can fetch:

| URI | Name | Description |
|-----|------|-------------|
| `raven://guide/index` | Agent Guide Index | Overview of available agent guide topics |
| `raven://schema/current` | Current Schema | The vault's verbatim `schema.yaml` defining types and traits |
| `raven://queries/saved` | Saved Queries | Saved queries from `raven.yaml`, same shape as the `query_saved_list` command's `data` |
| `raven://vault/agent-instructions` | Agent Instructions | Vault-root `AGENTS.md` when present |

Additional topic resources are available under `raven://guide/<topic>`.

Vault-scoped resource content is produced by the same shared services that back the equivalent commands, so a resource read never drifts from the command output: `raven://queries/saved` mirrors `query_saved_list` (each entry carries `name`, `query`, `args`, and `description`), and `raven://schema/current` returns the raw on-disk `schema.yaml`.

Vault-scoped resources use stable URIs. On `resources/read`, `raven://schema/current`, `raven://queries/saved`, and `raven://vault/agent-instructions` also accept optional `vault` or `vault_path` params to target a different vault for that read. Do not pass both. `resources/list` accepts the same optional `vault`/`vault_path` params, so list and read stay consistent. The list reflects (and reports) the same vault a read with identical params would target.

Both `resources/list` and `resources/read` include a `vault_context` object in the result for vault-scoped requests, mirroring `meta.vault_context` on tool results, so multi-vault sessions can always confirm which vault was used. Resource resolution also honors session focus. A vault-scoped `resources/read` without a per-call target, session focus, or server launch pin fails with `VAULT_AMBIGUOUS`.

Example:

```json
{
  "uri": "raven://schema/current",
  "vault": "work"
}
```

## Vault agent instructions

Put a file named `AGENTS.md` at the vault root to give agents vault-specific operating guidance. Raven exposes this file through `raven://vault/agent-instructions` when it exists, so MCP clients can fetch the same instructions alongside the schema and saved queries.

Use `AGENTS.md` for durable rules that are specific to the vault, such as preferred traits, task formatting conventions, naming patterns, or safety constraints. Keep it concise and operational; agents should treat it as guidance for working in the vault, not as a replacement for `schema.yaml` or `raven.yaml`.

## The three tools

The tool set is small on purpose:

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

Failed bulk mutations preserve the attempted inputs in their original order
under the command's bulk argument key in `error.details`, together with
`total`, so callers can inspect or retry the exact selection.

### Discovery flow

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

### `raven_invoke` wrapper rules

Command arguments must be nested under `args`.

```json
{
  "command": "read",
  "args": {
    "reference": "project/website.md",
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

## Available tools

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
- `trash_list`
- `restore`
- `schema`
- `schema_add_type`

Use canonical registry command IDs with `raven_describe` and `raven_invoke`.
For machine-level vault inspection, use `vault_list`. Its result includes the
configured vaults plus `default_vault`, `active_vault`, and `current_vault`.
Pass `args: {"path-only": true}` when only the resolved path is needed. The
older `vault_current` and `vault_path` IDs remain invokable compatibility
aliases but are omitted from discovery. Path-only mode is vault-scoped, so MCP
still requires a per-call target, session focus, or launch pin.

`raven_describe` returns both a short `summary` and a fuller `description` from the command registry. Use `description` for command-specific syntax guidance, such as Raven query language examples for `query`.

## Parameter conventions

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

Flag arguments keep their registry names, including hyphens: use `dry-run`,
`start-line`, `count-only`, and `fields-json`, not underscore spellings. Named
positional arguments such as `query_string`, `old_str`, and `section_id` keep
their underscores. Use `raven_describe` as the authority for each command.

Commands that target one existing vault item use `reference`; the retired
`object` / `object_id` spellings are not aliases. Bulk target keys are
`references` for commands such as `set`, `delete`, `reclassify`, `open`,
`backlinks`, and `outlinks`; `object_ids` for bulk `add` and `move`; and
`trait_ids` for bulk `update`. MCP does not receive a stdin byte stream. Pass the
array named by `raven_describe`.

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
Pass runtime options such as `limit`, `offset`, `refresh`, `apply`, and
`confirm` in that same `query` invocation. Saved query definitions never store
execution policy.

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

## Common patterns

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
    "reference": "project/website.md",
    "raw": true,
    "start-line": 10,
    "end-line": 40
  }
}
```

### File links

Copy an external non-Markdown file into the vault with the host filesystem,
then invoke `reindex`. Link to it with standard Markdown, not `[[...]]`.

```json
{
  "command": "query",
  "args": {
    "query_string": "link .ext==pdf"
  }
}
```

Use `move` for a file already inside the vault so inbound Markdown file links
are rewritten:

```json
{
  "command": "move",
  "args": {
    "source": "files/downloads/paper.pdf",
    "destination": "files/paper.pdf"
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

Create the heading explicitly, then append body content to the returned section:

```json
{
  "command": "section_create",
  "args": {
    "file": "project/website-redesign",
    "title": "Notes",
    "level": 2
  }
}
```

```json
{
  "command": "add",
  "args": {
    "text": "- Kickoff next week",
    "to": "project/website-redesign#notes"
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

Single-object writes (`set`, `add`, `update`, `edit`,
`section_create`, `section_move`, `section_rename`, `reclassify`, and
single-object `delete`/`move`) apply immediately. The call below writes on the
first invocation:

```json
{
  "command": "edit",
  "args": {
    "reference": "project/website.md",
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
    "reference": "project/website.md",
    "old_str": "Status: draft",
    "new_str": "Status: published",
    "dry-run": true
  }
}
```

High-blast-radius operations stay preview-first and require `confirm` to apply:
bulk writes (bulk ID arrays), `query` with `apply`, `schema rename`, `schema
convert`, `check` fixes, and `section_delete`.

```json
{
  "command": "delete",
  "args": {
    "references": ["project/old-one", "project/old-two"],
    "confirm": true
  }
}
```

Section deletion accepts the `reference` argument only when it is a
`file#section` ID. Omit `confirm` to return exact line bounds,
`removed_content`, every `deleted_sections` ID, and affected `backlinks`
without writing. Apply the reviewed plan with:

```json
{
  "command": "section_delete",
  "args": {
    "reference": "project/website#old-plan",
    "confirm": true
  }
}
```

Raven leaves those backlinks unchanged because no safe replacement can be
inferred; repair them explicitly after applying.

Because single-object writes apply on the first call, only invoke them when the
user's intent is clear. For `delete`/`move`, check backlinks or read the object
first, or pass `dry-run`, when the impact is not already obvious.

Deletion recovery is a separate preview-first flow. List exact entries, preview
the restore, then confirm it:

```json
{
  "command": "trash_list",
  "args": {
    "reference": "project/old"
  }
}
```

```json
{
  "command": "restore",
  "args": {
    "reference": "project/old",
    "confirm": true
  }
}
```

Omit `confirm` for the restore preview. If a reference is ambiguous, retry with
an exact `trash_path` returned by `trash_list`. Restore refuses to overwrite an
occupied `restore_path`.

### Confirming a write happened

Every mutating command reports a uniform phase in `meta.mutation.phase`:

```json
{
  "ok": true,
  "meta": { "mutation": { "phase": "applied" } }
}
```

- `applied`: the change was written to the vault.
- `preview`: nothing was written yet (a dry run, an unconfirmed high-blast-radius
  operation, or a write blocked pending confirmation).

Branch on this field instead of command-specific `data` fields (`data.status`,
`data.preview`, …). It is present on every mutating command and omitted on
read-only commands and on failures. `query` with `apply` reports the phase of the
write it delegates to.

## Best practices

1. Check the schema before creating or mutating typed items.
2. Prefer `query` over `search` when the structure is known.
3. Use raw `read` ranges before building string replacements for `edit`.
4. Use `edit` only for content markdown files; use dedicated commands for `raven.yaml`, `schema.yaml`, and templates.
5. Single-object writes apply immediately (`dry-run` to preview); bulk and query-driven mutations stay preview-first and need `confirm`. Read `meta.mutation.phase` (`applied`/`preview`) to confirm whether a write happened.
6. Copy external non-Markdown files into the vault directly; use `move` for in-vault relocation so file links are rewritten.
7. Reindex after schema-level structural changes or out-of-band file changes when required.
8. Treat `raven_describe` as the authority for argument shape.

## Related resources

- `raven://guide/quickstart`
- `raven://guide/getting-started`
- `raven://guide/response-contract`
- `raven://guide/write-patterns`
- `raven://guide/key-flows`

## Related docs

- [Query language](../querying/query-language.md): RQL syntax.
- [Bulk operations](../vault-management/bulk-operations.md): `--apply` and
  `--ids` patterns.
- [File links](../using-your-vault/file-links.md): indexing, checks, and moves.
- [Common commands](../using-your-vault/common-commands.md): the user-facing
  command map.
- [Agent setup](../getting-started/agent-setup.md): skills and client setup.
- [Documentation map](../getting-started/documentation-map.md): every topic.
