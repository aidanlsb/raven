# File Format Reference

Raven object files are plain markdown with optional YAML frontmatter. Non-Markdown files under the configured asset root are assets, not object files.

## File Structure Overview

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

### File-Level Objects

For file-level objects, the ID is derived from the file path (relative to vault root) without the `.md` extension:

| File Path | Object ID |
|-----------|-----------|
| `person/freya.md` | `person/freya` |
| `project/website.md` | `project/website` |
| `random-note.md` | `random-note` |
| `daily/2026-01-10.md` | `daily/2026-01-10` |

**With directory organization** (configured in `raven.yaml` via `directories.type` and `directories.page`):

| File Path | Object ID |
|-----------|-----------|
| `type/person/freya.md` | `person/freya` |
| `type/project/website.md` | `project/website` |
| `page/random-note.md` | `random-note` |

The directory prefix (`type/`, `page/`) is stripped from IDs.

### Asset IDs

Assets are non-Markdown files under `directories.assets` in `raven.yaml`. Asset IDs preserve the vault-relative file path including the extension:

| File Path | Asset ID |
|-----------|----------|
| `assets/pdfs/paper.pdf` | `assets/pdfs/paper.pdf` |
| `assets/photos/diagram.png` | `assets/photos/diagram.png` |

Assets are graph resources, not schema object types. They do not have YAML frontmatter, sections, traits, templates, or user-defined fields. Raven derives asset metadata from the filesystem and index.

### Sections

Sections are immutable Markdown heading regions. Section IDs combine the file object ID with a heading-derived fragment:

```
<file-id>#<fragment>
```

| Section | ID |
|---------|-----|
| `## Tasks` in `project/website.md` | `project/website#tasks` |

---

## Slug Generation

Slugs are URL-friendly identifiers generated from text.

### Heading Slugs (for Fragments)

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

### Path Slugs (for Filenames)

When creating files with `rvn new`, titles are slugified:

| Title | Filename |
|-------|----------|
| `Website Redesign` | `website-redesign.md` |
| `Über Café` | `uber-cafe.md` (normalized) |
| `Q1 2026` | `q1-2026.md` |

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

### Reserved Keys

These keys are always allowed regardless of type:

| Key | Description |
|-----|-------------|
| `type` | Object type (defaults to `page` if omitted) |
| `id` | Explicit object ID override for the file-backed object |
| `alias` | Alternative name for reference resolution |

### Field Values

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

The `alias` reserved key lets any object define an alternative name for reference resolution (e.g., `alias: The Queen` makes `[[The Queen]]` resolve to that object). Aliases are matched case-insensitively and in slugified form. See `types-and-traits/schema.md` (Reserved Keys) for full details and examples.

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

### Section Fields

Sections have two fields:

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | The heading text |
| `level` | number (1-6) | Heading level (`#` = 1, `##` = 2, etc.) |

### Section Hierarchy

Section nesting follows heading levels. A `##` section is a child of the preceding `#` section:

```markdown
# Chapter 1           → parent: file
## Section 1.1        → parent: chapter-1
### Subsection 1.1.1  → parent: section-1-1
## Section 1.2        → parent: chapter-1
```

Use `in(...)`, `within(...)`, `has(...)`, and `contains(...)` predicates to query section scope.

Legacy `::type(...)` lines are treated as ordinary Markdown text. They do not create objects, set section types, override IDs, or add fields.

---

## References

Wiki-style links connect objects, sections, and assets across your vault:

```markdown
[[person/freya]]                   # Basic reference
[[person/freya|Freya]]             # With display text
[[project/website#tasks]]         # To a section
[[2026-01-10]]                     # Date reference (daily note)
[[assets/pdfs/paper.pdf]]          # Asset reference
```

Object references can appear in markdown body content and frontmatter `ref`/`ref[]` fields.

Vault-relative Markdown links and images to non-Markdown files are indexed as asset references:

```markdown
[Paper](assets/pdfs/paper.pdf)
![Diagram](assets/photos/diagram.png)
```

Raven resolves references to canonical IDs through alias, name field, date, path, asset path, and short name matching. Short references like `[[freya]]` or `[[paper]]` work when unambiguous.

For the full resolution model, ambiguity handling, frontmatter ref syntax, and maintenance commands, see `types-and-traits/references.md`.

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

### Trait Position

Traits can appear anywhere on a line:

```markdown
- @due(2026-02-15) Task description
- Task description @due(2026-02-15)
- Task @priority(high) with @due(tomorrow) multiple traits
```

Traits inside inline code spans (`` `like this` ``) are ignored.

### Trait Values

| Type | Example |
|------|---------|
| Date | `@due(2026-02-15)`, `@due(tomorrow)` |
| Datetime | `@remind(2026-02-15T09:00)` |
| Enum | `@priority(high)`, `@todo(done)` |
| String | `@note(Remember to follow up)` |
| Boolean | `@highlight` (no value needed) |

### Trait Association

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

## Complete Example

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
