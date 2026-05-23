# Error Handling

Use this guide to recover from tool failures predictably.

For envelope semantics, see `raven://guide/response-contract`.

## 1. Read the envelope first

- If `ok=true`, inspect `warnings`.
- If `ok=false`, branch on `error.code` and use `error.details`.
- Prefer `error.details.retry_with` when present.

## 2. Warning handling

- Surface warnings that affect correctness.
- For write operations, include warnings before asking to continue.
- If warning indicates stale derived state, run `reindex` and retry.

## 3. Transport failures

If there is no Raven JSON payload at all:
1. Retry once with identical arguments.
2. Re-check required args and command ID.
3. Do not assume data/schema corruption without a Raven envelope.

## 4. Schema validation failures

When `new`, `upsert`, `set`, `import`, or schema commands fail because a value does not match the schema:
1. Inspect `error.details`, especially `retry_with`, `field`, `expected`, and `actual` when present.
2. Read the live schema before retrying:
   ```text
   raven_invoke(command="schema", args={"subcommand":"type", "name":"<type>"})
   ```
3. Retry with schema-valid fields and values. Prefer command arguments such as `fields-json` over raw content edits.
4. If the user intent requires changing the schema, ask before adding fields, changing enum values, or relaxing required fields.

Do not bypass validation by editing frontmatter manually. Unknown frontmatter keys and invalid values will be reported by `check` and may not be indexed.

## 5. Recovery loop for check/repair tasks

```text
raven_invoke(command="check")
```

Then:
1. Prioritize issues by impact.
2. Apply a targeted fix with user confirmation.
3. Re-run a scoped check.

## Related topics

- `raven://guide/issue-types`
