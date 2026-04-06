# Raven Maintenance

Use this skill for vault health checks, index management, data import, and documentation access.

## Operating rules

- Prefer `rvn` CLI with `--json` for deterministic machine-readable output.
- When operating through Raven MCP, use the equivalent MCP tools instead of shelling out.
- For any fix or import operation, preview first, then apply with `--confirm` only after approval.
- Run `rvn check` after schema migrations, bulk edits, or external file changes.

## Vault health: check

`rvn check` validates vault content against the schema. It reports structured issues with suggested fixes.

- Full vault check: `rvn check --json`
- Check a specific file or directory: `rvn check <path> --json`
- Check all objects of a type: `rvn check --type project --json`
- Check all usages of a trait: `rvn check --trait due --json`
- Only specific issue types: `rvn check --issues missing_reference,unknown_type --json`
- Errors only (skip warnings): `rvn check --errors-only --json`

Each issue includes `fix_command` and `fix_hint` when a deterministic fix is available. Use those to resolve issues.

## Auto-fix workflow

1. Preview fixable issues: `rvn check fix --json`
2. Apply after review: `rvn check fix --confirm --json`
3. Create missing referenced pages: `rvn check create-missing --confirm --json`
4. Re-check the affected scope to verify: `rvn check <scope> --json`

## Reindex

The SQLite index is a derived cache. Rebuild it when queries return stale results or after external file changes.

- Incremental reindex (changed files only): `rvn reindex --json`
- Full rebuild: `rvn reindex --full --json`
- Dry run: `rvn reindex --dry-run --json`

Use `--full` after schema renames, bulk moves, or broad file changes outside Raven.

## Data import

`rvn import` creates or updates vault objects from external JSON data.

- Simple import: `echo '[{"name":"Freya"}]' | rvn import person --json`
- With field mapping: `rvn import person --file contacts.json --map full_name=name --json`
- With mapping file: `rvn import --mapping migration.yaml --file data.json --json`
- Dry run first: `rvn import person --file data.json --dry-run --json`
- Apply: `rvn import person --file data.json --confirm --json`

For complex imports, use a YAML mapping file. See `references/import-guide.md`.

## Documentation access

Browse Raven's long-form documentation stored in `.raven/docs`.

- Fetch or refresh docs: `rvn docs fetch --json`
- List sections: `rvn docs list --json`
- Read a topic: `rvn docs <section> <topic> --json`
- Search docs: `rvn docs search "<query>" --json`

## Cross-references

- Use `raven-schema` when check issues indicate schema changes are needed.
- Use `raven-query` for targeted queries to find objects affected by issues.
- Use `raven-core` for the mutation commands (`set`, `edit`, `reclassify`) used to fix issues.

## Load references as needed

- Check → fix → verify workflow details: `references/check-workflow.md`
- Import mapping file format and patterns: `references/import-guide.md`
