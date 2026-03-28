# Raven Schema

Use this skill for schema modeling, migrations, and schema-driven data cleanup.

## Operating rules

- Inspect current schema before changes: `rvn schema`.
- Prefer Raven MCP tool equivalents when already running inside an MCP session; otherwise use `rvn ... --json`.
- Prefer additive changes first, then backfill objects, then tighten constraints.
- Treat schema edits and object backfill as separate steps in the same migration.
- Use the right validation pass for the job:
- `rvn schema validate`: schema file correctness
- `rvn check`: vault content against the schema
- `rvn reindex` or `rvn reindex --full`: rebuild derived index after rename-heavy or external changes

## Typical flow

1. Inspect the current shape: `rvn schema`, `rvn schema type <name>`, `rvn schema trait <name>`.
2. Make additive schema changes first:
- Add: `rvn schema add type|field|trait`
- Update: `rvn schema update type|field|trait`
3. Backfill affected objects before tightening constraints:
- Find affected objects with `rvn query`
- Update metadata with `rvn set`
- Fix body text with `rvn edit`
- Re-type existing objects with `rvn reclassify` when the schema change implies a different type
4. Tighten or clean up after the backfill is complete:
- Rename: `rvn schema rename type|field` (preview first, then confirm)
- Remove: `rvn schema remove type|field|trait`
5. Validate and refresh derived state:
- `rvn schema validate` after editing schema definitions
- `rvn check` after object backfills or migrations
- `rvn reindex --full` after rename-heavy migrations or broad file changes outside Raven

## Load references as needed

- Multi-step migration loop and sequencing: `references/migration-checklist.md`

## Safety

- Do not make fields required until every affected object already contains a valid value.
- For ref or ref[] fields, always set the target type.
- Expect `reclassify` to surface dropped fields or missing required values when schema changes alter object shape.
