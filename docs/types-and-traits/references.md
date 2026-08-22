# References

References connect Raven objects and sections across your vault. Wiki-style
links are how you point at other notes. They also drive backlinks, query
predicates like `refs(...)` and `refd(...)`, and automatic updates when you
move Markdown objects.

## Syntax

| Format | Description | Example |
|--------|-------------|---------|
| `[[target]]` | Basic reference | `[[person/freya]]` |
| `[[target\|display]]` | Reference with display text | `[[person/freya\|Freya]]` |
| `[[target#fragment]]` | Reference to a section | `[[project/website#tasks]]` |
| `[[YYYY-MM-DD]]` | Date reference (resolves to daily note) | `[[2026-03-15]]` |

## Where references can appear

Object references work in three places:

**Markdown body content** (most common):

```markdown
Met with [[person/freya]] about [[project/website]].
```

**Frontmatter `ref` / `ref[]` fields** (bare IDs, no brackets needed):

```yaml
---
type: project
owner: person/freya
collaborators:
  - person/freya
  - person/thor
---
```

Non-Markdown files use normal Markdown links or images, not Raven references:

```markdown
See [paper](../files/paper.pdf).
![Diagram](../files/system.png)
```

## Resolution

When Raven encounters a reference, it resolves it to a canonical object or
section ID by checking these match sources:

1. **Alias**: the `alias` frontmatter field
2. **Name field**: the type's `name_field` value (e.g., `title`, `name`)
3. **Date**: absolute `YYYY-MM-DD` patterns resolve to daily notes
4. **Object ID / path**: full or suffix path match
5. **Short name**: the last segment of an object ID (e.g., `freya` for `person/freya`)

If multiple candidates match, the reference is ambiguous and is not resolved automatically.

### Command reference arguments

Commands that act on existing vault content use the positional argument name
`reference`; their MCP contracts use `reference` for one input and usually
`references` for bulk input. The command-specific exceptions are bulk
`add`/`move` (`object_ids`), bulk `update` (`trait_ids`), and `move`'s unchanged
single-item `source`/`destination` pair. Bulk CLI commands read IDs from
`--stdin`, one per line. Use `raven_describe` for the exact MCP array key.

The shared reference grammar accepts canonical object IDs, vault-relative
Markdown paths (with or without `.md`), section IDs (`object#fragment`),
aliases, name-field values, and unambiguous short forms. Date-aware
commands also accept ISO dates and documented dynamic dates such as `today`.
Prefer canonical object and section IDs for automation.

Each command narrows that grammar to the targets it can safely handle:

| Command | Accepted reference scope |
|---------|--------------------------|
| `resolve` | Objects and sections |
| `open` | Objects and sections; also accepts bulk `references` |
| `read` | Managed Markdown files and sections |
| `set`, `unset` | File-level Markdown objects; sections are rejected |
| `delete` | File-backed objects, or explicit non-Markdown file paths; section IDs are rejected in favor of `section delete` |
| `section delete` | Sections only; previews the complete subtree and affected backlinks, then requires `--confirm` |
| `move` | File-backed objects, or explicit non-Markdown file paths; section sources are rejected |
| `reclassify` | File-level Markdown objects; sections are rejected; also accepts bulk `references` |
| `edit` | Managed Markdown files and section subtrees; config, schema, templates, excluded files, and non-Markdown files are rejected |
| `check`, `check fix` | A file, directory, or object reference; omit it to check the entire vault |
| `backlinks` | Objects and sections; also accepts bulk `references` |
| `outlinks` | Objects and sections; also accepts bulk `references` |

### Canonical references

Prefer canonical object IDs when authoring references:

```markdown
[[person/freya]]
[[project/website]]
```

Object-creating commands (`rvn new`, `rvn upsert`, and `rvn daily`) return the
canonical ID as `data.id` in JSON output. Use that value in `[[...]]` links and
`ref` fields. The human CLI prints the same value as `link as <id>`. Do not
derive an ID from the file path: configured directory roots can make the two
different.

Short forms remain supported as resolution sugar for existing content and
interactive input. A bare name such as `[[freya]]` or `[[paper]]` resolves only
when it uniquely identifies one target. It is not the preferred authoring form,
and `rvn check` reports `short_ref_could_be_full_path` when it can replace a
short reference with the canonical ID.

A daily note's canonical object ID is the bare ISO date (`2026-03-15`), so
`[[2026-03-15]]` is already canonical. The
`directories.daily` setting only controls where the file is stored, not the ID.
Legacy references that include the daily directory (`[[daily/2026-03-15]]`) still
resolve to the same daily note as compatibility aliases.

Fields declared as `ref` with `target: date` can store the calendar date
literal (`2026-03-15`), which Raven resolves to the daily note's bare-date object
ID (`2026-03-15`) in indexes and queries. Relative inputs such as `today`,
`tomorrow`, and `yesterday` are normalized to `YYYY-MM-DD` when written through
Raven commands.

Because short forms depend on uniqueness, they become ambiguous when names
collide (e.g., `project/notes` and `meeting/notes`). Canonical IDs remain
explicit:

```markdown
[[project/notes]]    → unambiguous
[[meeting/notes]]    → unambiguous
[[notes]]             → ambiguous (not resolved)
```

This applies equally to a bare name that matches **both** an untyped page and a
typed object. Raven has no page-over-typed (or typed-over-page) preference: if
`freya` names both an untyped page and `person/freya`, the bare reference is
ambiguous. Qualify it to resolve. For example `person/freya` for the typed
object, or the page's own path (`page/freya`) for the page:

```markdown
[[freya]]         → ambiguous (page "freya" vs person/freya)
[[person/freya]]  → person/freya (typed object)
[[page/freya]]    → freya (untyped page)
```

Use `rvn resolve` to debug resolution and `rvn check` to find ambiguous references across the vault.

## Backlinks and outlinks

Backlinks are incoming references to an object or section. Outlinks are Raven
references made by an object.

```bash
rvn backlinks person/freya
```

```text
meeting/kickoff.md
  [[person/freya]] wants the initial scope confirmed

project/website.md
  Project lead: [[person/freya]]
```

```bash
rvn outlinks project/website
```

```text
person/freya (frontmatter: owner)
person/thor (body)
company/acme (body)
```

Backlinks show how an object is used across the vault without you maintaining a separate index.

For bulk graph traversal, pipe references to `--stdin`. Backlinks group JSON results under `items_by_target`; outlinks group JSON results under `items_by_source`.

```bash
rvn query 'type:project .status==active' --ids | rvn backlinks --stdin --json
rvn query 'type:project .status==active' --ids | rvn outlinks --stdin --json
```

## References in queries

RQL has predicates for querying the reference graph:

```bash
# Objects that reference a target
rvn query 'type:meeting refs([[person/freya]])'
rvn query 'type:meeting refs(type:project .status==active)'

# Objects referenced by a source
rvn query 'type:project refd(type:meeting)'

# Traits on lines that reference a target
rvn query 'trait:todo refs([[person/freya]])'
```

| Predicate | Meaning |
|-----------|---------|
| `refs(...)` | Item/trait references a target or query match |
| `refd(...)` | Item is referenced by a source or query match (type queries only) |

`refs()` accepts nested queries, wiki-links, or bare target shorthand. Prefer
canonical IDs in direct targets, especially in saved queries; bare shorthand is
resolution sugar and can become ambiguous as the vault grows. See
`querying/query-language.md` for the full syntax.

## Reference maintenance

### Automatic updates on move

`rvn move` updates all references to a moved file by default:

```bash
rvn move person/freya person/freya-odinsdottir
# All [[person/freya]] references are updated to [[person/freya-odinsdottir]]

rvn move files/downloads/paper.pdf files/paper.pdf
# Indexed Markdown file links/images matched by normalized target path are updated
```

Disable with `--update-refs=false` if needed.

### Finding broken references

```bash
rvn check --issues missing_reference
rvn check --issues broken_file_link  # filesystem only; URLs are not fetched
rvn check --issues ambiguous_reference,id_collision,alias_collision
```

### Referencing something that does not exist yet

Writes are permissive. If you create or edit an object with a reference (a `ref`
field value or a body `[[wikilink]]`) whose target does not exist yet, the write
still succeeds. Link integrity is a vault-health concern, not a write-time error.

When this happens, Raven surfaces the missing target instead of silently leaving it:

- In the interactive CLI (`rvn new`, `rvn upsert`, `rvn set`, `rvn add`, `rvn edit`),
  it prompts to create the missing page(s) right after the write.
- With `--json` (and over MCP), the successful response adds `missing_refs`,
  `missing_ref_items`, and a `REF_TARGET_MISSING` warning per missing target. Each
  warning also carries a structured `create_invoke` (`{command, args}`) alongside the
  `create_command` string so agents can remediate without shell-parsing. This warning
  is distinct from the fatal `REF_NOT_FOUND` error returned when a read/resolve fails.

Create the missing pages later with:

```bash
rvn check create-missing            # interactive
rvn check create-missing --confirm --json   # non-interactive / agents
```

### Debugging resolution

```bash
rvn resolve "person/freya" --json    # Confirm a canonical object ID
rvn resolve "The Queen" --json       # Alias resolution
rvn resolve "2026-03-15" --json      # Date resolution
rvn resolve "paper" --json           # Inspect short-form resolution
```

## Common patterns

**Linking people to projects:**

```markdown
Project lead: [[person/freya]]
```

Or use a `ref` field in frontmatter:

```yaml
owner: person/freya
```

**Date references in daily notes:**

```markdown
Follow up from [[2026-03-10]].
See also [[2026-03-14]] for context.
```

**Section references:**

```markdown
See the tasks list: [[project/website#tasks]]
```

Section fragments are derived from heading text. Create headings with
`rvn section create`, and reorder/reparent a complete section subtree with
`rvn section move`; moving preserves the heading text, level, slug, and
references. To rename a heading without breaking inbound references, use
`rvn section rename project/website#tasks "New Heading"`. It updates the heading
and rewrites every `[[...#tasks]]` reference to the new slug. To remove a
heading and every descendant, preview `rvn section delete
project/website#tasks`; the preview reports every inbound reference that would
become stale, and `--confirm` applies without guessing a replacement for those
references. The file/object `rvn move` command rejects section sources.

## Related docs

- `types-and-traits/file-format.md`: full resolution model, slug generation, and ambiguity handling
- `using-your-vault/file-links.md`: linking and moving non-Markdown files
- `querying/query-language.md`: `refs()`, `refd()`, and other structural predicates
- `using-your-vault/common-commands.md`: `rvn backlinks`, `rvn outlinks`, `rvn resolve`, `rvn check`
- `types-and-traits/schema.md`: `ref` and `ref[]` field types, `alias` reserved key
