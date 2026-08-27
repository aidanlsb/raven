# File format reference

Raven object files are plain Markdown with optional YAML frontmatter.
Non-Markdown files are ordinary vault files, not Raven objects.

## File structure overview

```markdown
---
type: project
status: active
---

# Website Redesign

Project description...

## Tasks

- @todo Finish homepage

## Standup

Meeting notes...
```

---

## Object IDs

Every object in Raven has a unique ID derived from its location.

### File-level objects

For file-level objects, the ID is derived from the file path (relative to vault root) without the `.md` extension:

| File Path | Object ID |
|-----------|-----------|
| `person/freya.md` | `person/freya` |
| `project/website.md` | `project/website` |
| `random-note.md` | `random-note` |
| `daily/2026-01-10.md` | `2026-01-10` |

**With directory organization** (configured in `raven.yaml` via `directories.type` and `directories.page`):

| File Path | Object ID |
|-----------|-----------|
| `type/person/freya.md` | `person/freya` |
| `type/project/website.md` | `project/website` |
| `page/random-note.md` | `random-note` |

The directory prefix (`type/`, `page/`) is stripped from IDs.

**Daily notes** are a special case: the `directories.daily` prefix is always
stripped so the object ID is the bare ISO date, regardless of the configured
daily directory. A file at `daily/2026-01-10.md` (or `journal/2026-01-10.md` with
`directories.daily: journal/`) has the object ID `2026-01-10`. The daily directory
is filesystem layout only and is never part of the daily note's identity.

### Sections

Sections are Markdown heading regions. They are derived from headings during
parsing. They have no frontmatter of their own, and their index entries are
rebuilt on every reindex. Use `rvn section create` to add headings,
`rvn section move` to reorder/reparent a complete subtree, and
`rvn section rename` to change heading identity while rewriting inbound
references. Use preview-first `rvn section delete <file#section>` to inspect and
then remove a heading's complete subtree; apply with `--confirm`. Delete reports
inbound references but leaves them unchanged when no safe replacement exists.
`rvn add` is body-only and rejects heading content. Section IDs combine the file
object ID with a heading-derived fragment:

```
<file-id>#<fragment>
```

| Section | ID |
|---------|-----|
| `## Tasks` in `project/website.md` | `project/website#tasks` |

---

## Slug generation

Slugs are URL-friendly identifiers generated from text.

### Heading slugs (for fragments)

Heading text is converted to a slug for fragment IDs:

1. Convert to lowercase
2. Replace spaces, hyphens, underscores, and colons with single hyphens
3. Remove other special characters
4. Trim trailing hyphens

**Examples:**

| Heading | Slug |
|---------|------|
| `## Tasks` | `tasks` |
| `## Weekly Standup` | `weekly-standup` |
| `## Q1 2026 Review` | `q1-2026-review` |
| `## My Tasks: High Priority` | `my-tasks-high-priority` |
| `## Über Café` | `über-café` (preserves unicode) |

### Unique IDs

When multiple headings would produce the same slug, a numeric suffix is added:

```markdown
## Tasks        → #tasks
## Tasks        → #tasks-2
## Tasks        → #tasks-3
```

### Path slugs (for filenames)

When creating files with `rvn new`, the title is a display name (stored verbatim
in frontmatter) and is slugified into the filename. Path separators and other
characters that are unsafe in paths are handled automatically, so a prose title
never needs a manual `--path`:

| Title | Filename |
|-------|----------|
| `Website Redesign` | `website-redesign.md` |
| `Über Café` | `uber-cafe.md` (normalized) |
| `Q1 2026` | `q1-2026.md` |
| `config.VaultConfig duplicates internal/paths` | `config-vaultconfig-duplicates-internal-paths.md` |

The `/` is slugified into the single filename component rather than treated as a
directory separator. Use `--path` when you want to control the directory and file
name explicitly (there `/` segments do map to directories).

---

## Frontmatter

YAML frontmatter appears at the start of a file between `---` markers:

```markdown
---
type: person
name: Freya
email: freya@asgard.realm
---

# Freya
```

### `type`

Specifies the object type. If omitted, defaults to `page`.

```yaml
---
type: project
---
```

### Reserved keys

These keys are always allowed regardless of type:

| Key | Description |
|-----|-------------|
| `type` | Object type (defaults to `page` if omitted) |
| `alias` | Alternative name for reference resolution |

Object IDs are derived from the file path and cannot be overridden in
frontmatter. There is no `id` key. Use `alias` to give an object an alternate
name for reference resolution. A stray `id:` key is treated as an undeclared
field and reported as `unknown_frontmatter_key` by `rvn check`.

### Field values

Field values in frontmatter follow YAML syntax:

```yaml
---
type: project
title: Website Redesign                  # string
status: active                           # enum value
priority: 3                              # number
due: 2026-02-15                          # date
time: 2026-01-10T09:00                   # datetime
archived: false                          # boolean
owner: person/freya                      # ref (object ID)
tags: [web, frontend, urgent]            # string array
collaborators:                           # ref array
  - person/freya
  - person/thor
---
```

**Datetime normalization:** YAML treats unquoted timestamps as dates/datetimes. Raven preserves
them as dates (`YYYY-MM-DD`) or normalizes datetimes to RFC3339-ish values (e.g.,
`2026-01-10T09:00:00Z`). If you need to preserve the exact literal string, quote the value.

### `alias`

