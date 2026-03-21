# Issue Types Reference

Use this guide for `check` triage.

Rule: in JSON mode, prefer each issue's `fix_command` and `fix_hint` over hard-coded repairs.

## Error-level issues

| Issue Type | Meaning | Typical Action |
|------------|---------|----------------|
| `unknown_type` | File uses a type not in schema | Add/rename type in schema, or change file type |
| `missing_reference` | Link points to missing object/section | Create missing target or update/remove reference |
| `unknown_frontmatter_key` | Field is not defined for object type | Add schema field or remove invalid key |
| `missing_required_field` | Required type field missing | Set required field value(s) |
| `invalid_field_value` | Field value violates schema | Correct value to match constraints |
| `wrong_target_type` | Ref points to object of wrong type | Replace with a ref targeting the correct type |

## Warning-level issues

| Issue Type | Meaning | Typical Action |
|------------|---------|----------------|
| `undefined_trait` | Trait used but not in schema | Add trait definition or remove usage |
| `unused_type` | Type defined but unused | Remove type or create instances |
| `unused_trait` | Trait defined but unused | Remove trait or start using it |
| `stale_index` | Index may be stale | Run `reindex` |
| `short_ref_could_be_full_path` | Short ref could be clearer | Consider explicit full-path refs |

## Filtering patterns

```text
raven_invoke(command="check", args={"issues":"missing_reference,unknown_type,missing_required_field"})
raven_invoke(command="check", args={"exclude":"unused_type,unused_trait,short_ref_could_be_full_path"})
raven_invoke(command="check", args={"errors_only":true})
```

## Practical repair loop

1. Run a scoped check (`path`, `type`, or `trait` when possible).
2. Group by `issue_type` and frequency.
3. Apply fixes using `fix_command` where supplied.
4. Re-run the same scoped check.
