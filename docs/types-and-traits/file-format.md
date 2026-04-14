# File Format Reference

Raven files are plain markdown with optional YAML frontmatter and optional embedded type declarations.

## File Structure Overview

```markdown
---
type: project
status: active
---

# Website Redesign

Project description...

## Tasks
::section()

- @todo Finish homepage

## Standup
::meeting(time=09:00, attendees=[[[person/freya]], [[person/thor]]])

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

**With directory organization** (configured in `raven.yaml`):

| File Path | Object ID |
|-----------|-----------|
| `objects/person/freya.md` | `person/freya` |
| `objects/project/website.md` | `project/website` |
| `pages/random-note.md` | `random-note` |

The directory prefix (`objects/`, `pages/`) is stripped from IDs.

### Embedded Objects

Embedded objects (sections and `::type()` declarations) have IDs that combine the file ID with a fragment:

```
<file-id>#<fragment>
```

| Object | ID |
|--------|-----|
| `## Tasks` in `project/website.md` | `project/website#tasks` |
| `## Weekly Standup` with `::meeting(...)` | `project/website#weekly-standup` |
| `## Tasks` with `::section(id=my-tasks)` | `project/website#my-tasks` |

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
| `id` | Explicit object ID (primarily for embedded objects) |
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

Every markdown heading creates a `section` object automatically (unless overridden by `::type()`).

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

Use `parent:`, `ancestor:`, `child:`, `descendant:` predicates to query this hierarchy.

---

## Embedded Type Declarations

The `::type` syntax declares a typed item embedded within a file.

### Syntax

```
::typename                              # shorthand (no fields)
::typename()                            # explicit empty (no fields)
::typename(field=value, field=value, ...) # with fields
```

Must appear on the line **immediately after** a heading:

```markdown
## Weekly Standup
::meeting(time=09:00, attendees=[[[person/freya]]])

Meeting notes go here...
```

### Rules

1. The `::type()` line must be directly after the heading (no blank lines)
2. The heading becomes an item of the specified type (not a `section`)
3. The item ID is `<file-id>#<slug>` where slug comes from the heading text
4. Use `id=custom-id` to override the slug

### Field Value Syntax

| Type | Syntax | Example |
|------|--------|---------|
| String | bare or quoted | `title=Hello`, `title="Hello, World"` |
| Number | bare | `priority=3`, `score=9.5` |
| Boolean | `true`/`false` | `active=true` |
| Date | YYYY-MM-DD | `due=2026-02-15` |
| Datetime | YYYY-MM-DDTHH:MM | `time=2026-01-10T09:00` |
| Reference | `[[id]]` | `owner=[[person/freya]]` |
| Array | `[item, item]` | `tags=[web, frontend]` |
| Ref Array | `[[[id]], [[id]]]` | `attendees=[[[person/freya]], [[person/thor]]]` |

### Examples

**Simple meeting:**

```markdown
## Team Sync
::meeting(time=09:00)
```

**With multiple fields:**

```markdown
## Project Kickoff
::meeting(time=14:00, attendees=[[[person/freya]], [[person/thor]]], important=true)
```

**Custom ID:**

```markdown
## Long Heading That Would Make a Verbose Slug
::task(id=task-1, status=todo)
```

This creates ID `file-id#task-1` instead of slugifying the heading.

**Empty declaration (just sets type):**

```markdown
## Design Notes
::section

## Another Section
::section()
```

Both `::section` and `::section()` are equivalent - parentheses are optional when there are no fields.

---

## References

Wiki-style links connect objects across your vault:

```markdown
[[person/freya]]                   # Basic reference
[[person/freya|Freya]]             # With display text
[[project/website#tasks]]         # To a section or embedded object
[[2026-01-10]]                     # Date reference (daily note)
```

References can appear in markdown body content, frontmatter `ref`/`ref[]` fields, and embedded type declarations.

Raven resolves references to canonical IDs through alias, name field, date, path, and short name matching. Short references like `[[freya]]` work when unambiguous.

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

Query with `on(...)` (direct parent) or `within(...)` (any ancestor):

```
trait:todo on(type:section .title=="Tasks")
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
::section()

- @todo Design new homepage
- @todo(done) Set up development environment
- @due(2026-02-01) @priority(high) Finalize color palette

## Weekly Standup
::meeting(time=09:00, attendees=[[[person/freya]], [[person/thor]]])

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
- Section: `project/website#overview` (type: `section`)
- Section: `project/website#tasks` (type: `section`, from `::section()`)
- Embedded object: `project/website#weekly-standup` (type: `meeting`)
- Section: `project/website#agenda` (type: `section`, parent: weekly-standup)
- Section: `project/website#notes` (type: `section`, parent: weekly-standup)
- Section: `project/website#references` (type: `section`)

Plus traits:
- `@todo` on `#tasks`
- `@todo(done)` on `#tasks`
- `@due(2026-02-01)` on `#tasks`
- `@priority(high)` on `#tasks`
- `@highlight` on `#notes`

And references:
- `[[person/freya]]` (in body and frontmatter)
- `[[person/thor]]` (in meeting attendees)
- `[[project/brand-guidelines]]`
- `[[company/acme]]`
