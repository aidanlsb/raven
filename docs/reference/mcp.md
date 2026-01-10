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
  - If the type has `name_field` configured, the title auto-populates that field
- `raven_query` — run ad-hoc or saved queries (`query_string`, plus `ids`, `apply`, `confirm`)
- `raven_workflow_render` — render a workflow (`name`, optional `input`)
- `raven_set` / `raven_add` / `raven_delete` / `raven_move` / `raven_edit` — mutations
- `raven_schema_add_type` — add a new type with optional `name-field` parameter
- `raven_schema_update_type` — update type, including setting `name-field`

For exact parameters, use `rvn schema commands --json`.

## name_field

Types can specify a `name_field` which:
1. Auto-populates from the `title` argument in `raven_new`
2. Enables semantic reference resolution (e.g., `[[Harry Potter]]` finds the book)

Check `raven_schema types` for hints about types missing `name_field`.

