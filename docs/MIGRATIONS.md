# Raven Migrations & Upgrades

This document describes how Raven handles version upgrades and breaking changes.

## Core Principle: Markdown is King

**Your markdown files are the source of truth.** The SQLite database is a disposable cache that can always be rebuilt with `rvn reindex`. This means:

- Database schema changes: Just delete `.raven/` and reindex
- No data loss risk: Your notes are plain text files
- Sync-friendly: Database is local-only, never synced

## Version Tracking

### Schema Version

`schema.yaml` includes a version field:

```yaml
version: 2  # Schema format version

types:
  # ...
```

When Raven loads a schema, it checks the version and:
1. If version is current: proceed normally
2. If version is old: warn user and offer migration
3. If version is missing: assume v1, suggest upgrade

### Database Version

The SQLite database tracks its schema version in a `meta` table. On startup:
1. Check database version vs. expected version
2. If mismatched: delete and recreate (it's just a cache)

## Breaking Changes

### Syntax Changes (e.g., trait format)

When we change syntax (like `@task(due=...)` → `@due(...)`):

1. **Deprecation period**: Both syntaxes work for 1-2 versions
2. **Migration tool**: `rvn migrate` updates files
3. **Check warnings**: `rvn check` warns about deprecated syntax
4. **Removal**: After deprecation period, old syntax becomes error

Example workflow:
```bash
# After upgrading Raven
rvn check
# Warning: Found 15 uses of deprecated @task() syntax
# Run 'rvn migrate' to update files

rvn migrate --dry-run  # Preview changes
rvn migrate            # Apply changes (creates backup)
```

### Schema Format Changes

When the schema.yaml format changes:

```bash
rvn check
# Warning: schema.yaml is version 1, current is version 2
# Run 'rvn migrate --schema' to upgrade

rvn migrate --schema --dry-run  # Preview
rvn migrate --schema            # Apply (backs up old schema)
```

### Configuration Changes

New config files (like `raven.yaml`) use sensible defaults if missing:
- If `raven.yaml` doesn't exist → use defaults
- If old format → migrate automatically or warn

## Migration Command

```bash
rvn migrate [flags]

Flags:
  --dry-run       Preview changes without applying
  --schema        Migrate schema.yaml format
  --syntax        Migrate deprecated syntax in markdown files
  --all           Migrate everything
  --backup-dir    Where to store backups (default: .raven/backups/)

# Check what needs migration
rvn migrate --dry-run --all

# Migrate everything
rvn migrate --all
```

## Backup Strategy

Before any file modification, Raven:
1. Creates timestamped backup: `.raven/backups/2025-01-15T10-30-00/`
2. Copies affected files
3. Applies changes
4. Reports what was changed

```bash
# Restore from backup if needed
cp -r .raven/backups/2025-01-15T10-30-00/* .
```

## Version Compatibility Matrix

| Raven Version | Schema Version | Syntax | Notes |
|---------------|----------------|--------|-------|
| 0.1.x | 1 | `@task(field=value)` | Initial release |
| 0.2.x | 2 | `@trait(value)` | Atomic traits, saved queries |

## Best Practices

### For Users

1. **Before upgrading Raven**: Run `rvn check` to ensure vault is clean
2. **After upgrading**: Run `rvn check` again to see any deprecation warnings
3. **Regular backups**: Your markdown files should be in git/synced anyway
4. **Don't delete .raven/**: It contains backups and local state

### For Development

1. **Add deprecation warnings first**: Let users know syntax will change
2. **Provide migration tools**: Don't just break things
3. **Support old syntax temporarily**: Allow graceful transition
4. **Document breaking changes**: Update this file
5. **Increment version numbers**: Track schema/syntax versions

## Changelog

### v0.2.0 (Atomic Traits)

**Breaking changes:**
- Trait syntax changed from `@trait(field=value, ...)` to `@trait(value)`
- Removed `cli:` block from trait definitions
- Saved queries moved to `raven.yaml`

**Migration:**
```bash
# Migrate trait syntax in files
rvn migrate --syntax

# Example transformations:
# @task(due=2025-02-01, priority=high) → @due(2025-02-01) @priority(high)
# @remind(at=2025-02-01T09:00) → @remind(2025-02-01T09:00)
# @highlight(color=yellow) → @highlight  (if color was default)

# Migrate schema.yaml
rvn migrate --schema

# Example transformations:
# traits.task.fields.due → traits.due.type: date
# traits.task.cli.alias → queries.tasks in raven.yaml
```
