# Critical Rules

These rules are non-negotiable.

## Use Raven commands, not shell file mutations

| Intent | Command ID | Do not use |
|--------|------------|------------|
| Import external non-Markdown files | `asset_import` | `cp`, `mv` |
| Move or rename files, including assets | `move` | `mv`, `git mv` |
| Create section headings | `section_create` | `add`, manual heading edits |
| Reorder/reparent sections | `section_move` | `move`, manual cut/paste |
| Rename section headings | `section_rename` | `section_move`, `move`, manual heading edits |
| Delete files | `delete` | `rm`, `trash` |
| Create typed items | `new` | `touch`, `echo >` |
| Read vault files | `read` | `cat`, `head`, `tail` |
| Edit content files | `edit` | ad hoc shell text replacement |
| Update frontmatter | `set` | manual YAML edits |

Why:
- `asset_import` confines new assets to `directories.assets`, handles collisions explicitly, and updates the index.
- `move` updates references, including Markdown links/images that point at assets.
- `section_create` validates levels and slug stability; `add` is body-only.
- `section_move` preserves heading identity and moves the complete subtree.
- `section_rename` updates inbound fragment references; object `move` rejects section sources.
- `delete` checks impact and uses safe deletion behavior.
- `new` applies schema and templates.
- `edit` is for content markdown only; use `vault config`, `schema`, and `template` for control-plane files.

Single-object `delete` applies immediately when invoked (both CLI and MCP). Only
use it after clear user intent; if deletion impact is uncertain, inspect the
object, run `backlinks`, or call with `dry-run=true` first. Bulk delete still
previews unless `confirm=true`.

If you bypass Raven and mutate files directly, reindex and repair before continuing. This also applies to adding, moving, or deleting files under the configured asset root. Use `asset_import` for external files and `move` for files already in the vault.

## Confirm the target vault before writing

Every vault-scoped result includes `meta.vault_context`. Confirm it points at the
intended vault before writing.

- Pass an explicit `vault` (configured name) or `vault_path` (absolute directory)
  on each call, set the session focus with `vault_focus`, or use a server pinned
  to one vault.
- To switch vaults for the rest of this MCP server session, invoke
  `vault_focus`; a per-call `vault`/`vault_path` still overrides it once. Clear
  focus to restore the server's launch pin.
- Calls without a per-call target, session focus, or server launch pin fail with
  `VAULT_AMBIGUOUS`. MCP never uses `active_vault` or `default_vault`.
- After `init` creates an additional vault, it becomes active for CLI routing.
  Surface the returned active/previous vault details and `switch_back` command,
  then explicitly target or focus it for MCP calls.

## Respect managed-content boundaries

`protected_prefixes` and `exclude` are different:
- `protected_prefixes` marks managed paths that Raven must not mutate.
- `exclude` marks unmanaged paths that Raven should not check, index, query, or mutate.

If a path is excluded, do not try to work around Raven by editing it as vault content. Ask the user whether they want to remove or narrow the exclusion first.
