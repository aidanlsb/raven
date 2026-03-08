# Issue Types Reference

Use this guide for `raven_check` triage.

Rule: in JSON mode, prefer each issue's `fix_command` and `fix_hint` over hard-coded repairs.

## Error-level issues (must fix)

| Issue Type | Meaning | Typical Action |
|------------|---------|----------------|
| `unknown_type` | File uses a type not in schema | Add/rename type in schema, or change file type. |
| `missing_reference` | Link points to missing object/section | Create missing target or update/remove reference. |
| `unknown_frontmatter_key` | Field is not defined for object type | Add schema field or remove invalid key. |
| `missing_required_field` | Required type field missing | Set required field value(s). |
| `invalid_field_value` | Field value violates schema | Correct value to match field constraints. |
| `invalid_enum_value` | Enum value outside allowed set | Use one of allowed enum values. |
| `wrong_target_type` | Ref points to object of wrong type | Replace with ref targeting correct type. |
| `invalid_date_format` | Date/datetime value malformed | Normalize to expected format (`YYYY-MM-DD`, etc.). |
| `duplicate_object_id` | Duplicate object IDs | Rename/move to unique IDs. |
| `parse_error` | YAML/markdown parse failure | Fix malformed frontmatter/content syntax. |
| `ambiguous_reference` | Ref resolves to multiple targets | Replace with explicit full-path reference. |
| `missing_target_type` | Ref/ref[] field target type not defined | Add target type or update field target. |
| `duplicate_alias` | Same alias used by multiple objects | Make aliases unique. |
| `alias_collision` | Alias collides with ID/short-name namespace | Rename alias to non-colliding value. |
| `missing_required_trait` | Required trait missing (when enforced) | Add required trait annotation or relax schema requirement. |

## Warning-level issues (usually fix)

| Issue Type | Meaning | Typical Action |
|------------|---------|----------------|
| `undefined_trait` | Trait used but not in schema | Add trait definition or remove usage. |
| `unused_type` | Type defined but unused | Remove type or create instances. |
| `unused_trait` | Trait defined but unused | Remove trait or start using it. |
| `stale_index` | Index may be stale | Run `raven_reindex()`. |
| `short_ref_could_be_full_path` | Short ref could be clearer | Consider explicit full path refs. |
| `id_collision` | Short-name ID collision risk | Use explicit full paths. |
| `self_referential_required` | Required self-ref creates impossible constraints | Make optional or provide defaults. |
| `stale_fragment` | File exists but referenced heading/fragment missing | Update fragment or target heading. |
| `unknown_field_type` | Schema field type is invalid/unsupported | Change field type to a valid schema field type. |

## Filtering patterns

```text
# Focus on high-impact fixable errors
raven_check(issues="missing_reference,unknown_type,missing_required_field")

# Exclude low-priority cleanup warnings
raven_check(exclude="unused_type,unused_trait,short_ref_could_be_full_path")

# Errors only
raven_check(errors_only=true)
```

## Practical repair loop

1. Run scoped check (`path=`, `type=`, or `trait=` when possible).
2. Group by `issue_type` and frequency.
3. Apply fixes using `fix_command` where supplied.
4. Re-run the same scoped check.
