# Response Contract

Use this guide when you need to interpret Raven tool results safely and consistently.

## Standard JSON envelope

All tools return a consistent envelope shape:

```json
{
  "ok": true,
  "data": {},
  "error": null,
  "warnings": [],
  "meta": {}
}
```

Field meanings:
- `ok`: command success flag.
- `data`: command payload when successful.
- `error`: structured error object when `ok=false`.
- `warnings`: non-fatal issues that still require agent attention.
- `meta`: optional context (counts, pagination, scope, etc.).

## Error handling rules

1. If `ok=false`, treat the operation as failed.
2. Use stable error codes to branch behavior.
3. If `error.details.retry_with` exists, use it as the canonical retry template.
4. Ask the user before retrying with assumptions.

Common error codes and next actions:

| Code | Typical Cause | Agent Action |
|------|---------------|--------------|
| `MISSING_ARGUMENT` | Required parameter not provided | Ask for the missing argument, then retry. |
| `REQUIRED_FIELD_MISSING` | Missing schema-required field | Ask for field values; prefer `retry_with` template. |
| `REF_AMBIGUOUS` | Short reference matches multiple objects | Show matches; ask user to choose full path. |
| `TYPE_NOT_FOUND` / `TRAIT_NOT_FOUND` | Schema element missing | Confirm whether to add schema or adjust request. |
| `UNKNOWN_FIELD` | Invalid frontmatter key for type | Correct field name or update schema intentionally. |
| `CONFIRMATION_REQUIRED` | Operation needs explicit confirmation | Surface preview and ask user before applying. |
| `DATA_INTEGRITY_BLOCK` | Protected destructive/schema action blocked | Explain risk and request explicit user decision. |
| `DATABASE_ERROR` | Query/search execution issue | Re-check syntax and rerun with simpler query. |

## Warning handling rules

Warnings are not errors. They are action items.

- Surface warnings to the user when they affect correctness/safety.
- Do not silently ignore warnings on destructive operations.
- If warnings indicate stale state (index/schema mismatch), run corrective steps (`raven_reindex`, schema check) before continuing.

## Preview and apply semantics

Many mutation tools are preview-first:
- First call without `confirm=true` to get a preview.
- Present the preview to the user.
- Apply only after explicit approval with `confirm=true`.

This applies to bulk and high-impact operations such as:
- `raven_query(..., apply="...")`
- `raven_edit(...)`
- `raven_delete(..., stdin=true)`
- `raven_move(..., stdin=true)`

## Transport vs tool failures

If the client reports no Raven payload (for example, "No result received"), treat it as transport/execution failure:
- Retry once with the same arguments.
- Re-validate required args.
- Do not misclassify as schema/data corruption without a Raven error envelope.

## Related topics

- `raven://guide/error-handling` - recovery patterns by scenario
- `raven://guide/issue-types` - `raven_check` issue-level fixes
- `raven://guide/key-workflows` - preview/confirm operational flow
