# Agents (MCP)

Raven exposes its CLI as MCP tools via `rvn serve`.

## Claude Desktop setup

Add a server entry to Claude Desktop config:

```json
{
  "mcpServers": {
    "raven": {
      "command": "/path/to/rvn",
      "args": ["serve", "--vault-path", "/path/to/vault"]
    }
  }
}
```

## How agents should use Raven

- Prefer **structured queries** (`raven_query`) over full-text search.
- When a command returns a structured "missing required fields" error, ask the user and retry.
- For bulk actions, preview first (no `confirm`), then apply after approval.
- Check `raven_schema types` for hints about types missing `name_field` configuration.

### name_field for easier object creation

Types can have a `name_field` that auto-populates from the title argument:

```
# With name_field: name configured on person type
raven_new(type="person", title="Freya")  # Automatically sets name="Freya"
```

If `raven_schema types` shows a hint about types without `name_field`, suggest setting it up:
```
raven_schema_update_type(name="person", name-field="name")
```

## Tool catalog

The MCP tool list is generated from Raven's command registry. For the authoritative list at runtime:
- `rvn schema commands --json` (CLI)

Reference: `reference/mcp.md`.
