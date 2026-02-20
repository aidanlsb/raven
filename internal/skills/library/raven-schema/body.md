# Raven Schema

Use this skill for schema modeling and schema migrations.

## Operating rules

- Inspect current schema before changes: `rvn schema`.
- Prefer incremental changes over large one-shot edits.
- Validate and reindex after structural changes.

## Typical flow

1. Inspect type and trait definitions: `rvn schema type <name>`, `rvn schema trait <name>`.
2. Apply changes:
- Add: `rvn schema add type|field|trait`.
- Update: `rvn schema update type|field|trait`.
- Rename: `rvn schema rename type|field` (preview first, then confirm).
- Remove: `rvn schema remove type|field|trait`.
3. Validate consistency: `rvn schema validate`, `rvn check`.
4. Refresh derived index: `rvn reindex --full` after rename-heavy migrations.

## Safety

- Do not make fields required until all objects contain valid values.
- For ref/ref[] fields, always set target type.
