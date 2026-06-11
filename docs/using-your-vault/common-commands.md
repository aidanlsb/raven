# Common Commands

This guide covers the everyday Raven commands that are not covered by dedicated docs elsewhere. Each command gets a brief description, key flags, and examples. Run `rvn help <command>` for the full flag reference.

For daily notes and quick capture (`rvn daily`, `rvn add`), see `using-your-vault/daily-notes.md`. For query syntax, see `querying/query-language.md`. For bulk operations on query results, see `vault-management/bulk-operations.md`.

## Interactive Pickers

If `fzf` is installed and Raven is running in an interactive terminal, a few commands open a picker when you omit the target argument:

```bash
rvn read   # Pick an indexed vault file to read
rvn open   # Pick an indexed vault file to open
rvn docs   # Pick a docs topic to view
```

Interactive pickers are disabled for `--json` output and other non-interactive/scripted contexts. If `fzf` is not installed, pass the target path or docs topic explicitly.

The picker respects your `FZF_DEFAULT_OPTS` environment variable, so you can customize its appearance (layout, height, border, colors, keybindings) the same way you configure fzf everywhere else. Raven applies `--layout=reverse --height=80% --border` as defaults, but anything in your `FZF_DEFAULT_OPTS` overrides them:

```bash
# Override Raven's picker defaults
export FZF_DEFAULT_OPTS='--height=100% --no-border --color=hl:cyan'
```

---

## Reading content

### `rvn read`

Display an object's content. By default, Raven renders wiki-links and appends backlinks. Use `--raw` for plain file content (recommended when preparing edits). Use your editor or operating system tools to view binary assets.

```bash
rvn read person/freya                     # Enriched output with backlinks
rvn read person/freya --raw               # Plain markdown, no extras
rvn read project/website --raw --start-line 10 --end-line 40   # Line range
rvn read                                  # Interactive picker (requires fzf)
```

Key flags:
- `--raw` — raw file content only (no backlinks, no rendered links)
- `--start-line`, `--end-line` — read a specific line range (with `--raw`)
- `--lines` — include line numbers (useful for agents preparing edits)

### `rvn open`

Open a file in your configured editor (`editor` in `config.toml` or `$EDITOR`). The `editor_mode` setting controls launch behavior: `auto` (detect GUI vs terminal), `terminal` (always inline), or `gui` (always detached). See `using-your-vault/configuration.md` for details.

```bash
rvn open project/website
rvn open                                  # Interactive picker (requires fzf)
```

---

## Finding content

### `rvn search`

Full-text search across all vault content with relevance ranking.

```bash
rvn search "meeting notes"                # Simple search
rvn search '"team meeting"'               # Exact phrase (quote inside quotes)
rvn search "meeting AND notes"            # Boolean query
rvn search "rollout" --type project       # Filter by type
rvn search "auth" --limit 5              # Limit results
```

Search syntax:

| Syntax | Meaning | Example |
|--------|---------|---------|
| `word` | Term search | `rollout` |
| `"phrase"` | Exact phrase match | `"team meeting"` |
| `A AND B` | Both terms required | `meeting AND notes` |
| `A OR B` | Either term | `design OR redesign` |
| `NOT A` | Exclude term | `meeting NOT standup` |

Key flags:
- `--type` / `-t` — filter results to a specific type
- `--limit` / `-n` — maximum results (default 20)

### `rvn backlinks`

Find all incoming references to an object or asset — everything that links *to* it.

```bash
rvn backlinks person/freya
rvn backlinks project/website
rvn backlinks assets/pdfs/paper.pdf
```

### `rvn outlinks`

Find all outgoing references from an object — everything it links *to*, including assets referenced with Markdown links or images.

```bash
rvn outlinks project/website
rvn outlinks meeting/kickoff
```

---

## Editing content

### `rvn edit`

Surgical string replacement in vault content files. The target string must appear exactly once in the file. Changes preview by default.

