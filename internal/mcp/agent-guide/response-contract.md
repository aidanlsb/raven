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

1. `raven_discover` to find a command ID.
2. `raven_describe(command="...")` to fetch the strict arg schema.
3. `raven_invoke(command="...", args={...})` to execute.

Important:
- Command params belong under `args`.
- Top-level keys are only `command`, `args`, `schema_hash`, `strict_schema`.

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

## Warnings

- Warnings are action items, not noise.
- Surface warnings that affect correctness or safety.
- If warnings indicate stale state, run corrective steps such as `reindex` before continuing.

## Related topics

- `raven://guide/error-handling`
- `raven://guide/issue-types`
- `raven://guide/key-workflows`
