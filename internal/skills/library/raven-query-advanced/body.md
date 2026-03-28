# Raven Query Advanced

Use this skill when the user needs non-trivial Raven Query Language (RQL) work.

## Operating rules

- Prefer `rvn query` for structured filters and `rvn search` for open-ended text discovery.
- When the agent is already connected through Raven MCP, prefer the matching MCP tool over an extra CLI subprocess.
- Use single-quoted query strings in shell examples.
- Decide early whether the result should be objects (`object:<type>`) or traits (`trait:<name>`).
- Count or sample before pulling large result sets into context.
- For `--apply`, always preview first, then add `--confirm` only when asked to apply.

## Typical flow

1. Verify schema shape first (`rvn schema`, optional `rvn resolve` and `rvn read` for ambiguous refs).
2. Estimate result size with `--count-only`, or start with a small `--limit` sample.
3. Page with `--limit` and `--offset`, and use `--ids` when the next step is another Raven command.
4. Read only the narrowed results you actually need; switch to `rvn search` only when structure is unknown.
5. If this is repeated work, save the query with `rvn query add`.
6. For bulk changes, preview `rvn query --apply ...`, inspect the sample or IDs, then add `--confirm` only after approval and follow with a verification query or `rvn check`.

## Load references only as needed

- RQL syntax and predicate semantics: `references/query-language.md`
- Common high-signal patterns to copy/adapt: `references/query-recipes.md`
- Error recovery and disambiguation: `references/query-troubleshooting.md`
