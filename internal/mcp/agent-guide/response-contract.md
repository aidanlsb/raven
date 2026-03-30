# Response Contract

Use this guide to interpret Raven MCP results safely and consistently.

## Standard JSON envelope

All commands return:

```json
{
  "ok": true,
  "data": {},
  "error": null,
  "warnings": [],
  "meta": {}
}
```

## Compact invoke flow

1. `raven_discover` to fetch the authoritative command catalog, optionally filtered by `category`, `mode`, or `risk`.
2. `raven_describe(command="...")` to fetch the strict arg schema.
3. `raven_invoke(command="...", args={...})` to execute.

Important:
- Command params belong under `args`.
- Top-level keys are only `command`, `args`, `vault`, `vault_path`, `schema_hash`, `strict_schema`.
- Use `vault` for a configured vault name or `vault_path` for an explicit vault directory on a single invocation.
- Do not pass both `vault` and `vault_path`.

## Error handling rules

1. If `ok=false`, treat the operation as failed.
2. Branch on stable `error.code`.
3. Prefer `error.details.retry_with` when present.
4. Ask before retrying with assumptions.

## Preview and apply semantics

Many mutation commands are preview-first.

Examples:

```text
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo", "apply":["update done"]})
raven_invoke(command="edit", args={"path":"projects/website.md", "old_str":"A", "new_str":"B"})
raven_invoke(command="delete", args={"stdin":true})
raven_invoke(command="move", args={"stdin":true})
```

Apply only after explicit approval with `confirm=true`.

## Vault context

Vault-bound responses include a `vault_context` block in `meta`:

```json
{
  "meta": {
    "vault_context": {
      "name": "work",
      "path": "/home/user/vaults/work",
      "source": "active_vault"
    }
  }
}
```

Fields:
- `path` — resolved absolute vault path (always present).
- `source` — how the vault was selected: `vault_path` (explicit path override), `vault` (named vault from invocation), `pinned` (server pinned path), `base_args` (from serve flags), `active_vault`, `default_vault`, or `default_vault_fallback`.
- `name` — configured vault name (omitted when no name could be resolved).

Commands that do not require vault resolution (e.g. `version`, `config show`) omit `vault_context`.

## Warnings

- Warnings are action items, not noise.
- Surface warnings that affect correctness or safety.
- If warnings indicate stale state, run corrective steps such as `reindex` before continuing.

## Related topics

- `raven://guide/error-handling`
- `raven://guide/issue-types`
- `raven://guide/key-workflows`