Use `rvn edit` for markdown content such as objects, pages, and daily notes. Do not use it for `raven.yaml`, `schema.yaml`, or template files; those have dedicated command surfaces.

```bash
# Preview a replacement
rvn edit project/website.md "Status: draft" "Status: published"

# Apply it
rvn edit project/website.md "Status: draft" "Status: published" --confirm

# Batch edits via JSON
rvn edit project/website.md --edits-json '{"edits":[{"old_str":"draft","new_str":"published"}]}' --confirm
```

Key flags:
- `--confirm` — apply the edit (default is preview only)
- `--edits-json` — multiple ordered replacements in one call

### `rvn set`

Set frontmatter fields on an existing object. Values are validated against the schema.

```bash
rvn set project/website status=published
rvn set person/freya email=freya@example.com role=lead
rvn set person/freya --fields-json '{"email":"true"}'
```

Use positional `field=value` arguments for shell-friendly literal updates. Use `--fields-json` when you need exact type control, such as preserving the string `"true"` instead of coercing it to a boolean.

For bulk field updates, pipe IDs from a query:

```bash
rvn query 'type:project .status==active' --ids | rvn set --stdin reviewed=true --confirm
rvn query 'type:person' --ids | rvn set --stdin --confirm --fields-json '{"email":"true"}'
```

### `rvn update`

Update a trait's value. Trait IDs come from `rvn query ... --ids`.

```bash
# Get trait IDs first
rvn query 'trait:todo .value==todo' --ids

# Update a specific trait
rvn update daily/2026-03-15.md:trait:0 done

# Bulk update
rvn query 'trait:todo .value==todo' --ids | rvn update --stdin done --confirm

# Bulk update from an explicit trait ID list
rvn update --trait-id daily/2026-03-15.md:trait:0 --trait-id daily/2026-03-16.md:trait:0 done --confirm
```

### `rvn upsert`

Create an object if it does not exist, or update it if it does. Useful for idempotent operations.

```bash
rvn upsert project "Website Redesign" --field status=active
rvn upsert person "Freya" --field email=freya@example.com --content "# Freya\n\nProject lead."
rvn upsert person "Freya" --field-json '{"email":"true"}'
```

Use `--field` for shell-friendly literal values. Use `--field-json` when you need exact type control, such as preserving the string `"true"` instead of coercing it to a boolean.

Key flags:
- `--field` — set field values
- `--field-json` — set fields as a JSON object
- `--content` — set the markdown body
- `--path` — explicit file path (defaults to slugified title)

---

## Organizing content

### `rvn move`

Move or rename an object or asset. References are updated automatically by default, including Raven wikilinks and Markdown links/images that point at moved assets.

```bash
rvn move inbox/idea project/idea              # Rename/relocate
rvn move project/old-name project/new-name    # Rename
rvn move assets/pdfs/draft.pdf assets/pdfs/final.pdf
```

Asset destinations must include a file extension. Raven treats non-Markdown moves as asset moves and keeps the asset index in sync.

Key flags:
- `--update-refs` — update all references to the moved file (default: true)
- `--force` — skip confirmation
- `--stdin` — bulk move from piped IDs

### `rvn reclassify`

Change an object's type. Raven updates frontmatter, applies defaults for the new type, and optionally moves the file to the new type's default directory.

```bash
rvn reclassify inbox/meeting-notes meeting --field name="Q1 Planning"
rvn reclassify person/freya company --field-json '{"legal_name":"false"}'
rvn reclassify page/scratch note --field title="Research Notes" --no-move
```

Key flags:
- `--field` — supply field values for the new type using Raven field literals
- `--field-json` — supply exact typed field values as JSON
- `--no-move` — keep the file in its current location
- `--update-refs` — update references if the file moves (default: true)
- `--force` — skip confirmation for dropped fields

### `rvn delete`

