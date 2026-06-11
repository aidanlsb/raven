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
| `stale_fragment` | Link points to an existing object but a missing section fragment | Update the fragment to match an existing heading, or remove the fragment |
| `ambiguous_reference` | Link matches multiple objects, aliases, or short names | Rewrite the link with a more specific object path or rename the conflicting alias/short name |
| `unknown_frontmatter_key` | Field is not defined for object type | Add schema field or remove invalid key |
| `duplicate_object_id` | A file defines the same object ID more than once | Rename one of the duplicate objects |
| `missing_required_field` | Required type field missing | Set required field value(s) |
| `missing_required_trait` | Required trait missing | Add the required trait or change the schema requirement |
| `invalid_field_value` | Field value violates schema | Correct value to match constraints |
| `invalid_enum_value` | Enum trait value is not allowed | Correct value to match the trait schema; for unnecessarily quoted enum values, run `check fix --confirm` |
| `invalid_date_format` | Date or datetime trait value has the wrong format | Use `YYYY-MM-DD`, `YYYY-MM-DDTHH:MM`, or `YYYY-MM-DDTHH:MM:SS` as appropriate |
| `wrong_target_type` | Ref points to object of wrong type | Replace with a ref targeting the correct type |
| `parse_error` | File could not be parsed as valid Raven markdown/frontmatter | Fix the YAML frontmatter or markdown syntax |
| `missing_target_type` | Schema ref field targets a type that does not exist | Add the target type or change the field target |
| `duplicate_alias` | Multiple objects define the same alias | Rename one of the conflicting aliases |
| `alias_collision` | Alias conflicts with an object ID or short name | Rename the alias or use full paths in references |
| `non_canonical_path` | File lives outside the configured directory root for its type | Run `check fix --confirm` to move file to canonical location |

## Mixed-level issues

| Issue Type | Meaning | Typical Action |
|------------|---------|----------------|
| `invalid_trait_value` | Trait value is missing, invalid, or the trait schema definition is invalid | Add a value when required, fix the value to match schema, or fix the schema definition |

## Warning-level issues

| Issue Type | Meaning | Typical Action |
|------------|---------|----------------|
| `undefined_trait` | Trait used but not in schema | Add trait definition or remove usage |
| `unused_type` | Type defined but unused | Remove type or create instances |
| `unused_trait` | Trait defined but unused | Remove trait or start using it |
| `stale_index` | Index may be stale | Run `raven_invoke(command="reindex")` (or `rvn reindex` in the CLI) |
| `unknown_field_type` | Schema field has an unrecognized field type | Change the schema field to a supported type |
| `self_referential_required` | Required ref field points to the same type, making the first object hard to create | Make the field optional or add a default value |
| `id_collision` | Short name matches multiple objects and that short name is used in a reference | Use full paths in references to avoid ambiguity |
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
