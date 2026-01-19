# Error Handling

When tools return errors, here's how to handle them:

| Error Type | Meaning | What to Do |
|------------|---------|------------|
| `validation_error` | Invalid input or missing required fields | Check `retry_with` in response for corrected call template. Ask user for missing values. |
| `not_found` | Object or file doesn't exist | Verify the path/reference. Offer to create it. |
| `ambiguous_reference` | Short reference matches multiple objects | Show user the matches, ask which one they meant. Use full path. |
| `data_integrity` | Operation blocked to protect data | Explain the safety concern to user, ask for confirmation. |
| `parse_error` | YAML/markdown syntax error | Read the file, identify the syntax issue, offer to fix it. |

**Validation error recovery:**

When `raven_new` or `raven_set` fails due to missing required fields, the response includes a `retry_with` template showing exactly what call to make with the missing fields filled in. Use this to ask the user for the missing values.
