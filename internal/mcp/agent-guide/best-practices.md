# Best Practices

1. Start with structure, not file scanning.
- Use `raven_invoke(command="schema", ...)`, `raven_invoke(command="vault_stats")`, and targeted `raven_invoke(command="query", ...)` first.
- Read files only after narrowing candidates.

2. Treat the markdown vault as source of truth.
- Index is rebuildable; use `raven_invoke(command="reindex")` when state looks stale.

3. Prefer explicit, schema-safe writes.
- Use `raven_invoke` with commands like `new`, `set`, `edit`, `move`, `delete`, and `upsert`.
- Avoid shell-level mutations for vault operations.
- See `raven://guide/critical-rules` and `raven://guide/write-patterns`.

4. Use preview-first mutation flow.
- For preview-capable commands, show preview, ask for approval, then apply with `confirm=true`.

5. Surface ambiguity instead of guessing.
- For ambiguous refs or unclear destructive intent, ask a focused clarifying question.

6. Prefer one strong query over many weak queries.
- Compose with predicates and relationships (`within`, `has`, `refs`, `ancestor`).
- Use `count-only`, `limit`, and `offset` for large sets.

7. Use issue-driven repair loops.
- `raven_invoke(command="check")` -> prioritize issue types -> apply fixes -> re-check scope.

8. Keep workflows lifecycle-aware.
- Before starting a new workflow run, check for existing `awaiting_agent` runs.
- Continue existing runs when appropriate.

9. Report both results and risk.
- Include what changed, what was validated, and any residual uncertainty.

10. Keep docs and behavior aligned.
- If command behavior changes, update guide docs and MCP user docs in the same change.
