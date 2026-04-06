# Check Workflow

## Common issue types

| Issue | Meaning | Typical fix |
|-------|---------|-------------|
| `unknown_type` | File declares a type not in schema | `rvn reclassify` or `rvn schema add type` |
| `missing_reference` | `[[ref]]` target doesn't exist | Create the target or fix the reference |
| `unknown_frontmatter_key` | Field not defined on the type | `rvn schema add field` or remove the key |
| `undefined_trait` | `@trait` not in schema | `rvn schema add trait` or remove from file |
| `missing_required_field` | Required field is absent | `rvn set <id> field=value` |
| `invalid_field_value` | Value doesn't match field type/enum | `rvn set <id> field=correct_value` |

## Scoped check patterns

```bash
# After editing a single file
rvn check project/website.md --json

# After schema migration affecting a type
rvn check --type project --json

# After bulk trait operations
rvn check --trait todo --json

# Focus on specific issues
rvn check --issues missing_reference,unknown_type --json

# Exclude noisy warnings
rvn check --exclude unused_type,unused_trait --json
```

## Fix → verify loop

```bash
# 1. Discover issues
rvn check --errors-only --json

# 2. Preview auto-fixes
rvn check fix --json

# 3. Apply fixes
rvn check fix --confirm --json

# 4. Handle missing references
rvn check create-missing --confirm --json

# 5. Verify
rvn check --errors-only --json
```

## When to reindex

```bash
# After external file changes (git pull, manual edits)
rvn reindex --json

# After schema renames or bulk moves
rvn reindex --full --json

# Check what would be reindexed
rvn reindex --dry-run --json
```
