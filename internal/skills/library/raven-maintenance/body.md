# Raven Maintenance

Use this skill for vault health checks, index management, and data import.

This skill is CLI-first. Use MCP as a fallback when CLI access is unavailable, preserving the same JSON and preview/apply expectations.

## Operating rules

- Use `rvn` with `--json` for deterministic machine-readable output.
- Preview first when the command supports it. For `rvn import`, use `--dry-run` for preview and rerun without it to apply.
- Run `rvn check` after schema migrations, bulk edits, or external file changes.

## Vault stats

Use `rvn vault stats --json` for a quick count of indexed files, objects, traits, references, and assets before or after maintenance work.

## Vault health: check

`rvn check` validates vault content against the schema. It reports structured issues with suggested fixes.

- Full vault check: `rvn check --json`
- Check a specific file or directory: `rvn check <path> --json`
- Check all objects of a type: `rvn check --type project --json`
- Check all usages of a trait: `rvn check --trait due --json`
- Only specific issue types: `rvn check --issues missing_reference,unknown_type --json`
- Errors only (skip warnings): `rvn check --errors-only --json`
- Group by file for triage: `rvn check --by-file --json`
- Full issue details: `rvn check --verbose --json`

Each issue includes `fix_command` and `fix_hint` when available. Use `rvn check fix` only for unambiguous safe fixes; other issues may require `raven-core`, `raven-schema`, or user clarification.

## Auto-fix workflow

1. Preview fixable issues: `rvn check fix --json`
2. Apply safe fixes after review: `rvn check fix --confirm --json`
3. Preview missing referenced pages: `rvn check create-missing --json`
4. Create missing referenced pages after review: `rvn check create-missing --confirm --json`
5. Re-check the affected scope to verify: `rvn check <scope> --json`

## Reindex

The SQLite index is a derived cache. Rebuild it when queries return stale results or after external file changes.

- Incremental reindex (changed files only): `rvn reindex --json`
- Full rebuild: `rvn reindex --full --json`
- Dry run: `rvn reindex --dry-run --json`

Use `--dry-run` to inspect reindex scope before applying. Use `--full` after schema renames, bulk moves, or broad file changes outside Raven.

## Data import

`rvn import` creates or updates vault objects from external JSON data.

- Simple import: `echo '[{"name":"Freya"}]' | rvn import person --json`
- With field mapping: `rvn import person --file contacts.json --map full_name=name --json`
- With mapping file: `rvn import --mapping migration.yaml --file data.json --json`
- Dry run first: `rvn import person --file data.json --dry-run --json`
- Apply: `rvn import person --file data.json --json`

For complex imports, use a YAML mapping file. After applying, verify with `rvn check --type <type> --json` and a targeted `rvn query`. See `references/import-guide.md`.

## Cross-references

- Use `raven-schema` when check issues indicate schema changes are needed.
- Use `raven-query` for targeted queries to find objects affected by issues.
- Use `raven-core` for the mutation commands (`set`, `edit`, `reclassify`) used to fix issues.

## Load references as needed

- Check → fix → verify workflow details: `references/check-workflow.md`
- Import mapping file format and patterns: `references/import-guide.md`
