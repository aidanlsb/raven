# Issue Types Reference

When `raven_check` returns issues, here's how to fix them:

**Errors (must fix):**

| Issue Type | Meaning | Fix Command |
|------------|---------|-------------|
| `unknown_type` | File uses a type not in schema | `raven_schema_add_type(name="book")` |
| `missing_reference` | Link to non-existent page | `raven_new(type="person", title="Freya")` |
| `unknown_frontmatter_key` | Field not defined for type | `raven_schema_add_field(type_name="person", field_name="company")` |
| `missing_required_field` | Required field not set | `raven_set(object_id="...", fields={"name": "..."})` |
| `missing_required_trait` | Required trait not set | `raven_set(object_id="...", fields={"due": "2025-02-01"})` |
| `invalid_enum_value` | Value not in allowed list | `raven_set(object_id="...", fields={"status": "done"})` |
| `wrong_target_type` | Ref field points to wrong type | Update the reference to point to correct type |
| `invalid_date_format` | Date/datetime value malformed | Fix to YYYY-MM-DD format |
| `duplicate_object_id` | Multiple objects share same ID | Rename one of the duplicates |
| `parse_error` | YAML frontmatter or syntax error | Fix the malformed syntax |
| `ambiguous_reference` | Reference matches multiple objects | Use full path: `[[people/freya]]` |
| `missing_target_type` | Ref field's target type doesn't exist | Add the target type to schema |
| `duplicate_alias` | Multiple objects use same alias | Rename one of the aliases |
| `alias_collision` | Alias conflicts with object ID/short name | Rename the alias |

**Warnings (optional to fix):**

| Issue Type | Meaning | Fix Suggestion |
|------------|---------|----------------|
| `undefined_trait` | Trait not in schema | `raven_schema_add_trait(name="toread", type="boolean")` |
| `unused_type` | Type defined but never used | Remove from schema or create an instance |
| `unused_trait` | Trait defined but never used | Remove from schema or use it |
| `stale_index` | Index needs reindexing | `raven_reindex()` |
| `short_ref_could_be_full_path` | Short ref could be clearer | Consider using full path |
| `id_collision` | Short name matches multiple objects | Use full paths in references |
| `self_referential_required` | Type has required ref to itself | Make field optional or add default |

**Using issue types for filtering:**

```
# Focus on actionable errors only
raven_check(issues="missing_reference,unknown_type,missing_required_field")

# Skip noisy schema warnings during cleanup
raven_check(exclude="unused_type,unused_trait,short_ref_could_be_full_path")

# Just errors, no warnings
raven_check(errors_only=true)
```
