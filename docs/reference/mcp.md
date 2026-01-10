# MCP (reference)

Raven exposes its CLI commands as MCP tools via `rvn serve`.

## Server

```bash
rvn serve --vault-path /path/to/vault
```

All tool calls execute Raven CLI commands with `--json` output.

## Tool discovery

Tools are generated from Raven’s command registry. For an authoritative list (including arguments):

```bash
rvn schema commands --json
```

## Conventions

- **Positional CLI args** become top-level tool properties (e.g. `type`, `title`, `name`).
- **Repeatable `--flag k=v`** patterns are represented as JSON objects in MCP (e.g. `field: {name: "Freya"}`).
- **Preview/confirm**: bulk-style actions typically default to preview; pass `confirm: true` to apply.

## Common tools (high level)

- `raven_new` — create a typed note (`type`, **title required in MCP/non-interactive**, optional `field`)
- `raven_query` — run ad-hoc or saved queries (`query_string`, plus `ids`, `apply`, `confirm`)
- `raven_workflow_render` — render a workflow (`name`, optional `input`)
- `raven_set` / `raven_add` / `raven_delete` / `raven_move` / `raven_edit` — mutations

For exact parameters, use `rvn schema commands --json`.