Remove an object. Files are moved to `.trash/` by default.

```bash
rvn delete project/old-project
rvn delete project/old-project --force         # Skip confirmation
rvn delete project/old-project --json          # Preview in CLI JSON mode
rvn delete project/old-project --confirm --json
```

Check backlinks before deleting to avoid broken references:

```bash
rvn backlinks project/old-project
```

---

## Validating content

### `rvn check`

Validate managed vault files against the schema and asset graph. Reports issues like unknown types, missing required fields, broken references, missing assets, orphaned assets, non-canonical asset locations, and undefined traits. Paths matched by `raven.yaml` `exclude` patterns are not checked.

```bash
rvn check                                       # Entire vault
rvn check project/website                       # One file or directory
rvn check --type project                         # Only project-type objects
rvn check --issues missing_reference,unknown_type  # Specific issue types
rvn check --by-file                              # Group output by file
```

Auto-fix capabilities:

```bash
rvn check fix                                    # Preview auto-fixes
rvn check fix --confirm                          # Apply fixes
rvn check create-missing                         # Preview creating missing referenced pages
rvn check create-missing --confirm               # Create them
```

`rvn check fix` handles these unambiguous fixes:

- **`short_ref_could_be_full_path`** — replace short refs with their canonical full path
- **`invalid_enum_value`** — remove unnecessary quotes around enum trait values when the unquoted value is valid
- **`non_canonical_ref`** — strip the configured root prefix from wikilink targets (e.g. `[[type/person/freya]]` → `[[person/freya]]`)
- **`non_canonical_path`** — move files into the configured directory root for their type and rewrite all references that point at them

Asset-related issues are reported by `rvn check`, but are not auto-fixed by `rvn check fix` in this release. Use `rvn move` to relocate assets so references are rewritten safely.

Key flags:
- `--type` / `-t` — check only objects of a specific type
- `--issues` — only report specific issue types (comma-separated)
- `--exclude` — exclude specific issue types
- `--strict` — treat warnings as errors
- `--fix` — preview/apply safe auto-fixes for unambiguous check issues
- `create-missing` — preview/create pages for unresolved references
- `--verbose` / `-V` — full details for every issue

### `rvn resolve`

Debug reference resolution. Shows how Raven resolves a reference string to an object or asset ID.

```bash
rvn resolve freya                                # Short name
rvn resolve "The Queen"                          # Alias
rvn resolve 2026-03-15                           # Date reference
rvn resolve project/website#tasks               # Section reference
rvn resolve paper                                # Short asset name if unambiguous
```

Returns whether the reference resolved, the target ID, and the match source (alias, name_field, object_id, short_name, asset path, etc.).

---

## Maintaining the vault

### `rvn reindex`

Rebuild the SQLite index from managed vault files, including Markdown objects and assets under the configured asset root. Paths matched by `raven.yaml` `exclude` patterns are skipped and removed from the index during incremental reindexing. Normally Raven reindexes automatically after commands (`auto_reindex: true` in `raven.yaml`). Manual reindexing is needed after:

- Editing files outside of Raven (e.g., in your editor or with git)
- Adding, moving, or deleting assets outside of Raven
- Changing asset root or kind rules in `raven.yaml`
- Schema changes that affect indexing
- Recovering from index corruption

```bash
rvn reindex                                      # Incremental (changed files only)
rvn reindex --full                               # Complete rebuild
rvn reindex --dry-run                            # Show what would be reindexed
```

---

## Related docs

- `querying/query-language.md` — full RQL syntax for complex queries
- `using-your-vault/assets.md` — organizing and referencing non-Markdown files
- `vault-management/bulk-operations.md` — `--apply` and `--ids` piping for bulk changes
- `vault-management/import.md` — bulk importing from JSON
- `types-and-traits/references.md` — reference syntax, resolution, and maintenance
- `using-your-vault/configuration.md` — `raven.yaml` and `config.toml` reference
