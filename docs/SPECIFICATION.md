# Raven

A personal knowledge system with typed blocks, traits, and powerful querying. Built in Go for speed, with plain-text markdown files as the source of truth.

*Named for Odin's ravens Huginn (thought) and Muninn (memory), who gathered knowledge from across the world.*

**CLI**: `rvn`

---

## Table of Contents

1. [Core Concepts](#core-concepts)
2. [Data Model](#data-model)
3. [File Format Specification](#file-format-specification)
4. [Schema Configuration](#schema-configuration)
5. [Syntax Reference](#syntax-reference)
6. [Architecture](#architecture)
7. [Database Schema](#database-schema)
8. [CLI Interface](#cli-interface)
9. [Implementation Phases](#implementation-phases)
10. [Future Enhancements](#future-enhancements)
11. [Design Decisions](#design-decisions)
12. [Technical Notes](#technical-notes)

---

## Core Concepts

### The Four Primitives

| Concept | Purpose | Syntax | Can be Referenced? | Example |
|---------|---------|--------|-------------------|---------|
| **Types** | Define what something *is* | Frontmatter `type:` | Yes, via `[[path/file]]` | person, project, meeting, book |
| **Embedded Types** | A typed section within a file | `::type(id=..., ...)` | Yes, via `[[path/file#id]]` | A meeting inside a daily note |
| **Sections** | Auto-created for every heading | Markdown headings (`#`, `##`, etc.) | Yes, via `[[path/file#slug]]` | Any heading without explicit type |
| **Traits** | Add behavior/metadata to content | `@name` or `@name(value)` | No (queryable, not referenceable) | @due, @priority, @highlight |

### Types vs Sections vs Traits Mental Model

- **Types are nouns** (declared with `::` or frontmatter): A `person` is a thing. A `meeting` is a thing. They exist, have identity, can be linked to.
- **Sections are structural nouns** (auto-created from headings): Every markdown heading becomes a `section` object automatically. This ensures the entire document structure is captured in the object model.
- **Traits are single-valued annotations** (declared with `@`): `@due(2025-02-01)` marks content as having a due date. `@highlight` marks something as important. They modify content, don't create new entities. Multiple traits can be combined: `@due(2025-02-01) @priority(high)`.

### Built-in Types

| Type | Purpose | Auto-created? |
|------|---------|---------------|
| `page` | Fallback for files without explicit type | Yes, when no type declared |
| `section` | Fallback for headings without explicit type | Yes, for every heading without `::type()` |
| `date` | Daily notes (files named `YYYY-MM-DD.md` in daily directory) | Yes, when filename matches date pattern |

**Why built-in?** These types ensure every structural element is represented in the object model. Without them, files and headings without explicit types would have no type, breaking queries and parent resolution.

**The `date` type is special**:
- Files named `YYYY-MM-DD.md` (e.g., `2025-02-01.md`) in the configured `daily_directory` are automatically type `date`
- This is a controlled exception to "frontmatter is the only source of truth" because dates are a foundational concept
- Date references use shorthand syntax: `[[2025-02-01]]` resolves to the daily note for that date
- All date-typed fields are indexed for temporal queries (`today`, `this-week`, `overdue`)
- The daily directory is configured in `raven.yaml` (default: `daily/`)

**Not in schema.yaml**: These types are added programmatically and don't need to appear in your `schema.yaml`.

**Customizable**: You can add `page` or `section` definitions to your `schema.yaml` to extend them with custom fields. The `date` type is locked and cannot be modified—use traits for additional daily note metadata (e.g., `@mood(great)`).

### ⚠️ Files as Source of Truth (Critical Design Principle)

**Plain-text markdown files are the ONLY source of truth. The database is disposable.**

| Component | Role | Can be deleted? |
|-----------|------|-----------------|
| `*.md` files | **Source of truth** - all user data | NO - this IS your data |
| `schema.yaml` | **Source of truth** - type/trait definitions | NO - defines your structure |
| `raven.yaml` | **Source of truth** - vault configuration | NO - defines your settings |
| `.raven/index.sqlite` | **Derived cache** - for fast queries only | YES - rebuilt by `rvn reindex` |

**What this means:**
- Delete `.raven/`, run `rvn reindex`, everything is restored
- Files can be edited with any text editor
- Files sync via Dropbox/iCloud/git without conflicts (`.raven/` is local-only, gitignore it)
- The database NEVER contains data that doesn't exist in your text files
- If anything goes wrong with the database, delete it and reindex

**This is non-negotiable.** It's what makes Raven portable, trustworthy, and future-proof. Your notes will outlive this tool.

---

## Data Model

### Objects (Types)

Objects are referenceable entities. They come in two forms:

#### File-Level Objects
The file itself represents the object. Type declared in frontmatter.

```
people/alice.md  →  Object(id="people/alice", type="person", ...)
```

**Object ID**: The file path without extension (e.g., `people/alice`).

#### Embedded Objects (Explicit Types)
A section within a file can be explicitly typed with `::type()` on the line after a heading.

```
daily/2025-02-01.md
  └── ## Weekly Standup     →  Object(id="daily/2025-02-01#standup", type="meeting", ...)
        ::meeting(id=standup, ...)
```

**Object ID**: The file path + `#` + explicit ID (e.g., `daily/2025-02-01#standup`). The `id` field is **required** for explicitly typed embedded objects.

#### Sections (Auto-Created from Headings)
Every markdown heading automatically becomes a `section` object, even without an explicit `::type()` declaration.

```
daily/2025-02-01.md
  └── ## Morning            →  Object(id="daily/2025-02-01#morning", type="section", ...)
  └── ## Weekly Standup     →  Object(id="daily/2025-02-01#standup", type="meeting", ...)  # explicit type
  └── ## Afternoon          →  Object(id="daily/2025-02-01#afternoon", type="section", ...)
```

**Section Object ID**: The file path + `#` + slugified heading text (e.g., `daily/2025-02-01#morning`).

**ID Generation Rules**:
1. Heading text is slugified (lowercased, spaces become hyphens, special chars removed)
2. If multiple headings have the same slug, numbers are appended: `#ideas`, `#ideas-2`, `#ideas-3`
3. If heading text is empty, fallback to `#section-{line_number}`

**Section Fields**:
| Field | Type | Description |
|-------|------|-------------|
| `title` | string | The heading text |
| `level` | number | Heading level (1-6) |

**Why sections?** This ensures every structural element in a document is queryable and can have traits attached to it. Traits and references are always parented to the nearest section or explicit type.

### Object Hierarchy

All headings form a tree based on heading levels. Every heading becomes an object (either an explicit type or a section):

```markdown
# Daily Note (file root, type: daily)

## Project Review (type: meeting, parent: daily)
::meeting(id=project-review, time=09:00)

### Website Discussion (type: topic, parent: meeting)
::topic(id=website-discussion, project=[[projects/website]])

### Mobile App Discussion (type: topic, parent: meeting)
::topic(id=mobile-discussion, project=[[projects/mobile]])

## Random Notes (type: section, parent: daily, auto-created)
```

**Hierarchy Rules**:
- A heading becomes a child of the nearest ancestor heading with a lower level
- If no ancestor exists, parent is the file root
- Explicit types (`::type()`) take precedence over auto-created sections
- The `::type()` must appear on the line immediately after the heading

**Nesting limit**: Standard markdown heading depth (H1-H6). The `rvn check` command validates nesting doesn't exceed limits.

**Parent Resolution for Traits/Refs**: Traits and references are assigned to the object (section or explicit type) that contains them based on line numbers.

### Traits

Traits are annotations that attach metadata to content. They are:
- **Queryable**: Find all tasks due this week
- **Not referenceable**: You can't link to a specific task
- **Parented**: Every trait belongs to an object (file or embedded)

```markdown
## Weekly Standup
::meeting(id=standup, time=09:00)

- @due(2025-02-03) Send estimate         ← trait, parent is the meeting
- Regular bullet point                    ← just content, not a trait

## Random Notes

- @due(2025-02-05) @priority(high) Task  ← two traits, parent is the file root
```

**Trait content**: The content associated with a trait is everything on the same line(s) between carriage returns (i.e., the line or paragraph containing the trait annotation).

### References

References create links between objects:

```markdown
Met with [[people/alice]] about [[projects/website]].
```

Both outgoing refs (what this note links to) and backlinks (what links to this note) are indexed.

**Reference resolution**:
- Use full path for clarity: `[[people/alice]]`
- Short names allowed if unambiguous: `[[alice]]` works if only one `alice` exists
- Embedded objects: `[[daily/2025-02-01#standup]]`
- The `rvn check` command warns about ambiguous short references

**Slugified matching**: References are matched using slugification, so `[[people/Emily Jia]]` will resolve to `people/emily-jia.md`. This allows natural inline references while maintaining clean, URL-safe filenames.

**Validation**: All references must resolve to existing objects. The `rvn check` command errors on broken references.

---

## File Format Specification

### Frontmatter (File-Level Metadata)

YAML frontmatter defines the file's type and fields:

```markdown
---
type: person
name: Alice Chen
email: alice@example.com
---

# Alice Chen

Content here...
```

**Rules**:
- Frontmatter is optional
- If present, must be valid YAML between `---` markers
- The `type` field determines the object type
- Other fields are validated against the schema

### Embedded Types

Declared with `::type()` on a heading:

```markdown
## Meeting Title
::meeting(id=team-sync, time=2025-02-01T09:00, attendees=[[[people/alice]], [[people/bob]]])

Content of the meeting...
```

**Rules**:
- `::type()` must appear on the line immediately after the heading (or within 2 lines)
- First argument is the type name
- The `id` field is **required** and must be unique within the file
- Additional arguments are `key=value` field assignments
- Scope extends from this heading to the next heading at same or higher level
- Full object ID = `file-path#id` (e.g., `daily/2025-02-01#team-sync`)

**Why `::` instead of `@`?** The `::` prefix distinguishes type declarations (which create referenceable objects) from trait annotations (which add metadata to content). This makes the syntax visually distinct and unambiguous.

### Traits

**Traits are single-valued annotations** using `@name` or `@name(value)` syntax:

```markdown
- @due(2025-02-01) @priority(high) Complete the report
- @remind(2025-02-05T09:00) Follow up on this
- @highlight This is an important insight
```

**Rules**:
- Each trait has at most one value (or no value for boolean traits)
- Traits can appear anywhere in content
- The annotated content is the text on the same line (after all traits)
- Boolean traits have no value: `@highlight`, `@pinned`, `@archived`
- Valued traits take a single argument: `@due(2025-02-01)`, `@priority(high)`
- Multiple traits can be combined: `@due(2025-02-01) @priority(high) @status(todo)`
- Undefined traits (not in `schema.yaml`) trigger a warning during `rvn check`

**"Tasks" are emergent**: Instead of a composite `@task` trait, use atomic traits. Anything with `@due` or `@status` is effectively a "task". Use saved queries to define what "tasks" means in your workflow.

### References

Wiki-style links:

```markdown
[[people/alice]]               # Full path reference (preferred)
[[alice]]                      # Short reference (if unambiguous)
[[alice|Alice Chen]]           # Reference with display text
[[daily/2025-02-01#standup]]   # Reference to embedded object
```

**Resolution rules**:
1. If path contains `/`, treat as full path from vault root
2. If short name, search for unique match across vault
3. If ambiguous (multiple matches), `rvn check` warns and requires full path
4. All references must resolve to existing objects (validated during check)

### Tags

Tags provide lightweight categorization for objects using `#hashtag` syntax.

### Behavior

Tags attach to the **object** (file or embedded type), not to a specific line. All tags found within an object's content are aggregated and stored as metadata on that object.

**Tag inheritance**: Tags from child embedded objects are also inherited by parent objects.

```markdown
---
type: daily
date: 2025-02-01
tags: [work]                   # Tags can also be declared in frontmatter
---

# February 1, 2025

Some thoughts about #productivity today.

## Weekly Standup
::meeting(id=standup, time=09:00)

Discussed #planning and #roadmap items.

## Evening

Read about #productivity and #habits.
```

In this example:
- The `meeting` object gets tags: `["planning", "roadmap"]` (from its section)
- The `daily` object gets tags: `["work", "productivity", "habits", "planning", "roadmap"]` (frontmatter + own content + inherited from children)

### Storage

Tags are stored as a JSON array in the object's `fields`:

```json
{
  "date": "2025-02-01",
  "tags": ["productivity", "habits"]
}
```

### Querying

```bash
# Find all objects with a specific tag
rvn query "tags:productivity"

# Combine with type filter
rvn query "type:daily tags:productivity"

# Multiple tags (AND)
rvn query "tags:productivity tags:habits"
```

### Database Schema Addition

```sql
-- Add index for tag queries (uses JSON extraction)
CREATE INDEX idx_objects_tags ON objects(json_extract(fields, '$.tags'));
```

### Syntax Rules

| Syntax | Valid? | Notes |
|--------|--------|-------|
| `#productivity` | ✓ | Simple tag |
| `#my-tag` | ✓ | Hyphens allowed |
| `#tag_name` | ✓ | Underscores allowed |
| `#123` | ✗ | Numbers-only tags are skipped (avoids issue refs like #123) |
| `#tag123` | ✓ | Tags can contain numbers, just not start with them |
| `#my tag` | ✗ | No spaces (would be `#my` only) |
| `#über` | ✓ | Unicode letters allowed |
| `` `#code` `` | ✗ | Tags inside inline code are ignored |
| Code blocks | ✗ | Tags inside code blocks are ignored |

### Tags vs Traits

| Aspect | Tags | Traits |
|--------|------|--------|
| Syntax | `#name` | `@name` or `@name(value)` |
| Attaches to | Object (aggregated) | Specific line/content |
| Has value | No | Optional (single value) |
| Use case | Categorization | Behavior/metadata |
| Example | `#productivity` | `@due(2025-02-01)`, `@highlight` |

### Implementation Notes

1. **Extraction**: Parse `#([\w-]+)` patterns from content, plus `tags:` array from frontmatter
2. **Aggregation**: Collect all tags within an object's scope, plus inherited tags from children
3. **Deduplication**: Store unique tags only
4. **Storage**: Add to object's `fields.tags` as JSON array during indexing

---

## Vault Configuration

### File: `raven.yaml`

Located at vault root. Controls vault-level settings.

```yaml
# Raven Vault Configuration
# These settings control vault-level behavior.

# Where daily notes are stored (default: daily)
daily_directory: daily

# Quick capture settings for 'rvn add'
capture:
  destination: daily      # "daily" (default) or a file path like "inbox.md"
  heading: "## Captured"  # Optional - append under this heading
  timestamp: false        # Prefix with time (default: false, use --timestamp flag)
  reindex: true           # Reindex file after capture (default: true)

# Saved queries - run with 'rvn query <name>'
queries:
  tasks:
    traits: [due, status]
    filters:
      status: "todo,in_progress,"   # Include items without explicit status
    description: "Open tasks"

  overdue:
    traits: [due]
    filters:
      due: past
    description: "Items past due date"

  this-week:
    traits: [due]
    filters:
      due: this-week
    description: "Items due this week"
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `daily_directory` | string | `daily` | Directory for daily notes (files named `YYYY-MM-DD.md`) |
| `queries` | map | `{}` | Saved queries (see below) |

### Saved Queries

Saved queries define reusable trait queries. Run with `rvn query <name>`.

```yaml
queries:
  <name>:
    types: [type1, type2, ...]        # Object types to query (optional)
    traits: [trait1, trait2, ...]     # Traits to query (optional)
    filters:                          # Optional value filters per trait
      trait1: "value"
      trait2: "today"                 # Supports date filters
    description: "Human-readable description"
```

Queries can include types, traits, or both:

```yaml
queries:
  # Query by types only
  people:
    types: [person]
    description: "All people"

  # Query by traits only  
  overdue:
    traits: [due]
    filters:
      due: past
    description: "Overdue items"

  # Mixed: types AND traits
  project-tasks:
    types: [project]
    traits: [due, status]
    description: "Projects with tasks"
```

**Why separate from schema.yaml?** Vault configuration controls *behavior* (where things go, how dates work, what queries exist), while schema defines *structure* (what types and traits exist). Separation keeps each file focused.

---

## Schema Configuration

### File: `schema.yaml`

Located at vault root. Defines all types and traits.

```yaml
version: 2  # Schema format version

# Global trait definitions
# Traits can be used inline (@trait) OR in frontmatter (if type declares them)
traits:
  # Date-related traits
  due:
    type: date

  remind:
    type: datetime

  # Priority/status traits
  priority:
    type: enum
    values: [low, medium, high]
    default: medium

  status:
    type: enum
    values: [todo, in_progress, done, blocked]
    default: todo

  # Boolean/marker traits (no value needed)
  highlight:
    type: boolean

  pinned:
    type: boolean

  archived:
    type: boolean

  # Reference traits
  assignee:
    type: ref

# Type definitions
types:
  # Built-in: Fallback type for files without explicit type
  # page:
  #   fields: {}

  # Built-in: Auto-created for every heading without explicit ::type()
  # section:
  #   fields: { title: string, level: number }

  person:
    default_path: people/
    fields:
      name:
        type: string
        required: true
      email:
        type: string
      company:
        type: ref
        target: company
    traits: [due, priority]  # Optional: person can have these in frontmatter

  project:
    default_path: projects/
    fields:
      lead:
        type: ref
        target: person
      technologies:
        type: string[]
    traits:
      due:
        required: true       # Projects MUST have a due date
      priority:
        default: high        # Override trait default for projects
      status: {}             # Optional, uses trait default

  # Note: 'date' type is built-in for daily notes (YYYY-MM-DD.md)

  meeting:
    fields:
      time:
        type: datetime
      attendees:
        type: ref[]
        target: person
    traits: [remind]  # Meetings can have reminders
      
  book:
    fields:
      title:
        type: string
      author:
        type: ref
        target: person
      rating:
        type: number
        min: 1
        max: 5
    traits:
      status:
        required: true
```

### Traits on Types

Types can declare which traits are valid in their frontmatter using the `traits` field.

**Simple list (all optional):**
```yaml
person:
  fields:
    name: { type: string, required: true }
  traits: [due, priority]  # Person can have @due and @priority in frontmatter
```

**Map with configuration:**
```yaml
project:
  fields:
    lead: { type: ref, target: person }
  traits:
    due:
      required: true    # Project MUST have a due date
    priority:
      default: high     # Override trait default for this type
    status: {}          # Optional, uses trait default
```

**Key rules:**
- Frontmatter can ONLY contain: `type`, `tags`, `id`, declared fields, and declared traits
- Unknown frontmatter keys trigger validation errors
- Inline `@trait` annotations can appear anywhere regardless of type
- Traits in frontmatter apply to the whole object; inline traits apply to specific content
- Both frontmatter traits and inline traits are indexed and queryable

**Example: Frontmatter traits vs inline traits**

```markdown
---
type: project
due: 2025-06-30       # The project itself is due June 30
priority: high
---

# Website Redesign

## Tasks

- @due(2025-03-01) Send proposal       # This specific task is due March 1
- @due(2025-03-15) Review feedback
```

Both dates appear in `rvn trait due`:
- The project's `due: 2025-06-30` (frontmatter trait on the object)
- The tasks' `@due(2025-03-01)` and `@due(2025-03-15)` (inline traits on content)

### Field Types

| Type | Description | Example |
|------|-------------|---------|
| `string` | Plain text | `name: "Alice"` |
| `string[]` | Array of strings | `technologies: [rust, typescript]` |
| `number` | Numeric value | `rating: 4.5` |
| `number[]` | Array of numbers | `scores: [85, 92, 78]` |
| `date` | ISO 8601 date | `due: 2025-02-01` |
| `date[]` | Array of dates | `milestones: [2025-02-01, 2025-03-15]` |
| `datetime` | ISO 8601 datetime | `time: 2025-02-01T09:00` |
| `enum` | One of specified values | `status: active` |
| `bool` | Boolean | `archived: true` |
| `ref` | Reference to another object | `author: [[people/alice]]` |
| `ref[]` | Array of references | `attendees: [[[people/alice]], [[people/bob]]]` |

### Field Properties

| Property | Description |
|----------|-------------|
| `required` | Field must be present (error if missing during check) |
| `default` | Default value if not specified |
| `values` | Allowed values (for enum type) |
| `target` | Target type (for ref types) |
| `min`, `max` | Numeric bounds |
| `derived` | How to compute value (e.g., `from_filename`) |
| `positional` | For traits: value can be first arg without key |

### Reserved Fields

The following field names are reserved and cannot be used in type/trait definitions:

| Field | Purpose |
|-------|---------|
| `id` | Embedded object identifier (required for `::type()`) |
| `type` | Object type name |
| `tags` | Aggregated tags (auto-populated) |

### File Location (default_path)

Types can specify a `default_path` for file creation convenience:

```yaml
types:
  person:
    default_path: people/    # rvn new person creates files here
    fields:
      name:
        type: string
```

**Behavior:**
- `rvn new person "Alice"` creates `people/alice.md`
- `rvn new person "Emily Jia"` creates `people/emily-jia.md` (slugified)
- The directory is created if it doesn't exist
- If no `default_path` is set, files are created in the vault root
- Filenames are always slugified (lowercase, spaces become hyphens)
- The original title is preserved in the file's heading (e.g., `# Emily Jia`)

**Important:** File location has **no effect on type**. A file's type is determined **solely** by its frontmatter `type:` field (or defaults to `page` if not specified). You can have a `person` file anywhere in your vault—Raven doesn't care about directory structure.

**Fallback**: Files without an explicit `type:` field are assigned the `page` type.

---

## Syntax Reference

### Quick Reference

| Syntax | Purpose | Creates Object? |
|--------|---------|-----------------|
| `---`...`---` (frontmatter) | File-level type declaration | Yes |
| `::type(id=..., ...)` | Embedded type declaration | Yes |
| `@trait(...)` | Trait annotation | No |
| `[[path/file]]` | Reference to file object | — |
| `[[path/file#id]]` | Reference to embedded object | — |
| `#tag` | Tag (aggregates to parent object) | — |

### Type Declaration Syntax

**File-level** (YAML frontmatter):
```yaml
---
type: meeting
time: 2025-02-01T09:00
attendees:
  - [[people/alice]]
  - [[people/bob]]
---
```

**Embedded** (inline with `::`):
```
::meeting(id=standup, time=2025-02-01T09:00, attendees=[[[people/alice]], [[people/bob]]])
   │       │           └── key=value field assignments
   │       └── required ID (unique within file)
   └── type name
```

### Trait Annotation Syntax: `@name(key=value, ...)`

```
@due(2025-02-01) @priority(high) Complete the report
  │    │              │     └── trait value
  │    │              └── second trait
  │    └── trait value
  └── trait name
```

### Type Declaration Value Syntax

For embedded type declarations (`::type(...)`), values use key=value pairs:

| Value Type | Syntax | Example |
|------------|--------|---------|
| Simple value | `key=value` | `priority=high` |
| Quoted string | `key="value"` | `title="My Project"` |
| Single ref | `key=[[path]]` | `author=[[people/alice]]` |
| Ref array | `key=[[[a]], [[b]]]` | `attendees=[[[people/alice]], [[people/bob]]]` |
| String array | `key=[a, b, c]` | `technologies=[rust, typescript]` |
| Quoted string array | `key=["a b", "c d"]` | `topics=["Q2 planning", "budget"]` |
| Date | `key=2025-02-01` | `due=2025-02-01` |
| Datetime | `key=2025-02-01T09:00` | `time=2025-02-01T09:00` |

### Trait Value Syntax

Traits use a simpler single-value syntax:

| Trait Type | Syntax | Example |
|------------|--------|---------|
| Boolean | `@name` | `@highlight`, `@pinned` |
| Date | `@name(YYYY-MM-DD)` | `@due(2025-02-01)` |
| Datetime | `@name(YYYY-MM-DDTHH:MM)` | `@remind(2025-02-01T09:00)` |
| Enum/String | `@name(value)` | `@priority(high)`, `@status(todo)` |
| Reference | `@name([[path]])` | `@assignee([[people/alice]])` |

**Note:** Unlike type declarations, traits take a single positional value only. Use multiple traits instead of multiple fields: `@due(2025-02-01) @priority(high)`

### Complete Example

```markdown
---
type: date
tags: [work]
---

# Saturday, February 1, 2025

Morning coffee, reviewed [[projects/website-redesign]].

## Weekly Standup
::meeting(id=standup, time=09:00, attendees=[[[people/alice]], [[people/bob]]], recurring=[[meetings/weekly-standup]])

Discussed Q2 roadmap. [[people/alice]] raised concerns about timeline.

- @due(2025-02-03) @assignee([[people/alice]]) Send revised estimate
- Agreed to revisit next week
- @highlight Key insight: we need more buffer time

## 1:1 with Bob
::meeting(id=one-on-one-bob, time=14:00, attendees=[[[people/bob]]])

Talked about his career growth.

- @due(2025-02-10) Write up promotion case
- He's interested in the tech lead role on [[projects/mobile-app]]

## Reading

Started [[books/atomic-habits]] by [[people/james-clear]].

- @highlight Habits are compound interest for self-improvement #productivity
- @due(2025-02-15) Finish chapter 3

## Random Thoughts

- @remind(2025-02-02T10:00) Check if designs are ready
- Need to revisit my #productivity system
```

**Object IDs in this example**:
- File: `daily/2025-02-01`
- Embedded: `daily/2025-02-01#standup`, `daily/2025-02-01#one-on-one-bob`

---

## Architecture

### Directory Structure

```
~/.config/raven/
└── config.toml              # Global app configuration

~/vault/                      # Your notes (synced to cloud)
├── schema.yaml              # Type/trait definitions
├── .raven/
│   └── index.db             # SQLite index (NOT synced, .gitignore it)
├── daily/
│   └── 2025-02-01.md
├── people/
│   └── alice.md
├── projects/
│   └── website.md
└── books/
    └── atomic-habits.md
```

### App Configuration: `~/.config/raven/config.toml`

The global config file specifies the default vault and preferences:

```toml
# Default vault path (required for commands without --vault)
vault = "/Users/you/Dropbox/vault"

# Editor for opening files (defaults to $EDITOR)
editor = "code"
```

**Config Resolution**:
1. Check `~/.config/raven/config.toml` (XDG-style, preferred)
2. Fall back to OS-specific config dir (`~/Library/Application Support/raven/` on macOS)

**Vault Resolution** (in order):
1. `--vault` CLI flag (always wins)
2. `vault` in config file
3. Error if neither specified (no fallback to current directory for safety)

### Security: Vault Scoping

**All operations are strictly scoped to the vault directory**. The CLI will never read, write, or traverse files outside the configured vault.

**Protections**:
- Symlinks are not followed during directory traversal
- Canonical path validation ensures files are within the vault
- Path traversal attacks (e.g., `../../../etc/passwd`) are blocked
- The `rvn init` command is the only operation that can create files at an arbitrary path (user-specified)

**Implementation**:
```go
// Walk configured to stay within vault
filepath.WalkDir(vaultPath, func(path string, d fs.DirEntry, err error) error {
    // Skip symlinks
    if d.Type()&os.ModeSymlink != 0 {
        return nil
    }
    
    // Canonical path validation
    canonicalFile, _ := filepath.EvalSymlinks(path)
    if !strings.HasPrefix(canonicalFile, canonicalVault) {
        return nil  // Skip files outside vault
    }
    // ...
})
```

### Code Structure

```
cmd/
└── rvn/
    └── main.go              # Entry point, Cobra root command

internal/
├── commands/
│   └── registry.go          # Command metadata registry (single source of truth)
├── config/
│   ├── config.go            # Load ~/.config/raven/config.toml (global config)
│   └── vault.go             # Load raven.yaml (vault config)
├── schema/
│   ├── types.go             # Schema type definitions
│   ├── loader.go            # Load schema.yaml
│   └── validator.go         # Validate fields against schema
├── parser/
│   ├── frontmatter.go       # Parse YAML frontmatter
│   ├── markdown.go          # Parse markdown structure (goldmark)
│   ├── typedecl.go          # Parse ::type() declarations
│   ├── traits.go            # Parse @trait() annotations
│   ├── refs.go              # Extract [[references]] and #tags
│   └── document.go          # Combine into ParsedDocument
├── resolver/
│   └── resolver.go          # Resolve short refs to full paths
├── index/
│   ├── database.go          # SQLite operations
│   ├── queries.go           # Query builder
│   └── dates.go             # Date filter parsing
├── check/
│   └── validator.go         # Vault-wide validation (rvn check)
├── pages/
│   └── create.go            # Consolidated page creation logic
├── vault/
│   ├── dates.go             # Date parsing utilities
│   ├── walk.go              # Markdown file walking
│   └── editor.go            # Open files in editor
├── audit/
│   └── audit.go             # Audit log operations
├── mcp/
│   ├── server.go            # MCP server (JSON-RPC over stdin/stdout)
│   └── tools.go             # Generate MCP tools from command registry
└── cli/
    ├── root.go              # Cobra root command setup
    ├── results.go           # Shared JSON result types
    ├── json.go              # JSON output helpers
    ├── errors.go            # Error codes and handling
    ├── init.go              # rvn init
    ├── reindex.go           # rvn reindex
    ├── check.go             # rvn check
    ├── trait.go             # rvn trait
    ├── type.go              # rvn type
    ├── tag.go               # rvn tag
    ├── query.go             # rvn query
    ├── backlinks.go         # rvn backlinks
    ├── stats.go             # rvn stats
    ├── untyped.go           # rvn untyped
    ├── daily.go             # rvn daily
    ├── date.go              # rvn date
    ├── new.go               # rvn new
    ├── add.go               # rvn add
    ├── set.go               # rvn set
    ├── read.go              # rvn read
    ├── delete.go            # rvn delete
    ├── schema.go            # rvn schema (introspection)
    ├── schema_edit.go       # rvn schema add (modification)
    └── serve.go             # rvn serve (MCP server)
```

### Command Registry

The **command registry** (`internal/commands/registry.go`) is the single source of truth for all CLI commands:

- Each command has metadata: name, description, args, flags, examples
- MCP tools are auto-generated from this registry
- `rvn schema commands` reads from this registry
- This ensures CLI and MCP tool schemas never diverge

When adding a new command:
1. Add metadata to `internal/commands/registry.go`
2. Create the Cobra command handler in `internal/cli/`
3. The MCP tool becomes automatically available

---

## Database Schema

### SQLite Tables

```sql
-- All referenceable objects (files + embedded types)
CREATE TABLE objects (
    id TEXT PRIMARY KEY,              -- Full path (file) or path#id (embedded)
    file_path TEXT NOT NULL,          -- Which file it lives in
    type TEXT NOT NULL,               -- person, meeting, project, page (fallback), etc.
    heading TEXT,                     -- NULL for file-level, heading text for embedded
    heading_level INTEGER,            -- NULL for file-level
    fields TEXT NOT NULL DEFAULT '{}', -- JSON of all field values (including tags)
    line_start INTEGER NOT NULL,      -- Line number where object starts
    line_end INTEGER,                 -- Line number where object ends (embedded only)
    parent_id TEXT,                   -- Parent object ID (for nested embedded)
    created_at INTEGER,
    updated_at INTEGER
);

-- All trait annotations
CREATE TABLE traits (
    id TEXT PRIMARY KEY,              -- Generated ID
    file_path TEXT NOT NULL,
    parent_object_id TEXT NOT NULL,   -- Which object this belongs to
    trait_type TEXT NOT NULL,         -- task, remind, highlight, etc.
    content TEXT NOT NULL,            -- The annotated text (line/paragraph)
    fields TEXT NOT NULL DEFAULT '{}', -- JSON of all field values
    line_number INTEGER NOT NULL,
    created_at INTEGER
);

-- References between objects
CREATE TABLE refs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id TEXT NOT NULL,          -- Object or trait that contains the ref
    target_id TEXT NOT NULL,          -- Resolved target object ID
    target_raw TEXT NOT NULL,         -- Original reference text (for display)
    display_text TEXT,                -- Optional display text
    file_path TEXT NOT NULL,
    line_number INTEGER,
    position_start INTEGER,
    position_end INTEGER
);

-- Date index for temporal queries
CREATE TABLE date_index (
    date TEXT NOT NULL,               -- YYYY-MM-DD
    source_type TEXT NOT NULL,        -- 'object' or 'trait'
    source_id TEXT NOT NULL,          -- Object or trait ID
    field_name TEXT NOT NULL,         -- Which field (due, date, start, etc.)
    file_path TEXT NOT NULL,
    PRIMARY KEY (date, source_type, source_id, field_name)
);

-- Indexes for fast queries
CREATE INDEX idx_objects_file ON objects(file_path);
CREATE INDEX idx_objects_type ON objects(type);
CREATE INDEX idx_objects_parent ON objects(parent_id);

CREATE INDEX idx_traits_file ON traits(file_path);
CREATE INDEX idx_traits_type ON traits(trait_type);
CREATE INDEX idx_traits_parent ON traits(parent_object_id);

-- JSON field indexes for common queries
CREATE INDEX idx_traits_status ON traits(json_extract(fields, '$.status'));
CREATE INDEX idx_traits_due ON traits(json_extract(fields, '$.due'));
CREATE INDEX idx_objects_tags ON objects(json_extract(fields, '$.tags'));

CREATE INDEX idx_refs_source ON refs(source_id);
CREATE INDEX idx_refs_target ON refs(target_id);
CREATE INDEX idx_refs_file ON refs(file_path);

CREATE INDEX idx_date_index_date ON date_index(date);
CREATE INDEX idx_date_index_file ON date_index(file_path);
```

**Design notes**:
- All trait-specific fields (like `status`, `due`, `priority`) are stored in the `fields` JSON blob
- Common query patterns use JSON indexes for performance
- The `type` column is NOT NULL since all objects have a type (fallback to `page`)
- Object IDs include full path for uniqueness

---

## CLI Interface

### Commands

```bash
# Initialize a new vault
rvn init <path>

# Validate vault (compile step)
rvn check
rvn check --strict              # Treat warnings as errors
rvn check --create-missing      # Interactively create missing referenced pages

# Reindex all files
rvn reindex

# Query traits (generic form)
rvn trait <name> [filters]
rvn trait task                           # All tasks
rvn trait task --status todo             # Filter by field
# Query traits by type
rvn trait due                            # All items with @due
rvn trait due --value today              # Items due today
rvn trait due --value past               # Overdue items
rvn trait priority --value high          # High priority items
rvn trait status --value todo            # Items with @status(todo)
rvn trait highlight                      # All highlighted items

# Saved queries (defined in raven.yaml)
rvn query --list                         # List available saved queries
rvn query tasks                          # Run 'tasks' saved query
rvn query overdue                        # Run 'overdue' saved query
rvn query add my-tasks --traits due,status --filter status=todo  # Create saved query
rvn query remove my-tasks                # Remove saved query

# Query objects
rvn query "type:person"
rvn query "type:meeting attendees:[[people/alice]]"
rvn query "type:project status:active"

# Show backlinks to a note
rvn backlinks <target>
rvn backlinks people/alice
rvn backlinks daily/2025-02-01#standup

# Show index statistics
rvn stats

# List untyped pages (missing explicit type)
rvn untyped

# Open/create daily notes
rvn daily                        # Today's note
rvn daily yesterday              # Yesterday's note
rvn daily tomorrow               # Tomorrow's note
rvn daily 2025-02-01             # Specific date

# Date hub - show everything related to a date
rvn date                         # Today
rvn date yesterday
rvn date 2025-02-01

# Create a new typed note
rvn new person "Alice Chen"       # Creates people/alice-chen.md
rvn new project "Website"         # Creates projects/website.md, prompts for required fields
rvn new person                    # Prompts for title interactively

# Quick capture
rvn add "Call Alice about the project"
rvn add "@due(tomorrow) Send estimate"
rvn add "Idea" --to inbox.md      # Override destination

# Watch for changes and auto-reindex (future)
rvn watch

# Start MCP server for AI agents
rvn serve --vault-path /path/to/vault
```

### Date Filters

Date fields support relative date expressions in queries:

| Filter | Meaning |
|--------|---------|
| `today` | Current day |
| `yesterday` | Previous day |
| `tomorrow` | Next day |
| `this-week` | Monday-Sunday of current week |
| `next-week` | Monday-Sunday of next week |
| `past` | Before today |
| `future` | After today |
| `YYYY-MM-DD` | Specific date |

**Examples:**
```bash
rvn trait due --value today           # Items due today
rvn trait due --value this-week       # Items due this week
rvn trait due --value past            # Overdue items
rvn trait remind --value tomorrow     # Reminders for tomorrow
rvn query overdue                     # Run saved 'overdue' query
```

### Date References

Date references use shorthand syntax:

| Syntax | Resolves To |
|--------|-------------|
| `[[2025-02-01]]` | `daily/2025-02-01` (or configured daily directory) |

This allows natural linking to dates without knowing the directory structure:
```markdown
See [[2025-02-01]] for the kickoff meeting notes.
```

### The `rvn date` Command

Shows everything related to a specific date (the "date hub"):
- The daily note for that date
- Tasks due on that date
- Events on that date  
- Any object with a date field matching that date
- References to that date

```bash
rvn date                  # Today's date hub
rvn date yesterday        # Yesterday
rvn date 2025-02-01       # Specific date
```

### The `rvn trait` Command

Generic interface for querying any trait type:

```bash
rvn trait <trait-name> [--field value] [--field value] ...
```

**Examples**:
```bash
rvn trait task                           # All tasks
rvn trait task --status todo             # Tasks with status=todo
rvn trait task --due today               # Due today
rvn trait task --due this-week           # Due this week  
rvn trait task --due overdue             # Past due
rvn trait task --assignee [[people/bob]] # Assigned to Bob
rvn trait task --parent.type meeting     # Tasks inside meetings

rvn trait remind --at today              # Reminders for today
rvn trait remind --at this-week          # This week's reminders

rvn trait highlight                      # All highlights
rvn trait highlight --color yellow       # Yellow highlights only
```

**Output formats**:
```bash
rvn trait task --format table            # Human-readable (default)
rvn trait task --format json             # Machine-readable
rvn trait task --format compact          # One-line per item
```

### The `rvn type` Command

List all objects of a specific type:

```bash
rvn type <type-name>
```

**Examples**:
```bash
rvn type person                          # List all people
rvn type project                         # List all projects
rvn type meeting                         # List all meetings
rvn type --list                          # Show all types with object counts
```

**Output**:
```
# person (2)

• people/alice
  email: alice@example.com, name: Alice Chen
  people/alice.md:1
• people/bob
  email: bob@example.com, name: Bob Smith
  people/bob.md:1
```

Use `rvn type --list` to see all available types:
```
Types:
  date            1 objects (built-in)
  meeting         1 objects
  page            - (built-in)
  person          2 objects
  project         1 objects
  section         12 objects (built-in)
```

### The `rvn tag` Command

Query objects by tag:

```bash
rvn tag <tag-name>
```

**Examples**:
```bash
rvn tag project                          # Find all objects tagged #project
rvn tag important                        # Find all objects tagged #important
rvn tag --list                           # Show all tags with usage counts
```

**Output**:
```
# #project (3)

• projects/website.md
    projects/website (line 1)
• daily/2025-02-01.md
    daily/2025-02-01 (line 15)
• people/alice.md
    people/alice#current-work (line 20)
```

Use `rvn tag --list` to see all tags in your vault:
```
Tags (8 total):

  #project              3 objects
  #important            2 objects
  #work                 2 objects
  #personal             1 object
  #reading              1 object
```

### The `rvn add` Command

Quick capture for low-friction note-taking:

```bash
rvn add <text>
rvn add <text> --to <file>
```

**Examples**:
```bash
rvn add "Call Alice about the project"
rvn add "@due(tomorrow) @priority(high) Send estimate"
rvn add "Project idea" --to inbox.md
```

**Behavior**:
- By default, appends to today's daily note (creates if needed)
- Traits in the text are preserved and indexed
- Timestamps are added by default

**Configuration** (in `raven.yaml`):
```yaml
capture:
  destination: daily      # "daily" or a file path
  heading: "## Captured"  # Append under this heading (optional)
  timestamp: false        # Prefix with time (default: false, use --timestamp flag)
  reindex: true           # Auto-reindex after capture
```

**Output**:
```
✓ Added to daily/2026-01-01.md
```

The captured line in the file (default, no timestamp):
```markdown
- @due(tomorrow) @priority(high) Send estimate
```

With `--timestamp` flag:
```markdown
- 15:30 @due(tomorrow) @priority(high) Send estimate
```

### The `rvn set` Command

Update frontmatter fields on existing objects:

```bash
rvn set <object_id> <field=value>...
```

**Examples**:
```bash
rvn set people/alice email=alice@example.com
rvn set people/alice name="Alice Chen" status=active
rvn set projects/website priority=high
```

**Behavior**:
- Validates fields against the object's type schema
- Warns (but allows) unknown fields
- Preserves existing frontmatter fields not being updated
- Logs updates to the audit log

**JSON output** (for agents):
```json
{
  "ok": true,
  "data": {
    "file": "people/alice.md",
    "object_id": "people/alice",
    "type": "person",
    "updated_fields": {"email": "alice@example.com"}
  }
}
```

### Saved Queries

Saved queries provide ergonomic shortcuts for common queries. Define them in `raven.yaml`:

```yaml
queries:
  tasks:
    traits: [due, status]
    filters:
      status: "todo,in_progress,"
    description: "Open tasks"

  people:
    types: [person]
    description: "All people"

  project-summary:
    types: [project]
    traits: [due]
    description: "Projects with due dates"

  important:
    tags: [important]
    description: "Items tagged #important"

  work-projects:
    tags: [work, project]
    description: "Items with both #work AND #project tags"
```

Queries can include `types`, `traits`, `tags`, or any combination. When multiple tags are specified, objects must have ALL tags (AND logic).

Then run them:

```bash
rvn query tasks              # Run saved query (traits)
rvn query people             # Run saved query (types)
rvn query important          # Run saved query (tags)
rvn query project-summary    # Run saved query (mixed)
rvn query --list             # List all saved queries
rvn query add my-tasks --traits due,status --filter status=todo  # Create
rvn query remove my-tasks    # Remove
```

For direct queries, use `rvn trait`, `rvn type`, or `rvn tag`:

```bash
rvn trait due --value past   # All overdue items
rvn trait highlight          # All highlights
rvn type person              # All people
rvn type project             # All projects
rvn tag important            # All items tagged #important
rvn tag --list               # List all tags
```

### Trait Definition

Traits are single-valued annotations defined in `schema.yaml`:

```yaml
traits:
  due:
    type: date

  priority:
    type: enum
    values: [low, medium, high]
    default: medium

  highlight:
    type: boolean
```

| Property | Description |
|----------|-------------|
| `type` | Value type: `date`, `datetime`, `enum`, `string`, `boolean`, `ref` |
| `values` | For enum types: list of allowed values |
| `default` | Default value if none provided |

**Note**: To create "task-like" shortcuts, use saved queries in `raven.yaml` rather than trait configuration.

### The `rvn check` Command

The check command validates the entire vault and surfaces errors and warnings:

**Errors** (must fix):
- Type not defined in `schema.yaml`
- Required field missing
- Field value doesn't match schema type
- Enum value not in allowed list
- Number outside min/max bounds
- Reference to non-existent object
- Embedded object missing `id` field
- Duplicate object IDs
- Ambiguous short reference (multiple matches)

**Warnings** (informational):
- Undefined trait (not in schema, will be skipped)
- Orphan files (not referenced by anything)
- Heading nesting approaching limit

**Output**:
```bash
$ rvn check
Checking 847 files...

ERROR: daily/2025-02-01.md:15 - Missing required field 'id' in embedded type 'meeting'
ERROR: projects/website.md:8 - Reference [[alice]] is ambiguous (matches: people/alice, clients/alice)
WARN:  notes/random.md:23 - Undefined trait '@custom' will be skipped
WARN:  books/old-book.md - No incoming references (orphan)

Found 2 errors, 2 warnings in 847 files.
```

**Create missing references** (`--create-missing`):

When references point to non-existent files, `rvn check --create-missing` offers to create them interactively:

```bash
$ rvn check --create-missing

--- Missing References ---

Certain (from typed fields):
  • people/carol → person (from daily/2025-01-01#team-sync.attendees)
  • people/dan → person (from daily/2025-01-01#team-sync.attendees)

Create these pages? [Y/n] y
  ✓ Created people/carol.md (type: person)
  ✓ Created people/dan.md (type: person)

Unknown type (please specify):
  ? notes/random-idea (referenced in projects/website.md:15)

Available types: meeting, person, project

Type for notes/random-idea (or 'skip'): skip
  Skipped notes/random-idea

✓ Created 2 missing page(s).
```

Type inference:
- **Certain**: Reference is from a typed field (e.g., `attendees: ref[], target: person`) → type is known
- **Inferred**: Reference path matches a type's `default_path` → type is suggested
- **Unknown**: No inference possible → user must specify or skip

### Global Options

```bash
rvn --vault /path/to/vault <command>
rvn --config /path/to/config.toml <command>
```

---

## Implementation Phases

### Phase 1: Core Parser & Index (MVP)

**Goal**: Parse markdown files and build a queryable index.

1. **Schema Loader**
   - Parse `schema.yaml`
   - Define TypeDefinition and TraitDefinition structs
   - Validate schema structure
   - Include built-in `page` and `section` types

2. **Global Config Loader**
   - Load from `~/.config/raven/config.toml`
   - Support vault path and editor settings
   - Require explicit vault (no fallback to cwd for safety)

3. **Frontmatter Parser**
   - Extract YAML between `---` markers
   - Convert to map[string]interface{}
   - Support `tags:` array in frontmatter

4. **Markdown Parser** (using pulldown-cmark)
   - Use AST-based parsing, NOT string manipulation
   - Extract heading hierarchy with proper code block handling
   - Track line numbers via offset iterator
   - Validate nesting depth (H1-H6)
   - Ignore headings inside code blocks

5. **Type Declaration Parser**
   - Parse `::type(id=..., key=value, ...)` syntax
   - Require `id` field for embedded types
   - Handle various value types (strings, refs, arrays)
   - Generate full object ID: `file-path#id`

6. **Trait Annotation Parser**
   - Parse `@trait(key=value, ...)` syntax
   - Support positional arguments (must precede named)
   - Extract content between carriage returns as trait content

7. **Reference & Tag Extractor**
   - Find all `[[ref]]` and `[[ref|display]]` patterns
   - Handle array syntax `[[[ref1]], [[ref2]]]` correctly
   - Extract `#tags` using AST (ignore tags in code blocks)
   - Skip number-only tags like `#123`
   - Track positions for source mapping

8. **Document Parser**
   - Combine all parsers into ParsedDocument
   - **Create section objects for every heading**
   - If heading has `::type()` on next line, use that type instead of section
   - Auto-generate section IDs from slugified heading text
   - Handle duplicate slugs: `#ideas`, `#ideas-2`, `#ideas-3`
   - Build object tree from heading hierarchy
   - Assign parents to traits based on line numbers
   - Compute line_end for each object

8. **Reference Resolver**
   - Resolve short refs to full paths
   - Build index of all object IDs for resolution
   - Flag ambiguous references

9. **SQLite Indexer**
   - Create database schema
   - Index parsed documents
   - Handle incremental updates (delete file, re-insert)
   - Store all trait fields in JSON blob

10. **Basic CLI**
    - `rvn init`
    - `rvn reindex`
    - `rvn check` (validation)
    - `rvn check --create-missing` (interactively create missing references)
    - `rvn trait` (query by trait type)
    - `rvn type` (query by object type)
    - `rvn tag` (query by tag)
    - `rvn query` (saved queries)
    - `rvn backlinks`
    - `rvn stats`
    - `rvn untyped`
    - `rvn daily`
    - `rvn date`
    - `rvn new` (create typed object)
    - `rvn add` (quick capture)
    - `rvn set` (update frontmatter fields)
    - `rvn delete` (delete object, moves to trash)
    - `rvn read` (read raw file content)
    - `rvn path` (print vault path)
    - `rvn vaults` (list configured vaults)
    - `rvn schema` (introspect schema)
    - `rvn schema add type/trait/field` (add to schema)
    - `rvn schema update type/trait/field` (modify schema)
    - `rvn schema remove type/trait/field` (remove from schema)
    - `rvn schema validate` (validate schema)
    - `rvn serve` (MCP server for AI agents)

### Phase 2: Enhanced Querying

1. **Query Language**
   - Parse query strings like `type:meeting attendees:[[alice]]`
   - Support field filters with JSON extraction
   - Support date ranges (`due:this-week`)
   - Support parent filters (`parent.type:meeting`)

2. **Full-Text Search**
   - Add FTS5 virtual table
   - Index content for text search
   - Combine with structured queries

3. **Output Formatting**
   - JSON output for scripting
   - Table format for humans
   - Customizable fields to display

### Phase 3: File Watching & Live Index

1. **File Watcher**
   - Use `fsnotify` package to watch vault directory
   - Debounce rapid changes
   - Incremental reindex on file change

2. **Background Service**
   - `rvn watch` runs in background
   - Keeps index always up-to-date

### Phase 4: Refactoring Tools

1. **Reference Updates**
   - When an object is renamed/moved, update all references
   - `rvn mv <old-path> <new-path>` command

2. **Note Promotion**
   - Move embedded object to standalone file
   - `rvn promote <object-id> --to <new-path>`
   - Update references automatically

### Phase 5: Web UI

1. **Local Web Server**
   - `rvn serve` starts HTTP server
   - Serve static files for UI

2. **Read-Only Views**
   - Browse all objects by type
   - View backlinks graph
   - Task list with filters
   - Full-text search

3. **Editing (Future), don't implment yet**
   - Edit notes in browser
   - Task completion toggles
   - Quick capture

### Phase 6: Calendar Integration (Future, not now)

1. **Date Handling**
   - Robust ISO 8601 parsing
   - Timezone support from config
   - Relative dates (`today`, `this-week`)

2. **Calendar Sync**
   - Export meetings to ICS
   - Google Calendar API integration
   - Two-way sync (future)

3. **Recurring Events**
   - RRULE parsing
   - Friendly syntax: `weekly on mon, wed, fri`

---

## Future Enhancements

### Note Promotion

Move embedded object to standalone file:

```bash
rvn promote mtg-standup-2025-02-01 --to meetings/weekly-standup-2025-02-01.md
```

This would:
1. Create new file with frontmatter from embedded fields
2. Move content to new file
3. Replace original section with `![[new-file]]` or `[[new-file]]`
4. Update all references to point to new location

### Task Management

```bash
rvn task complete <task-id>          # Mark as done
rvn task snooze <task-id> --to tomorrow
rvn task list --overdue
```

### Templates

```yaml
# In schema.yaml
types:
  meeting:
    template: |
      ## Attendees
      
      ## Agenda
      
      ## Notes
      
      ## Action Items
```

### Plugins/Extensions

Allow custom traits with behavior:

```yaml
traits:
  pomodoro:
    fields:
      duration: { type: number, default: 25 }
    hooks:
      on_complete: "notify-send 'Pomodoro complete!'"
```

---

## Design Decisions

This section documents key design decisions made during planning.

### Syntax Choices

| Element | Syntax | Rationale |
|---------|--------|-----------|
| File-level type | YAML frontmatter | Familiar, standard markdown convention |
| Embedded type | `::type(id=..., ...)` | `::` distinguishes from traits (`@`), inline for speed |
| Traits | `@trait(...)` | `@` is intuitive for annotations |
| References | `[[path/file#id]]` | Wiki-style links, `#` for fragments (standard) |
| Tags | `#tag` | Standard hashtag syntax |

### ID Strategy

| Object Type | ID Format | Example |
|-------------|-----------|---------|
| File-level | Path without extension | `people/alice` |
| Embedded (explicit) | Path + `#` + explicit ID | `daily/2025-02-01#standup` |
| Section (auto) | Path + `#` + slugified heading | `daily/2025-02-01#morning` |

- **Explicit IDs required**: Explicitly typed embedded objects must have an `id` field
- **Section IDs auto-generated**: Slugified from heading text, with duplicate handling (`#ideas`, `#ideas-2`)
- **Path uniqueness**: File paths must be unique across the vault
- **Short references**: Allowed if unambiguous, warned otherwise

### Section Objects

Every markdown heading creates an object in the model. This ensures:
- Complete document structure is queryable
- Traits and refs have a parent context
- Backlinks can point to specific sections

**Explicit types override sections**: If a heading has `::type()` on the next line, that type is used instead of `section`.

**Hierarchy**: Sections nest based on heading levels. H2 is child of H1, H3 is child of H2, etc.

### Vault Scoping (Security)

All CLI operations are strictly scoped to the configured vault:
- No fallback to current directory (prevents accidental scanning of wrong folders)
- Symlinks are not followed during directory traversal
- Canonical path validation blocks path traversal attacks
- Only `rvn init <path>` can create files at an arbitrary location

### Trait Metadata Storage

All trait fields are stored in a JSON `fields` blob (no dedicated columns). Common query patterns use JSON indexes:

```sql
CREATE INDEX idx_traits_status ON traits(json_extract(fields, '$.status'));
CREATE INDEX idx_traits_due ON traits(json_extract(fields, '$.due'));
```

**Rationale**: Uniform model, schema-driven, avoids migrations when adding trait fields.

### Incremental Updates

When a file changes:
1. Delete all objects, traits, and refs from that file
2. Re-parse and re-insert everything

**Rationale**: Simple implementation, trait IDs are internal. Object IDs (path-based) remain stable.

### Type Resolution

Type is determined by a simple rule:

1. If frontmatter has `type:` field → use that type
2. Otherwise → `page` (fallback)

**No detection or inference.** File location, content, or other fields never affect the type. This is explicit and predictable.

### Tag Inheritance

Tags flow upward: child embedded objects' tags are inherited by parent objects.

### CLI Trait Commands

Rather than hard-coding trait-specific commands, we use a hybrid approach:

1. **Generic `rvn trait <name>`**: Universal interface for querying any trait
2. **Schema-defined aliases**: Users add `cli.alias` to traits in `schema.yaml` for shortcuts
3. **Default schema**: `rvn init` creates a schema with common aliases like `tasks`

All CLI aliases are explicit in the schema—no hidden behavior.

---

## Technical Notes

### Go Packages

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `gopkg.in/yaml.v3` | YAML parsing |
| `encoding/json` | JSON serialization (stdlib) |
| `github.com/BurntSushi/toml` | TOML config parsing |
| `modernc.org/sqlite` | Pure Go SQLite database |
| `github.com/yuin/goldmark` | Markdown parsing (AST-based) |
| `regexp` | Pattern matching (stdlib) |
| `github.com/gosimple/slug` | Slugifying heading text for IDs |
| `path/filepath` | Directory traversal (stdlib) |
| `os` | File operations (stdlib) |
| `time` | Date/time handling (stdlib) |

### Markdown Parsing with goldmark

**Critical**: Use `goldmark` for proper markdown AST parsing. Manual string parsing leads to bugs.

**Why AST parsing matters**:
- Headings inside code blocks are correctly ignored
- Tags (`#tag`) inside code blocks or inline code are correctly ignored
- Edge cases in markdown syntax are handled correctly

**Implementation pattern**:
```go
import "github.com/yuin/goldmark/ast"

func extractHeadings(content []byte) []Heading {
    md := goldmark.New()
    reader := text.NewReader(content)
    doc := md.Parser().Parse(reader)
    
    var headings []Heading
    ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
        if heading, ok := n.(*ast.Heading); ok && entering {
            // Extract heading text and level
            text := extractText(heading, content)
            headings = append(headings, Heading{
                Level: heading.Level,
                Text:  text,
                Line:  lineNumber(heading),
            })
        }
        return ast.WalkContinue, nil
    })
    return headings
}
```

**Tag extraction** must also use the AST to avoid false positives:
```go
ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
    // Skip code blocks entirely
    if _, ok := n.(*ast.CodeBlock); ok {
        return ast.WalkSkipChildren, nil
    }
    if _, ok := n.(*ast.FencedCodeBlock); ok {
        return ast.WalkSkipChildren, nil
    }
    // Skip inline code
    if _, ok := n.(*ast.CodeSpan); ok {
        return ast.WalkSkipChildren, nil
    }
    // Extract tags from text nodes
    if text, ok := n.(*ast.Text); ok && entering {
        segment := text.Segment
        tags = append(tags, extractTagsFromText(string(content[segment.Start:segment.Stop]))...)
    }
    return ast.WalkContinue, nil
})
```

**Additional tag rules**:
- Tags must not start with a digit (avoid `#123` issue references)
- Tags must be preceded by whitespace or punctuation

### Performance Considerations

- Parse files in parallel using goroutines
- Batch database inserts in transactions
- Use prepared statements for repeated queries
- Keep index in WAL mode for concurrent reads
- Only reparse changed files (check mtime)

### Testing Strategy

- Unit tests for each parser component
- Integration tests with sample vaults
- Property-based tests for parser edge cases
- Benchmark tests for large vaults (1000+ files)

---

## MCP Server (Agent Integration)

Raven provides first-class AI agent support via the **Model Context Protocol (MCP)**. The MCP server wraps CLI commands with structured JSON input/output, enabling LLM agents to interact with your knowledge base.

### Starting the Server

```bash
rvn serve --vault-path /path/to/vault
```

The server communicates via JSON-RPC 2.0 over stdin/stdout, compatible with Claude Desktop and other MCP clients.

### Available Tools

| Tool | Description | Required Args |
|------|-------------|---------------|
| `raven_new` | Create typed object | `type`, `title`, optional `fields` |
| `raven_set` | Update frontmatter fields on object | `object_id`, `fields` (key-value pairs) |
| `raven_read` | Read raw file content | `path` |
| `raven_add` | Append to existing file or daily note | `text`, optional `to` |
| `raven_delete` | Delete object (trash by default) | `object_id` |
| `raven_trait` | Query by trait | `trait_type`, optional `value` |
| `raven_query` | Run saved query | `query_name` |
| `raven_query_add` | Create saved query | `name`, optional `traits`, `types`, `tags`, `filter`, `description` |
| `raven_query_remove` | Remove saved query | `name` |
| `raven_type` | List objects by type | `type_name` |
| `raven_tag` | Query by tag | `tag` |
| `raven_backlinks` | Find references to object | `target` |
| `raven_date` | Get activity for date | `date` |
| `raven_stats` | Vault statistics | (none) |
| `raven_schema` | Introspect schema | optional `subcommand` |
| `raven_schema_add_type` | Add type to schema | `name`, optional `default_path` |
| `raven_schema_add_trait` | Add trait to schema | `name`, optional `type`, `values` |
| `raven_schema_add_field` | Add field to type | `type_name`, `field_name`, optional flags |
| `raven_schema_update_type` | Update type | `name`, optional `default_path`, `add_trait`, `remove_trait` |
| `raven_schema_update_trait` | Update trait | `name`, optional `type`, `values`, `default` |
| `raven_schema_update_field` | Update field (blocks on integrity issues) | `type_name`, `field_name`, optional flags |
| `raven_schema_remove_type` | Remove type (files become 'page') | `name`, optional `force` |
| `raven_schema_remove_trait` | Remove trait (instances stay in files) | `name`, optional `force` |
| `raven_schema_remove_field` | Remove field (blocks if required) | `type_name`, `field_name` |
| `raven_schema_validate` | Validate schema | (none) |

### JSON Response Format

All commands return a standard envelope:

```json
{
  "ok": true,
  "data": { ... },
  "warnings": [ ... ],
  "meta": {
    "count": 5,
    "query_time_ms": 12
  }
}
```

Errors use structured codes for programmatic handling:

```json
{
  "ok": false,
  "error": {
    "code": "REQUIRED_FIELD_MISSING",
    "message": "Missing required fields: name",
    "details": { ... },
    "suggestion": "Ask user for values, then retry with --field flags"
  }
}
```

### Design Principles

1. **Schema Discovery**: Agents can call `raven_schema commands` to discover available operations
2. **Graceful Errors**: Missing required fields return structured errors with hints
3. **Backlink Awareness**: Delete warns about references to the deleted object
4. **Read-Only by Default**: `add` only appends to existing files; use `new` for creation
5. **Vault Scoped**: All operations are restricted to the configured vault
6. **Auto-Generated Tools**: MCP tools are generated from the command registry, ensuring CLI and MCP are always in sync

---

## Appendix: Example Vault

```
vault/
├── schema.yaml
├── daily/
│   ├── 2025-02-01.md
│   └── 2025-02-02.md
├── people/
│   ├── alice.md
│   └── bob.md
├── projects/
│   ├── website-redesign.md
│   └── mobile-app.md
├── books/
│   └── atomic-habits.md
└── meetings/
    └── weekly-standup.md    # Recurring meeting series
```

### Sample: `people/alice.md`

```markdown
---
type: person
name: Alice Chen
email: alice@example.com
---

# Alice Chen

Senior engineer on the platform team.

## Notes

- Met at 2024 company offsite
- Leading the [[projects/website-redesign]] project
- @due(2025-02-01) Send her the API docs

## 1:1 Topics

- Career growth
- Team dynamics
```

**Object IDs generated from this file**:
| ID | Type | Heading |
|----|------|---------|
| `people/alice` | `person` | (file-level) |
| `people/alice#alice-chen` | `section` | Alice Chen |
| `people/alice#notes` | `section` | Notes |
| `people/alice#1-1-topics` | `section` | 1:1 Topics |

### Sample: `daily/2025-02-01.md`

```markdown
---
type: daily
date: 2025-02-01
tags: [work]
---

# Saturday, February 1, 2025

## Morning

Reviewed [[projects/website-redesign]] progress. Looking good.

## Weekly Standup
::meeting(id=standup, time=09:00, attendees=[[[people/alice]], [[people/bob]]])

Discussed Q2 priorities.

- @due(2025-02-03) Follow up on timeline
- [[people/alice]] will send updated estimates

## Afternoon

- @due(2025-02-02) @priority(high) Review PR #1234
- @remind(2025-02-02T14:00) Call with vendor

## Reading

Chapter 2 of [[books/atomic-habits]].

- @highlight Small habits compound over time
```

**Object IDs generated from this file**:
| ID | Type | Heading |
|----|------|---------|
| `daily/2025-02-01` | `daily` | (file-level) |
| `daily/2025-02-01#saturday-february-1-2025` | `section` | Saturday, February 1, 2025 |
| `daily/2025-02-01#morning` | `section` | Morning |
| `daily/2025-02-01#standup` | `meeting` | Weekly Standup (explicit type) |
| `daily/2025-02-01#afternoon` | `section` | Afternoon |
| `daily/2025-02-01#reading` | `section` | Reading |
