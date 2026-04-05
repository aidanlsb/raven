# MCP Reference

Raven exposes a compact MCP surface via `rvn serve`.

The only documented public MCP tools are:
- `raven_discover`
- `raven_describe`
- `raven_invoke`

Legacy direct-call compatibility tools are not part of the MCP surface.

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

## MCP Resources

Raven exposes MCP resources that agents can fetch:

| URI | Name | Description |
|-----|------|-------------|
| `raven://guide/index` | Agent Guide Index | Overview of available agent guide topics |
| `raven://schema/current` | Current Schema | The vault's `schema.yaml` defining types and traits |
| `raven://queries/saved` | Saved Queries | Saved queries from `raven.yaml` |
| `raven://vault/agent-instructions` | Agent Instructions | Vault-root `AGENTS.md` when present |

Additional topic resources are available under `raven://guide/<topic>`.

## Compact Tool Surface

The MCP surface is intentionally compact:

- `raven_discover` lists all discoverable commands with compact metadata.
- `raven_describe` returns the strict invocation contract for one command.
- `raven_invoke` executes a registry command with validation and policy checks.

### Discovery Flow

Use this sequence:

1. `raven_discover` to fetch the full command catalog.
2. `raven_describe(command="...")` to fetch the strict argument contract.
3. `raven_invoke(command="...", args={...})` to execute.

Example:

```json
{
  "command": "query",
  "args": {
    "query_string": "object:project .status==active",
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

Preview:

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

Apply:

```json
{
  "command": "edit",
  "args": {
    "path": "project/website.md",
    "old_str": "Status: draft",
    "new_str": "Status: published",
    "confirm": true
  }
}
```

## Best Practices

1. Check the schema before creating or mutating typed objects.
2. Prefer `query` over `search` when the structure is known.
3. Use raw `read` ranges before building string replacements for `edit`.
4. Preview destructive or bulk mutations before applying with `confirm=true`.
5. Reindex after schema-level structural changes when required.
6. Treat `raven_describe` as the authority for argument shape.

## Related Resources

- `raven://guide/quickstart`
- `raven://guide/getting-started`
- `raven://guide/response-contract`
- `raven://guide/write-patterns`
- `raven://guide/key-flows`

## Related Docs

- `querying/query-language.md` — RQL syntax for `query` commands
- `vault-management/bulk-operations.md` — `--apply` and `--ids` patterns for bulk changes
- `using-your-vault/common-commands.md` — full command surface (read, search, edit, check, etc.)
