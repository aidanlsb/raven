# Migration Checklist

1. Snapshot current state (`rvn schema`, optional git commit).
2. Introduce additive changes first (new optional fields/types/traits).
3. Backfill data where needed.
4. Enforce stronger constraints (required/enums/ref targets).
5. Run `rvn schema validate` and `rvn check`.
6. Run `rvn reindex` (or `--full` for broad rename/migration changes).
