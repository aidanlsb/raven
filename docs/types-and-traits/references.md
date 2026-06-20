# References

References connect objects and assets across your vault into a graph. Wiki-style links connect Raven objects and sections; normal Markdown links/images can connect notes to non-Markdown assets. References power navigation, backlinks, query predicates like `refs(...)` and `refd(...)`, and automatic link updates when you move files.

## Syntax

| Format | Description | Example |
|--------|-------------|---------|
| `[[target]]` | Basic reference | `[[person/freya]]` |
| `[[target\|display]]` | Reference with display text | `[[person/freya\|Freya]]` |
| `[[target#fragment]]` | Reference to a section | `[[project/website#tasks]]` |
| `[[YYYY-MM-DD]]` | Date reference (resolves to daily note) | `[[2026-03-15]]` |
| `[text](assets/file.pdf)` | Markdown link to an asset | `[Paper](assets/pdfs/paper.pdf)` |
| `![alt](assets/image.png)` | Markdown image asset | `![Diagram](assets/photos/diagram.png)` |

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

Asset references work in Markdown body content via vault-relative Markdown links/images or Raven wikilinks:

```markdown
See [paper](assets/pdfs/paper.pdf).
![Diagram](assets/photos/system.png)
See also [[assets/pdfs/paper.pdf]].
```

## Resolution

When Raven encounters a reference, it resolves it to a canonical object or asset ID by checking these match sources in order:

1. **Alias** — the `alias` frontmatter field
2. **Name field** — the type's `name_field` value (e.g., `title`, `name`)
3. **Date** — absolute `YYYY-MM-DD` patterns resolve to daily notes
4. **Object ID / path** — full or suffix path match
5. **Asset path** — indexed non-Markdown asset paths such as `assets/pdfs/paper.pdf`
6. **Short name** — the last segment of an object or asset ID (e.g., `freya` for `person/freya`)

If multiple candidates match, the reference is ambiguous and is not resolved automatically.

### Short references

When a short name uniquely identifies one object or asset, you can use it without the full path:

```markdown
[[freya]]       → person/freya (if only one "freya" exists)
[[website]]     → project/website
[[2026-03-15]]  → daily/2026-03-15
[[paper]]       → assets/pdfs/paper.pdf
```

When short names collide (e.g., `project/notes` and `meeting/notes`, or `paper.pdf` and `paper.png`), use the full path to disambiguate:

```markdown
[[project/notes]]    → unambiguous
[[meeting/notes]]    → unambiguous
[[notes]]             → ambiguous (not resolved)
```

Use `rvn resolve` to debug resolution and `rvn check` to find ambiguous references across the vault.

## Backlinks and outlinks

Backlinks are incoming references — everything that links *to* an object or asset. Outlinks are outgoing references — everything an object links *to*.

```bash
rvn backlinks person/freya
rvn backlinks assets/pdfs/paper.pdf
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

Backlinks are a powerful way to see how an object is used across the vault without maintaining manual indexes.

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
rvn query 'type:page refs([[assets/pdfs/paper.pdf]])'

# Objects referenced by a source
rvn query 'type:project refd(type:meeting)'

# Traits on lines that reference a target
rvn query 'trait:todo refs([[person/freya]])'
```

| Predicate | Meaning |
|-----------|---------|
| `refs(...)` | Item/trait references a target or query match |
| `refd(...)` | Item is referenced by a source or query match (type queries only) |

`refs()` accepts nested queries, wiki-links, or bare target shorthand. See `querying/query-language.md` for the full syntax.

## Reference maintenance

### Automatic updates on move

`rvn move` updates all references to a moved file by default:

```bash
rvn move person/freya person/freya-odinsdottir
# All [[person/freya]] references are updated to [[person/freya-odinsdottir]]

rvn move assets/downloads/paper.pdf assets/pdfs/paper.pdf
# Markdown links/images and wikilinks pointing at the asset are updated
```

Disable with `--update-refs=false` if needed.

### Finding broken references

```bash
rvn check --issues missing_reference
rvn check --issues missing_asset
rvn check --issues ambiguous_reference,id_collision,alias_collision
```

### Referencing something that does not exist yet

Writes are permissive. If you create or edit an object with a reference (a `ref`
field value or a body `[[wikilink]]`) whose target does not exist yet, the write
still succeeds — link integrity is a vault-health concern, not a write-time error.

When this happens, Raven surfaces the missing target instead of silently leaving it:

- In the interactive CLI (`rvn new`, `rvn upsert`, `rvn set`, `rvn add`, `rvn edit`),
  it prompts to create the missing page(s) right after the write.
- With `--json` (and over MCP), the successful response adds `missing_refs`,
  `missing_ref_items`, and a `REF_NOT_FOUND` warning per missing target.

Create the missing pages later with:

```bash
rvn check create-missing            # interactive
rvn check create-missing --confirm --json   # non-interactive / agents
```

### Debugging resolution

```bash
rvn resolve "freya" --json           # See match source and target
rvn resolve "The Queen" --json       # Alias resolution
rvn resolve "2026-03-15" --json      # Date resolution
rvn resolve "paper" --json           # Short asset name when unambiguous
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

## Related docs

- `types-and-traits/file-format.md` — full resolution model, slug generation, and ambiguity handling
- `using-your-vault/assets.md` — organizing and referencing non-Markdown files
- `querying/query-language.md` — `refs()`, `refd()`, and other structural predicates
- `using-your-vault/common-commands.md` — `rvn backlinks`, `rvn outlinks`, `rvn resolve`, `rvn check`
- `types-and-traits/schema.md` — `ref` and `ref[]` field types, `alias` reserved key
