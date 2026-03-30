# References

References are wiki-style links that connect objects across your vault into a graph. They power navigation, backlinks, query predicates like `refs(...)` and `refd(...)`, and automatic link updates when you move files.

## Syntax

| Format | Description | Example |
|--------|-------------|---------|
| `[[target]]` | Basic reference | `[[people/freya]]` |
| `[[target\|display]]` | Reference with display text | `[[people/freya\|Freya]]` |
| `[[target#fragment]]` | Reference to a section or embedded object | `[[projects/website#tasks]]` |
| `[[YYYY-MM-DD]]` | Date reference (resolves to daily note) | `[[2026-03-15]]` |

## Where references can appear

References work in three places:

**Markdown body content** (most common):

```markdown
Met with [[people/freya]] about [[projects/website]].
```

**Frontmatter `ref` / `ref[]` fields** (bare IDs, no brackets needed):

```yaml
---
type: project
owner: people/freya
collaborators:
  - people/freya
  - people/thor
---
```

**Embedded type declarations**:

```markdown
## Weekly Standup
::meeting(attendees=[[[people/freya]], [[people/thor]]])
```

## Resolution

When Raven encounters a reference, it resolves it to a canonical object ID by checking these match sources in order:

1. **Alias** — the `alias` frontmatter field
2. **Name field** — the type's `name_field` value (e.g., `title`, `name`)
3. **Date** — absolute `YYYY-MM-DD` patterns resolve to daily notes
4. **Object ID / path** — full or suffix path match
5. **Short name** — the last segment of an object ID (e.g., `freya` for `people/freya`)

If multiple candidates match, the reference is ambiguous and is not resolved automatically.

### Short references

When a short name uniquely identifies one object, you can use it without the full path:

```markdown
[[freya]]       → people/freya (if only one "freya" exists)
[[website]]     → projects/website
[[2026-03-15]]  → daily/2026-03-15
```

When short names collide (e.g., `projects/notes` and `meetings/notes`), use the full path to disambiguate:

```markdown
[[projects/notes]]    → unambiguous
[[meetings/notes]]    → unambiguous
[[notes]]             → ambiguous (not resolved)
```

Use `rvn resolve` to debug resolution and `rvn check` to find ambiguous references across the vault.

## Backlinks and outlinks

Backlinks are incoming references — everything that links *to* an object. Outlinks are outgoing references — everything an object links *to*.

```bash
rvn backlinks people/freya
```

```text
meeting/kickoff.md
  [[people/freya]] wants the initial scope confirmed

projects/website.md
  Project lead: [[people/freya]]
```

```bash
rvn outlinks projects/website
```

```text
people/freya (frontmatter: owner)
people/thor (body)
companies/acme (body)
```

Backlinks are a powerful way to see how an object is used across the vault without maintaining manual indexes.

## References in queries

RQL has predicates for querying the reference graph:

```bash
# Objects that reference a target
rvn query 'object:meeting refs([[people/freya]])'
rvn query 'object:meeting refs(object:project .status==active)'

# Objects referenced by a source
rvn query 'object:project refd(object:meeting)'

# Traits on lines that reference a target
rvn query 'trait:todo refs([[people/freya]])'
```

| Predicate | Meaning |
|-----------|---------|
| `refs(...)` | Object/trait references a target or query match |
| `refd(...)` | Object is referenced by a source or query match (object queries only) |

`refs()` accepts nested queries, wiki-links, or bare target shorthand. See `querying/query-language.md` for the full syntax.

## Reference maintenance

### Automatic updates on move

`rvn move` updates all references to a moved file by default:

```bash
rvn move people/freya people/freya-odinsdottir
# All [[people/freya]] references are updated to [[people/freya-odinsdottir]]
```

Disable with `--update-refs=false` if needed.

### Finding broken references

```bash
rvn check --issues missing_reference
rvn check --issues ambiguous_reference,id_collision,alias_collision
```

### Debugging resolution

```bash
rvn resolve "freya" --json           # See match source and target
rvn resolve "The Queen" --json       # Alias resolution
rvn resolve "2026-03-15" --json      # Date resolution
```

## Common patterns

**Linking people to projects:**

```markdown
Project lead: [[people/freya]]
```

Or use a `ref` field in frontmatter:

```yaml
owner: people/freya
```

**Date references in daily notes:**

```markdown
Follow up from [[2026-03-10]].
See also [[2026-03-14]] for context.
```

**Section references:**

```markdown
See the tasks list: [[projects/website#tasks]]
```

## Related docs

- `types-and-traits/file-format.md` — full resolution model, slug generation, and ambiguity handling
- `querying/query-language.md` — `refs()`, `refd()`, and other structural predicates
- `using-your-vault/common-commands.md` — `rvn backlinks`, `rvn outlinks`, `rvn resolve`, `rvn check`
- `types-and-traits/schema.md` — `ref` and `ref[]` field types, `alias` reserved key
