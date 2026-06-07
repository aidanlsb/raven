# Migration Checklist

1. Snapshot current state: inspect `rvn schema` and, if needed, query affected objects before changing definitions.
2. Introduce additive changes first: add new optional fields, traits, or types before changing constraints.
3. Backfill existing objects with Raven commands such as `rvn query`, `rvn set`, `rvn edit`, or `rvn reclassify`.
4. Remember that fields and traits share the same value type set; traits have one value slot, but it may be an array value.
5. Enforce stronger constraints only after the backfill is complete: required fields, enum narrowing, ref targets, or removals.
6. Run `rvn schema validate` to check schema correctness.
7. Run `rvn check` to verify vault content against the updated schema.
8. Run `rvn reindex` after normal schema changes, or `rvn reindex --full` after broad rename-heavy or external file changes (see `raven-maintenance`).
9. Re-run a focused query or check pass to confirm the intended objects now satisfy the new shape.
