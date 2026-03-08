# Error Handling

Use this guide to recover from tool failures with predictable behavior.

For envelope semantics, see `raven://guide/response-contract`.

## 1. Read the envelope first

- If `ok=true`, continue and inspect `warnings`.
- If `ok=false`, branch on `error.code` and use `error.details`.
- Prefer `error.details.retry_with` when present.

## 2. Common error codes and responses

| Code | Meaning | Agent Response |
|------|---------|----------------|
| `MISSING_ARGUMENT` | Required arg missing | Ask for the missing input and retry. |
| `REQUIRED_FIELD_MISSING` | Schema-required field missing | Ask for required field values; use `retry_with` template. |
| `REF_AMBIGUOUS` | Reference resolves to multiple objects | Present candidates and ask for explicit target. |
| `REF_NOT_FOUND` / `FILE_NOT_FOUND` / `OBJECT_NOT_FOUND` | Target missing | Confirm intent: create, correct path, or skip. |
| `TYPE_NOT_FOUND` / `TRAIT_NOT_FOUND` | Schema element absent | Confirm whether to add schema or change operation. |
| `UNKNOWN_FIELD` | Field not valid for type | Correct field or update schema intentionally. |
| `CONFIRMATION_REQUIRED` | Unsafe change requires explicit approval | Show preview/impact and request approval. |
| `DATA_INTEGRITY_BLOCK` | Operation blocked to protect data | Explain risk and ask how to proceed. |
| `QUERY_INVALID` / `DATABASE_ERROR` | Query expression/search execution issue | Simplify query; quote special tokens; retry. |
| `WORKFLOW_*` | Workflow state/input/execution failure | Inspect run/status/step outputs and resume carefully. |

## 3. Warning handling

Warnings are actionable, not ignorable noise.

- If warning affects correctness (stale index, mismatched type paths, backlinks), surface it.
- For write operations, include warnings in your summary before asking to continue.
- If warning indicates stale derived state, run `raven_reindex` and retry.

## 4. "No result received" or silent tool failures

If there is no Raven JSON payload at all, treat it as transport/execution failure.

Recovery steps:
1. Retry once with identical arguments.
2. Re-check required args and exact tool name.
3. If still failing, avoid assuming data/schema corruption.

## 5. Recovery loop for check/repair tasks

1. `raven_check(...)`
2. Prioritize issues by impact.
3. Apply targeted fix with user confirmation.
4. Re-run scoped check to verify.

## Related topics

- `raven://guide/issue-types` - issue-level fixes for `raven_check`
- `raven://guide/key-workflows` - operational mutation playbooks
- `raven://guide/workflow-lifecycle` - workflow-specific recovery