The `alias` reserved key lets any object define an alternative name for
reference resolution (e.g., `alias: The Queen` makes `[[The Queen]]` resolve to
that object). Aliases are matched case-insensitively and in slugified form. See
[Schema reference](schema.md#reserved-keys) for details and examples.

---

## Sections

Every markdown heading creates a section automatically.

```markdown
# Main Title

## Overview
Content here...

### Details
Nested content...
```

This creates:
- File-level object (from frontmatter)
- `section` with ID `file-id#overview`
- `section` with ID `file-id#details`

### Section fields

Sections expose structural fields for queries and navigation:

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | The heading text |
| `level` | number (1-6) | Heading level (`#` = 1, `##` = 2, etc.) |
| `line_start` | number | Heading line |
| `line_end` / `direct_line_end` | number | Direct section end, before the next heading of any level |
| `subtree_line_end` | number | Full subtree end, before the next same-or-higher heading |

### Section hierarchy

Section nesting follows heading levels. A `##` section is a child of the preceding `#` section:

```markdown
# Chapter 1           → parent: file
## Section 1.1        → parent: chapter-1
### Subsection 1.1.1  → parent: section-1-1
## Section 1.2        → parent: chapter-1
```

Use `in(...)`, `within(...)`, `has(...)`, and `contains(...)` predicates to query section scope.

---

## References

Wiki-style links connect objects and sections across your vault:

```markdown
[[person/freya]]                   # Basic reference
[[person/freya|Freya]]             # With display text
[[project/website#tasks]]         # To a section
[[2026-01-10]]                     # Date reference (daily note)
```

Object references can appear in markdown body content and frontmatter `ref`/`ref[]` fields.
Section references must be global (`[[object#fragment]]`); source-relative fragment links like `[[#tasks]]` are not Raven references.

Markdown links and images to non-Markdown files are indexed as outgoing link
edges, not Raven references:

```markdown
[Paper](../files/paper.pdf)
![Diagram](../files/diagram.png)
```

Raven resolves references to canonical IDs through alias, name field, date,
path, and short-name matching. Author references with canonical object IDs
(for example, `[[person/freya]]`). Bare short forms still resolve when
unambiguous, but they are resolution sugar rather than the preferred authoring
form.

For the full resolution model, ambiguity handling, frontmatter ref syntax, and
maintenance commands, see [References](references.md).

---

## Traits

Traits are inline annotations in content:

```markdown
- @due(2026-02-15) Finish homepage design
- @priority(high) Review pull request
- @highlight This is an important insight
- @todo Refactor the auth module
```

### Syntax

| Format | Description |
|--------|-------------|
| `@name` | Boolean trait (presence = true) |
| `@name(value)` | Trait with value |

### Trait position

Traits can appear anywhere on a line:

```markdown
- @due(2026-02-15) Task description
- Task description @due(2026-02-15)
- Task @priority(high) with @due(tomorrow) multiple traits
```

Traits inside inline code spans (`` `like this` ``) are ignored.

### Trait values

| Type | Example |
|------|---------|
| Date | `@due(2026-02-15)`, `@due(tomorrow)` |
| Datetime | `@remind(2026-02-15T09:00)` |
| Enum | `@priority(high)`, `@todo(done)` |
| String | `@note(Remember to follow up)` |
| Boolean | `@highlight` (no value needed) |

### Trait association

Traits are associated with the nearest containing object (the section or file they appear in):

```markdown
## Tasks

- @todo Buy groceries        ← Associated with "file#tasks" section

## Notes

- @highlight Key insight     ← Associated with "file#notes" section
```

Query with `in(...)` (direct scope) or `within(...)` (recursive scope):

```
trait:todo in(section .title=="Tasks")
trait:highlight within(type:project .status==active)
```

---

## Complete example

```markdown
---
type: project
title: Website Redesign
status: active
owner: person/freya
tags: [web, frontend]
---

# Website Redesign

A complete redesign of the company website.

Project lead: [[person/freya]]

## Overview

Goals and objectives...

## Tasks

- @todo Design new homepage
- @todo(done) Set up development environment
- @due(2026-02-01) @priority(high) Finalize color palette

## Weekly Standup

### Agenda

1. Progress update
2. Blockers
3. Next steps

### Notes

@highlight The deadline is firm - no scope changes.

## References

- [[project/brand-guidelines]]
- [[company/acme]]
```

This creates:
- File object: `project/website` (type: `project`)
- Section: `project/website#overview`
- Section: `project/website#tasks`
- Section: `project/website#weekly-standup`
- Section: `project/website#agenda` (parent: weekly-standup)
- Section: `project/website#notes` (parent: weekly-standup)
- Section: `project/website#references`

Plus traits:
- `@todo` on `#tasks`
- `@todo(done)` on `#tasks`
- `@due(2026-02-01)` on `#tasks`
- `@priority(high)` on `#tasks`
- `@highlight` on `#notes`

And references:
- `[[person/freya]]` (in body and frontmatter)
- `[[project/brand-guidelines]]`
- `[[company/acme]]`

## Related docs

- [Core concepts](../getting-started/core-concepts.md) for the shorter mental
  model.
- [Schema reference](schema.md) for field and trait definitions.
- [References](references.md) for resolution, backlinks, and maintenance.
- [Query language](../querying/query-language.md) for section and trait queries.
- [Documentation map](../getting-started/documentation-map.md) for every topic.
