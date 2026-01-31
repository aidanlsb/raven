# Best Practices

1. **Always use Raven commands instead of shell commands**: Raven commands maintain index consistency and update references automatically.

   | Task | Use This | NOT This |
   |------|----------|----------|
   | Move/rename files | `raven_move` | `mv` |
   | Delete files | `raven_delete` | `rm` |
   | Create typed objects | `raven_new` | Writing files directly |
   | Update frontmatter | `raven_set` | Manual file edits |
   | Edit content | `raven_edit` | `sed`, `awk`, etc. |
   | Read vault files | `raven_read` | `cat`, `head`, etc. |

   **Why this matters:**
   - `raven_move` updates all references to the moved file automatically
   - `raven_delete` warns about backlinks and moves files to `.trash/`
   - `raven_new` applies templates and validates against the schema
   - `raven_set` validates field values and triggers reindexing

2. **Master the query language**: A single well-crafted query is better than multiple simple queries and file reads. Invest time in understanding predicates and composition.

3. **Err on more information**: When in doubt about what the user wants, provide more results rather than fewer. Run multiple query interpretations if ambiguous.

4. **Always ask before bulk changes**: "I found 45 files with unknown type 'book'. Should I add this type to your schema?"

5. **Preview before applying**: Operations like `raven_edit`, `raven_query --apply`, and bulk operations preview by default. Changes are NOT applied unless `confirm=true`.

   - For `raven_edit`, the default output *is* a dry-run preview (includes before/after context and line number). Only run again with `confirm=true` to apply.
   - When preparing an edit, prefer reading raw content first: `raven_read(path="...", raw=true)` so you can copy an exact `old_str`.

6. **Use the schema as source of truth**: If something isn't in the schema, it won't be indexed or queryable. Guide users to define their types and traits.

7. **Prefer structured queries over search**: Use `raven_query` with the query language before falling back to `raven_search`.

   - Object/trait query results include `file_path` (and often `line`) in JSON output—use those paths for `raven_read` and `raven_edit`.

8. **Check before creating**: Use `raven_backlinks` or `raven_search` to see if something already exists before creating duplicates.

9. **Respect user's organization**: Look at existing `default_path` settings to understand where different types of content belong.

10. **Reindex after schema changes**: If you add types or traits, run `raven_reindex(full=true)` so all files are re-parsed with the new schema.

11. **Check for workflows proactively**: When a user asks for complex analysis, check `raven_workflow_list()` first — there may be a workflow designed for their request.
