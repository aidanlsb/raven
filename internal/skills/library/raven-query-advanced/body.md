# Raven Query Advanced

Use this skill when the user needs non-trivial Raven Query Language (RQL) work.

## Operating rules

- Prefer `rvn query` for structured filters and `rvn search` for open-ended text discovery.
- Use single-quoted query strings in shell examples.
- Decide early whether the result should be objects (`object:<type>`) or traits (`trait:<name>`).
- For `--apply`, always preview first, then add `--confirm` only when asked to apply.

## Typical flow

1. Verify schema shape first (`rvn schema`, optional `rvn resolve` and `rvn read` for ambiguous refs).
2. Start with a narrow query (`--limit`, `--count-only`, `--ids`) before broadening.
3. If this is repeated work, save the query with `rvn query add`.
4. For bulk changes, use `rvn query --apply ...` with command support appropriate to object vs trait queries.

## Load references only as needed

- RQL syntax and predicate semantics: `references/query-language.md`
- Common high-signal patterns to copy/adapt: `references/query-recipes.md`
- Error recovery and disambiguation: `references/query-troubleshooting.md`
