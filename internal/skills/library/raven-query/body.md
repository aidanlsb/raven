# Raven Query

Use this skill for structured retrieval, text search, link traversal, and saved query management.

This skill is CLI-first. Use MCP as a fallback when CLI access is unavailable, preserving the same JSON and preview/apply expectations.

## Operating rules

- Use `rvn query` for structured filters when type, field, trait, or asset shape is known.
- Use `rvn search` for open-ended text discovery when structure is unknown.
- Always pass `--json` so output is deterministic and parseable.
- Use single-quoted query strings in shell invocations to avoid shell expansion.
- Decide early whether the result should be objects (`type:<type>`), traits (`trait:<name>`), or assets (`asset`).
- Count or sample before pulling large result sets into context.
- For `--apply`, always preview first, then add `--confirm` only when the user approves.

## Choosing query vs search

- `rvn query`: returns schema-aware item rows, real trait rows, or asset rows. Use when you know the type, field names, trait names, or asset metadata. Supports predicates and pagination; object and trait queries support bulk apply.
- `rvn search`: returns file/snippet matches ranked by relevance. Use when you don't yet know the right type or structural context. Supports prefix matching, phrases, and boolean operators.
- `rvn backlinks`/`rvn outlinks`: direct link traversal by object or asset ID. Use when you need the reference graph around one specific target.

## Typical flow

1. Verify schema shape first: `rvn schema`, `rvn schema type <name>`, `rvn schema trait <name>`.
2. Estimate result size with `--count-only`, or start with a small `--limit` sample.
3. Page with `--limit` and `--offset`, and use `--ids` when the next step is another Raven command.
4. Read only the narrowed results you actually need.
5. If this is repeated work, save the query with `rvn query saved set`.
6. For bulk changes, preview with `rvn query --apply ...`, inspect the results, then add `--confirm` only after approval. Follow with a verification query or `rvn check`.

## Saved queries

- List all saved queries: `rvn query saved list --json`
- Show a saved query definition: `rvn query saved get <name> --json`
- Create or replace: `rvn query saved set <name> '<rql>' --json`
- With declared inputs: `rvn query saved set <name> '<rql with {{args.x}}>' --arg x --json`
- Remove: `rvn query saved remove <name> --json`
- Run a saved query: `rvn query <name> [inputs...] --json`

## Cross-references

- Use `raven-core` for the read/resolve commands needed to inspect query results.
- Use `raven-maintenance` for `rvn check` after bulk apply operations.

## Load references as needed

- RQL syntax and predicate semantics: `references/query-language.md`
- Common high-signal patterns to copy/adapt: `references/query-recipes.md`
- Error recovery and disambiguation: `references/query-troubleshooting.md`
