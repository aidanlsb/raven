# Raven Schema

Use this skill for schema modeling, migrations, and schema-driven data cleanup.

This skill is CLI-first. Use MCP as a fallback when CLI access is unavailable, preserving the same JSON and preview/apply expectations.

## Operating rules

- Inspect current schema before changes: `rvn schema --json`.
- Use `rvn ... --json` for all schema operations so output stays deterministic.
- Prefer additive changes first, then backfill objects, then tighten constraints.
- Treat schema edits and object backfill as separate steps in the same migration.
- Fields and traits use the same value type set; traits have one value slot, but that value may be an array.
- Use the right validation pass for the job:
  - `rvn schema validate`: schema file correctness
  - `rvn check`: vault content against the schema
  - `rvn reindex`: refresh derived state after schema/index-impacting changes
  - `rvn reindex --full`: rebuild after rename-heavy or external file changes

## Value types

Valid field and trait value types are:

- Scalars: `string`, `number`, `url`, `date`, `datetime`, `enum`, `bool`, `ref`
- Arrays: `string[]`, `number[]`, `url[]`, `date[]`, `datetime[]`, `enum[]`, `bool[]`, `ref[]`

Rules:

- `enum` and `enum[]` require `--values`.
- `ref` and `ref[]` fields require `--target`; trait references do not have target constraints.
- Legacy trait type `boolean` is accepted as an alias for `bool`.

Examples:

- `rvn schema add trait tags --type string[] --json`
- `rvn schema add trait reviewers --type ref[] --json`
- `rvn schema add field project reviewers --type ref[] --target person --json`

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
   - `rvn check` after object backfills or migrations (see `raven-maintenance`)
   - `rvn reindex` after normal schema changes
   - `rvn reindex --full` after rename-heavy migrations or broad file changes (see `raven-maintenance`)

## Load references as needed

- Multi-step migration loop and sequencing: `references/migration-checklist.md`

## Cross-references

- Use `raven-core` for the backfill commands (`rvn set`, `rvn edit`, `rvn reclassify`) used during migrations.
- Use `raven-query` for finding affected objects during backfill.
- Use `raven-maintenance` for `rvn check`, `rvn check fix`, and `rvn reindex` after migrations complete.

## Safety

- Do not make fields required until every affected object already contains a valid value.
- For ref or ref[] fields, always set the target type.
- Removed fields remain in frontmatter but are no longer validated; removed traits remain in files but are no longer indexed.
- Expect `reclassify` to surface dropped fields or missing required values when schema changes alter object shape.
