# Critical Rules

**⚠️ NEVER use shell commands for file operations in a Raven vault.**

Raven maintains an index and reference graph that shell commands bypass. Using shell commands will corrupt the index and break references.

| Task | ✅ USE THIS | ❌ NEVER USE |
|------|-------------|--------------|
| Move/rename files | `raven_move` | `mv`, `git mv` |
| Delete files | `raven_delete` | `rm`, `trash` |
| Create typed objects | `raven_new` | `touch`, `echo >` |
| Read vault files | `raven_read` | `cat`, `head`, `tail` |
| Edit content | `raven_edit` | `sed`, `awk`, `vim` |
| Update frontmatter | `raven_set` | Manual file edits |

**Why this is critical:**
- `raven_move` automatically updates ALL references to the moved file across the vault
- `raven_delete` warns about incoming backlinks and safely moves files to `.trash/`
- `raven_new` applies templates and validates against the schema
- Shell commands bypass ALL of this, causing broken links and stale index

If you use `mv` or `rm`, you MUST run `raven_reindex(full=true)` afterward and manually fix all broken references. **Just use the Raven commands.**
