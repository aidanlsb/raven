# Issue Types Reference

Use this guide for `check` triage.

Rule: in JSON mode, prefer each issue's `fix_command` and `fix_hint` over hard-coded repairs.

`check` does not report issues for paths matched by `raven.yaml` `exclude`
patterns. Those files are outside Raven's managed content model, not hidden
check failures.

## Error-level issues

| Issue Type | Meaning | Typical Action |
|------------|---------|----------------|
| `unknown_type` | File uses a type not in schema | Add/rename type in schema, or change file type |
| `missing_reference` | Link points to missing object/section | Preview `check create-missing`, then confirm or update/remove the reference |
| `missing_asset` | Asset reference points to a missing non-Markdown file | Add the asset under the configured asset root or update/remove the reference |
| `local_fragment_ref` | Wikilink uses unsupported source-relative fragment syntax like `[[#tasks]]` | Rewrite it as a global section ref like `[[object#tasks]]` |
| `unknown_frontmatter_key` | Field is not defined for object type | Add schema field or remove invalid key |
| `missing_required_field` | Required type field missing | Set required field value(s) |
| `invalid_field_value` | Field value violates schema | Correct value to match constraints |
| `invalid_enum_value` | Enum trait value is not allowed | Correct value to match the trait schema; for unnecessarily quoted enum values, run `check fix --confirm` |
| `wrong_target_type` | Ref points to object of wrong type | Replace with a ref targeting the correct type |
| `non_canonical_path` | File lives outside the configured directory root for its type | Run `check fix --confirm` to move file to canonical location |

## Warning-level issues

| Issue Type | Meaning | Typical Action |
|------------|---------|----------------|
| `undefined_trait` | Trait used but not in schema | Add trait definition or remove usage |
| `unused_type` | Type defined but unused | Remove type or create instances |
| `unused_trait` | Trait defined but unused | Remove trait or start using it |
| `stale_index` | Index may be stale | Run `raven_invoke(command="reindex")` (or `rvn reindex` in the CLI) |
| `short_ref_could_be_full_path` | Short ref could be clearer | Run `check fix --confirm` to rewrite to explicit full-path refs |
| `non_canonical_ref` | Wikilink target includes the configured root prefix (e.g. `[[type/person/jane]]`) | Run `check fix --confirm` to rewrite to canonical form (`[[person/jane]]`) |
| `orphaned_asset` | Indexed asset has no incoming references | Link it from a note or remove it if unused |

## Filtering patterns

```text
raven_invoke(command="check", args={"issues":"missing_reference,unknown_type,missing_required_field"})
raven_invoke(command="check", args={"exclude":"unused_type,unused_trait,short_ref_could_be_full_path"})
raven_invoke(command="check", args={"errors_only":true})
raven_invoke(command="check create-missing", args={})
raven_invoke(command="check create-missing", args={"confirm":true})
```

## Practical repair loop

1. Run a scoped check (`path`, `type`, or `trait` when possible).
2. Group by `issue_type` and frequency.
3. Apply fixes using `fix_command` where supplied.
4. Re-run the same scoped check.

For `missing_reference` summaries, use `check create-missing` as a preview-first
batch workflow. It creates only deterministic typed targets when confirmed; ask
the user before creating uncertain targets or editing existing links.
