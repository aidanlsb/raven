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
::meeting(time=09:00, attendees=[[[people/freya]], [[people/thor]]])

Meeting notes...
```

---

## Object IDs

Every object in Raven has a unique ID derived from its location.

### File-Level Objects

For file-level objects, the ID is derived from the file path (relative to vault root) without the `.md` extension:

| File Path | Object ID |
|-----------|-----------|
| `people/freya.md` | `people/freya` |
| `projects/website.md` | `projects/website` |
| `random-note.md` | `random-note` |
| `daily/2026-01-10.md` | `daily/2026-01-10` |

**With directory organization** (configured in `raven.yaml`):

| File Path | Object ID |
|-----------|-----------|
| `objects/people/freya.md` | `people/freya` |
| `objects/projects/website.md` | `projects/website` |
| `pages/random-note.md` | `random-note` |

The directory prefix (`objects/`, `pages/`) is stripped from IDs.

### Embedded Objects

Embedded objects (sections and `::type()` declarations) have IDs that combine the file ID with a fragment:

```
<file-id>#<fragment>
```

| Object | ID |
|--------|-----|
| `## Tasks` in `projects/website.md` | `projects/website#tasks` |
| `## Weekly Standup` with `::meeting(...)` | `projects/website#weekly-standup` |
| `## Tasks` with `::section(id=my-tasks)` | `projects/website#my-tasks` |

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
owner: people/freya                      # ref (object ID)
tags: [web, frontend, urgent]            # string array
collaborators:                           # ref array
  - people/freya
  - people/thor
---
```

### `alias`

The `alias` field enables alternative reference resolution. It's a reserved key, so any object can have an alias without needing to declare it in the schema:

```yaml
# people/freya.md
---
type: person
name: Freya
alias: The Queen
---
```

Now `[[The Queen]]` resolves to `people/freya`.

Aliases are matched case-insensitively and also in slugified form (e.g., `[[the-queen]]` also works).

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

The `::type` syntax declares a typed object embedded within a file.

### Syntax

```
::typename                              # shorthand (no fields)
::typename()                            # explicit empty (no fields)
::typename(field=value, field=value, ...) # with fields
```

Must appear on the line **immediately after** a heading:

```markdown
## Weekly Standup
::meeting(time=09:00, attendees=[[[people/freya]]])

Meeting notes go here...
```

### Rules

1. The `::type()` line must be directly after the heading (no blank lines)
2. The heading becomes an object of the specified type (not a `section`)
3. The object ID is `<file-id>#<slug>` where slug comes from the heading text
4. Use `id=custom-id` to override the slug

### Field Value Syntax

| Type | Syntax | Example |
|------|--------|---------|
| String | bare or quoted | `title=Hello`, `title="Hello, World"` |
| Number | bare | `priority=3`, `score=9.5` |
| Boolean | `true`/`false` | `active=true` |
| Date | YYYY-MM-DD | `due=2026-02-15` |
| Datetime | YYYY-MM-DDTHH:MM | `time=2026-01-10T09:00` |
| Reference | `[[id]]` | `owner=[[people/freya]]` |
| Array | `[item, item]` | `tags=[web, frontend]` |
| Ref Array | `[[[id]], [[id]]]` | `attendees=[[[people/freya]], [[people/thor]]]` |

### Examples

**Simple meeting:**

```markdown
## Team Sync
::meeting(time=09:00)
```

**With multiple fields:**

```markdown
## Project Kickoff
::meeting(time=14:00, attendees=[[[people/freya]], [[people/thor]]], important=true)
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

Wiki-style links reference other objects:

```markdown
[[people/freya]]
[[people/freya|Freya]]
[[projects/website#tasks]]
[[2026-01-10]]
```

### Syntax

| Format | Description |
|--------|-------------|
| `[[target]]` | Basic reference |
| `[[target\|display text]]` | Reference with display text |
| `[[target#fragment]]` | Reference to embedded object |

### Resolution

References are resolved in this order:

1. **Alias match** — Reference matches an object's `alias` field
2. **Name field match** — Reference matches an object's `name_field` value
3. **Date match** — `[[YYYY-MM-DD]]` resolves to daily notes
4. **Object ID match** — Reference matches a full object path
5. **Short name match** — Reference matches filename (without path)

**Short references** work when unambiguous:

```markdown
[[freya]]              → people/freya (if only one "freya" exists)
[[website]]            → projects/website
[[2026-01-10]]         → daily/2026-01-10
```

**Ambiguous references** fail with an error listing matches:

```
Reference "notes" is ambiguous: matches [projects/notes, meetings/notes]
```

### References in Frontmatter

Ref fields can use bare IDs or wiki-link syntax:

```yaml
---
owner: people/freya          # Bare ID
owner: "[[people/freya]]"    # Also valid
collaborators:
  - people/freya
  - people/thor
---
```

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

Query with `on:` (direct parent) or `within:` (any ancestor):

```
trait:todo on:{object:section .title=="Tasks"}
trait:highlight within:{object:project .status==active}
```

---

## Complete Example

```markdown
---
type: project
title: Website Redesign
status: active
owner: people/freya
tags: [web, frontend]
---

# Website Redesign

A complete redesign of the company website.

Project lead: [[people/freya]]

## Overview

Goals and objectives...

## Tasks
::section()

- @todo Design new homepage
- @todo(done) Set up development environment
- @due(2026-02-01) @priority(high) Finalize color palette

## Weekly Standup
::meeting(time=09:00, attendees=[[[people/freya]], [[people/thor]]])

### Agenda

1. Progress update
2. Blockers
3. Next steps

### Notes

@highlight The deadline is firm - no scope changes.

## References

- [[projects/brand-guidelines]]
- [[companies/acme]]
```

This creates:
- File object: `projects/website` (type: `project`)
- Section: `projects/website#overview` (type: `section`)
- Section: `projects/website#tasks` (type: `section`, from `::section()`)
- Embedded object: `projects/website#weekly-standup` (type: `meeting`)
- Section: `projects/website#agenda` (type: `section`, parent: weekly-standup)
- Section: `projects/website#notes` (type: `section`, parent: weekly-standup)
- Section: `projects/website#references` (type: `section`)

Plus traits:
- `@todo` on `#tasks`
- `@todo(done)` on `#tasks`
- `@due(2026-02-01)` on `#tasks`
- `@priority(high)` on `#tasks`
- `@highlight` on `#notes`

And references:
- `[[people/freya]]` (in body and frontmatter)
- `[[people/thor]]` (in meeting attendees)
- `[[projects/brand-guidelines]]`
- `[[companies/acme]]`
