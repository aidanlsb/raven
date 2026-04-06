# Critical Rules

These rules are non-negotiable.

## Use Raven commands, not shell file mutations

| Intent | Command ID | Do not use |
|--------|------------|------------|
| Move or rename files | `move` | `mv`, `git mv` |
| Delete files | `delete` | `rm`, `trash` |
| Create typed objects | `new` | `touch`, `echo >` |
| Read vault files | `read` | `cat`, `head`, `tail` |
| Edit content files | `edit` | ad hoc shell text replacement |
| Update frontmatter | `set` | manual YAML edits |

Why:
- `move` updates references.
- `delete` checks impact and uses safe deletion behavior.
- `new` applies schema and templates.
- `edit` is for content markdown only; use `vault config`, `schema`, and `template` for control-plane files.

If you bypass Raven and mutate files directly, reindex and repair before continuing.
