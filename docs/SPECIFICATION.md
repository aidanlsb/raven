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
13. [MCP Server](#mcp-server-agent-integration)
14. [LSP Server](#lsp-server-editor-integration)

---

## Core Concepts

### The Four Primitives

| Concept | Purpose | Syntax | Can be Referenced? | Example |
|---------|---------|--------|-------------------|---------|
| **Types** | Define what something *is* | Frontmatter `type:` | Yes, via `[[path/file]]` | person, project, meeting, book |
| **Embedded Types** | A typed section within a file | `::type(...)` | Yes, via `[[path/file#slug]]` | A meeting inside a daily note |
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

### Objects

Objects are instances of types. They are referenceable entities that come in two forms:

#### File-Level Objects
The file itself represents the object. Type declared in frontmatter.

```
people/freya.md  →  Object(id="people/freya", type="person", ...)
```

**Object ID**: The file path without extension (e.g., `people/freya`).

#### Embedded Objects (Explicit Types)
A section within a file can be explicitly typed with `::type()` on the line after a heading.

```
daily/2025-02-01.md
  └── ## Weekly Standup     →  Object(id="daily/2025-02-01#weekly-standup", type="meeting", ...)
        ::meeting(time=09:00)
```

**Object ID**: The file path + `#` + slugified heading text (e.g., `daily/2025-02-01#weekly-standup`).

**ID Generation Rules** (same as sections):
1. Heading text is slugified (lowercased, spaces become hyphens, special chars removed)
2. If multiple headings have the same slug, numbers are appended: `#team-sync`, `#team-sync-2`
3. If heading text is empty, fallback to the type name as the slug
4. An explicit `id` field can override the auto-generated slug: `::meeting(id=standup)` → `#standup`

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

All headings form a tree based on heading levels. Every heading becomes an object (either an explicitly typed object via `::type()` or a `section` object by default):

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

**Nesting limit**: Standard markdown heading depth (H1-H6). Deeper nesting is not supported.

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
Met with [[people/freya]] about [[projects/website]].
```

Both outgoing refs (what this note links to) and backlinks (what links to this note) are indexed.

**Reference resolution**:
- Use full path for clarity: `[[people/freya]]`
- Short names allowed if unambiguous: `[[freya]]` works if only one `freya` exists
- Aliases can be used: `[[goddess]]` resolves to `people/freya` if that object has `alias: goddess`
- Embedded objects: `[[daily/2025-02-01#standup]]`
- The `rvn check` command warns about ambiguous short references

**Slugified matching**: References are matched using slugification, so `[[people/Sif]]` will resolve to `people/sif.md`. This allows natural inline references while maintaining clean, URL-safe filenames.

**Validation**: All references must resolve to existing objects. The `rvn check` command errors on broken references.

### Aliases

Objects can define an optional `alias` field in their frontmatter to provide an alternative name for reference resolution:

```yaml
---
type: person
name: Freya
alias: goddess
---
```

Now `[[goddess]]` will resolve to `people/freya`.

**Alias rules**:
- Aliases must be unique across the vault
- An alias cannot conflict with an existing object's short name or ID
- If a conflict exists, references using that alias become **ambiguous** (explicit is better than implicit)
- Aliases are case-insensitive (matched via slugification)

**Examples**:

```yaml
# people/freya.md
---
type: person
name: Freya
alias: goddess
---

# companies/acme-corp.md
---
type: company
name: Acme Corporation
alias: ACME
---
```

```markdown
Met with [[goddess]] about the [[ACME]] project.
# Resolves to: people/freya and companies/acme-corp
```

**Conflict detection**: The `rvn check` command reports alias conflicts:
- `duplicate_alias`: Multiple objects using the same alias
- `alias_collision`: An alias conflicts with an existing short name or object ID

**When conflicts occur**: If an alias conflicts with something else, references using that alias are treated as ambiguous—Raven won't silently guess. Use the full path to disambiguate.

---

## File Format Specification

### Frontmatter (File-Level Metadata)

YAML frontmatter defines the file's type and fields:

```markdown
---
type: person
name: Freya
email: freya@asgard.realm
---

# Freya

Content here...
```

**Rules**:
- Frontmatter is optional
- If present, must be valid YAML between `---` markers
- The `type` field determines which type this object is an instance of
- The `alias` field (optional) provides an alternative name for reference resolution
- Other fields are validated against the schema

### Embedded Types

Declared with `::type()` on a heading:

```markdown
## Team Sync
::meeting(time=2025-02-01T09:00, attendees=[[[people/freya]], [[people/thor]]])

Content of the meeting...
```

**Rules**:
- `::type()` must appear on the line immediately after the heading (or within 2 lines)
- First argument is the type name
- Object ID is derived from the slugified heading text (e.g., `## Team Sync` → `#team-sync`)
- Optional `id` field overrides the auto-generated slug: `::meeting(id=standup)` → `#standup`
- Additional arguments are `key=value` field assignments
- Scope extends from this heading to the next heading at same or higher level
- Full object ID = `file-path#slug` (e.g., `daily/2025-02-01#team-sync`)

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

**Bare boolean traits**:
When a boolean trait is used bare (e.g., `@highlight`), it's stored with value `"true"` in the index. This means:
- `@highlight` is equivalent to `@highlight(true)`
- Query with `rvn query "trait:highlight value:true"` or just `rvn query "trait:highlight"`

**Schema as source of truth**:
Only traits defined in `schema.yaml` are indexed and queryable. Undefined traits:
- Appear in your files (visible in markdown)
- Are NOT indexed in the database
- Show warnings during `rvn check`
- Can be added to schema interactively via `rvn check --create-missing`

**"Tasks" are emergent**: Instead of a composite `@task` trait, use atomic traits. Anything with `@due` or `@status` is effectively a "task". Use saved queries to define what "tasks" means in your workflow.

### References

Wiki-style links:

```markdown
[[people/freya]]               # Full path reference (preferred)
[[freya]]                      # Short reference (if unambiguous)
[[freya|Lady Freya]]           # Reference with display text
[[daily/2025-02-01#standup]]   # Reference to embedded object
```

**Resolution rules**:
1. If path contains `/`, treat as full path from vault root
2. If short name, search for unique match across vault
3. If ambiguous (multiple matches), `rvn check` warns and requires full path
4. All references must resolve to existing objects (validated during check)

---

## Vault Configuration

### File: `raven.yaml`

Located at vault root. Controls vault-level settings.

```yaml
# Raven Vault Configuration
# These settings control vault-level behavior.

# Where daily notes are stored (default: daily)
daily_directory: daily

# Auto-reindex after CLI operations that modify files (default: true)
# Commands like 'rvn add', 'rvn new', 'rvn set', 'rvn edit' will
# automatically update the index when they modify files.
auto_reindex: true

# Quick capture settings for 'rvn add'
capture:
  destination: daily      # "daily" (default) or a file path like "inbox.md"
  heading: "## Captured"  # Optional - append under this heading
  timestamp: false        # Prefix with time (default: false, use --timestamp flag)

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

**Filter Syntax:**
| Syntax | Meaning | Example |
|--------|---------|---------|
| `value` | Exact match | `status: done` |
| `a\|b` | OR - matches a or b | `due: "this-week\|past"` |
| `!value` | NOT - excludes value | `status: "!done"` |
| Date keywords | Relative dates | `today`, `this-week`, `past`, `future` |

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

  # OR filter: due this week or overdue
  urgent:
    traits: [due]
    filters:
      due: "this-week|past"
    description: "Due soon or overdue"

  # NOT filter: exclude done items
  open-tasks:
    traits: [status]
    filters:
      status: "!done"
    description: "Tasks not yet done"

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
# Traits appear inline in content using @trait or @trait(value) syntax
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

  project:
    default_path: projects/
    fields:
      lead:
        type: ref
        target: person
      technologies:
        type: string[]

  # Note: 'date' type is built-in for daily notes (YYYY-MM-DD.md)

  meeting:
    fields:
      time:
        type: datetime
      attendees:
        type: ref[]
        target: person
      
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
```

### Separation of Fields and Traits

**Key design principle:**
- **Fields** are type-specific structured data in frontmatter
- **Traits** are universal annotations in content using `@trait` or `@trait(value)` syntax

**Key rules:**
- Frontmatter can ONLY contain: `type`, `id`, `tags`, and declared fields
- Unknown frontmatter keys trigger validation errors
- Traits appear in content using `@trait` or `@trait(value)` syntax
- Traits can be used on ANY object type - they are universal
- Traits annotate specific content (lines) within documents

**Example: Fields vs Traits**

```markdown
---
type: project
status: active        # Field: type-specific metadata on the project
client: "[[clients/midgard]]"
---

# Website Redesign

## Tasks

- @due(2025-03-01) Send proposal       # Trait: annotates this specific task
- @due(2025-03-15) Review feedback     # Trait: annotates this specific task
- @priority(high) Security review
```

Fields (`status`, `client`) are queried with object filters: `rvn query "object:project .status:active"`
Traits (`@due`, `@priority`) are queried with trait filters: `rvn query "trait:due value:past"`

### Field Types

| Type | Description | Example |
|------|-------------|---------|
| `string` | Plain text | `name: "Freya"` |
| `string[]` | Array of strings | `technologies: [rust, typescript]` |
| `number` | Numeric value | `rating: 4.5` |
| `number[]` | Array of numbers | `scores: [85, 92, 78]` |
| `date` | ISO 8601 date | `due: 2025-02-01` |
| `date[]` | Array of dates | `milestones: [2025-02-01, 2025-03-15]` |
| `datetime` | ISO 8601 datetime | `time: 2025-02-01T09:00` |
| `enum` | One of specified values | `status: active` |
| `enum[]` | Array of enum values | `categories: [work, personal]` |
| `bool` | Boolean | `archived: true` |
| `bool[]` | Array of booleans | `flags: [true, false, true]` |
| `ref` | Reference to another object | `author: [[people/freya]]` |
| `ref[]` | Array of references | `attendees: [[[people/freya]], [[people/thor]]]` |

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
| `id` | Embedded object identifier (optional, overrides auto-generated slug from heading) |
| `type` | Object type name |
| `alias` | Alternative name for reference resolution (optional) |

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
- `rvn new person "Freya"` creates `people/freya.md`
- `rvn new person "Sif"` creates `people/sif.md` (slugified)
- The directory is created if it doesn't exist
- If no `default_path` is set, files are created in the vault root
- Filenames are always slugified (lowercase, spaces become hyphens)
- The original title is preserved in the file's heading (e.g., `# Sif`)

**Important:** File location has **no effect on type**. A file's type is determined **solely** by its frontmatter `type:` field (or defaults to `page` if not specified). You can have a `person` file anywhere in your vault—Raven doesn't care about directory structure.

**Fallback**: Files without an explicit `type:` field are assigned the `page` type.

---

## Syntax Reference

### Quick Reference

| Syntax | Purpose | Creates Object? |
|--------|---------|-----------------|
| `---`...`---` (frontmatter) | File-level type declaration | Yes |
| `::type(...)` | Embedded type declaration (id auto-generated from heading) | Yes |
| `@trait(...)` | Trait annotation | No |
| `[[path/file]]` | Reference to file object | — |
| `[[path/file#id]]` | Reference to embedded object | — |

### Type Declaration Syntax

**File-level** (YAML frontmatter):
```yaml
---
type: meeting
time: 2025-02-01T09:00
attendees:
  - [[people/freya]]
  - [[people/thor]]
---
```

**Embedded** (inline with `::`):
```
::meeting(id=standup, time=2025-02-01T09:00, attendees=[[[people/freya]], [[people/thor]]])
   │       │           └── key=value field assignments
   │       └── optional ID override (defaults to slugified heading)
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
| Single ref | `key=[[path]]` | `author=[[people/freya]]` |
| Ref array | `key=[[[a]], [[b]]]` | `attendees=[[[people/freya]], [[people/thor]]]` |
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
| Reference | `@name([[path]])` | `@assignee([[people/freya]])` |

**Note:** Unlike type declarations, traits take a single positional value only. Use multiple traits instead of multiple fields: `@due(2025-02-01) @priority(high)`

### Complete Example

```markdown
---
type: date
---

# Saturday, February 1, 2025

Morning coffee, reviewed [[projects/website-redesign]].

## Weekly Standup
::meeting(id=standup, time=09:00, attendees=[[[people/freya]], [[people/thor]]], recurring=[[meetings/weekly-standup]])

Discussed Q2 roadmap. [[people/freya]] raised concerns about timeline.

- @due(2025-02-03) @assignee([[people/freya]]) Send revised estimate
- Agreed to revisit next week
- @highlight Key insight: we need more buffer time

## 1:1 with Thor
::meeting(id=one-on-one-thor, time=14:00, attendees=[[[people/thor]]])

Talked about his career growth.

- @due(2025-02-10) Write up promotion case
- He's interested in the tech lead role on [[projects/mobile-app]]

## Reading

Started [[books/poetic-edda]] translated by [[people/snorri]].

- @highlight The Norns weave the fate of gods and men alike
- @due(2025-02-15) Finish chapter 3

## Random Thoughts

- @remind(2025-02-02T10:00) Check if designs are ready
```

**Object IDs in this example**:
- File: `daily/2025-02-01`
- Embedded: `daily/2025-02-01#standup`, `daily/2025-02-01#one-on-one-thor`

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
│   └── freya.md
├── projects/
│   └── website.md
└── books/
    └── poetic-edda.md
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
│   ├── refs.go              # Extract [[references]]
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
    ├── query.go             # rvn query (unified query interface)
    ├── backlinks.go         # rvn backlinks
    ├── stats.go             # rvn stats
    ├── untyped.go           # rvn untyped
    ├── daily.go             # rvn daily
    ├── date.go              # rvn date
    ├── new.go               # rvn new
    ├── add.go               # rvn add
    ├── set.go               # rvn set
    ├── read.go              # rvn read
    ├── edit.go              # rvn edit
    ├── move.go              # rvn move
    ├── search.go            # rvn search
    ├── delete.go            # rvn delete
    ├── path.go              # rvn path (print vault path)
    ├── vaults.go            # rvn vaults (list configured vaults)
    ├── schema.go            # rvn schema (introspection)
    ├── schema_edit.go       # rvn schema add/update/remove
    ├── lsp.go               # rvn lsp (LSP server mode)
    ├── watch.go             # rvn watch (file watching)
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
    fields TEXT NOT NULL DEFAULT '{}', -- JSON of all field values
    line_start INTEGER NOT NULL,      -- Line number where object starts
    line_end INTEGER,                 -- Line number where object ends (embedded only)
    parent_id TEXT,                   -- Parent object ID (for nested embedded)
    alias TEXT,                       -- Optional alias for reference resolution
    file_mtime INTEGER,               -- File modification time (for staleness detection)
    indexed_at INTEGER,               -- When this object was indexed
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
rvn check --by-file             # Group issues by file path

# Reindex (incremental by default - only changed/deleted files)
rvn reindex
rvn reindex --dry-run           # Show what would be reindexed
rvn reindex --full              # Force full reindex of all files

# Query objects
rvn query "object:person"                 # All people
rvn query "object:project"                # All projects
rvn query "object:project .status:active" # Active projects
rvn query "object:meeting .attendees:[[people/freya]]"

# Query traits
rvn query "trait:due"                     # All items with @due
rvn query "trait:due value:today"         # Due today
rvn query "trait:due value:past"          # Overdue items
rvn query "trait:priority value:high"     # High priority items
rvn query "trait:highlight"               # All highlighted items
rvn query "trait:highlight on:{object:book .status:reading}"
rvn query "trait:due refs:[[people/freya]]"  # Tasks referencing Freya

# Query with full-text content search
rvn query 'object:person content:"colleague"'   # People pages mentioning "colleague"
rvn query 'object:project content:"api design"' # Projects about API design

# Saved queries (defined in raven.yaml)
rvn query --list                          # List available saved queries
rvn query tasks                           # Run 'tasks' saved query
rvn query overdue                         # Run 'overdue' saved query
rvn query add my-tasks --traits due,status --filter status=todo  # Create saved query
rvn query remove my-tasks                 # Remove saved query

# Show backlinks to a note
rvn backlinks <target>
rvn backlinks people/freya
rvn backlinks daily/2025-02-01#standup

# Show index statistics
rvn stats

# List untyped pages (missing explicit type)
rvn untyped

# Open files by reference
rvn open cursor                  # Opens companies/cursor.md
rvn open companies/cursor        # Partial path also works
rvn open people/freya            # Opens people/freya.md

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
rvn new person "Freya"       # Creates people/freya-chen.md
rvn new project "Website"         # Creates projects/website.md, prompts for required fields
rvn new person                    # Prompts for title interactively

# Quick capture
rvn add "Call Odin about the Bifrost"
rvn add "@due(tomorrow) Send estimate"
rvn add "Idea" --to inbox.md      # Override destination

# Edit existing content
rvn edit "daily/2026-01-02.md" "old text" "new text" --confirm

# Move or rename files (with reference updates)
rvn move people/loki people/loki-archived
rvn move drafts/note.md projects/website/note.md --update-refs

# Delete objects (moves to .trash/ by default)
rvn delete people/loki

# Read raw file content
rvn read people/freya

# Full-text search
rvn search "meeting notes"
rvn search "api" --type project

# Watch for changes and auto-reindex
rvn watch

# Start MCP server for AI agents
rvn serve --vault-path /path/to/vault

# Start LSP server for editor integration
rvn lsp --vault-path /path/to/vault
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

**Filter operators** (work with all filters):

| Operator | Meaning | Example |
|----------|---------|---------|
| `a\|b` | OR | `"this-week\|past"` (due soon or overdue) |
| `!value` | NOT | `"!done"` (exclude done items) |

**Examples:**
```bash
rvn query "trait:due value:today"               # Items due today
rvn query "trait:due value:this-week"           # Items due this week
rvn query "trait:due value:past"                # Overdue items
rvn query "trait:due (value:this-week | value:past)"  # Due soon OR overdue
rvn query "trait:status !value:done"            # Everything except done
rvn query "trait:remind value:tomorrow"         # Reminders for tomorrow
rvn query overdue                               # Run saved 'overdue' query
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

### The `rvn query` Command

Unified interface for querying objects and traits:

```bash
rvn query "<query-string>"         # Run an ad-hoc query
rvn query <saved-query-name>       # Run a saved query
rvn query --list                   # List saved queries
```

**Object Query Examples**:
```bash
rvn query "object:person"                            # All people
rvn query "object:project"                           # All projects
rvn query "object:project .status:active"            # Active projects
rvn query "object:meeting .attendees:[[people/thor]]" # Thor's meetings
rvn query "object:project ancestor:{object:date}"    # Projects in daily notes
rvn query 'object:person content:"colleague"'        # People pages containing "colleague"
rvn query 'object:project content:"api"'             # Projects mentioning "api"
```

**Trait Query Examples**:
```bash
rvn query "trait:due"                                # All @due items
rvn query "trait:due value:today"                    # Due today
rvn query "trait:due value:past"                     # Overdue items
rvn query "trait:remind value:this-week"             # This week's reminders
rvn query "trait:highlight"                          # All highlights
rvn query "trait:highlight on:{object:meeting}"      # Highlights in meetings
rvn query "trait:due refs:[[people/freya]]"          # Tasks that reference Freya
rvn query "trait:highlight refs:{object:project}"    # Highlights referencing any project
```

**Boolean Logic**:
```bash
rvn query "object:project (.status:active | .status:planning)"  # Active or planning
rvn query "trait:due !value:done"                               # Not done
rvn query "object:project .status:active has:{trait:due}"       # Active with due dates
```

**Flags**:
```bash
rvn query "object:person" --json           # Machine-readable JSON
rvn query "object:person" --ids            # IDs only (one per line, for piping)
rvn query "..." --refresh                  # Refresh stale files before query
```

**Bulk Operations** (`--apply`):
```bash
# Preview bulk operation on query results
rvn query "trait:todo" --apply "set status=done"

# Apply the operation
rvn query "trait:todo" --apply "set status=done" --confirm

# Other supported commands: add, delete, move
rvn query "object:project .status:archived" --apply "delete" --confirm
```

**Output Example** (object query):
```
# person (2)

• people/freya
  email: freya@asgard.realm, name: Freya
  people/freya.md:1
• people/thor
  email: thor@asgard.realm, name: Thor Odinson
  people/thor.md:1
```

**Listing available types and traits**:
Use `rvn stats` to see available types and traits with counts:
```bash
rvn stats
```

### The `rvn add` Command

Quick capture for low-friction note-taking:

```bash
rvn add <text>
rvn add <text> --to <file-or-reference>
rvn add --stdin <text> [--confirm]
```

**Examples**:
```bash
# Single file
rvn add "Call Odin about the Bifrost"
rvn add "@due(tomorrow) @priority(high) Send estimate"
rvn add "Project idea" --to inbox.md
rvn add "Meeting notes" --to cursor           # Resolves to companies/cursor.md
rvn add "Update" --to companies/cursor        # Partial path also works

# Bulk operations via stdin
rvn query "object:project .status:active" --ids | rvn add --stdin "Review scheduled"
rvn query "object:project .status:active" --ids | rvn add --stdin "@reviewed" --confirm
```

**Behavior**:
- By default, appends to today's daily note (creates if needed)
- The `--to` flag accepts file paths, short references (`cursor`), or partial paths (`companies/cursor`)
- Traits in the text are preserved and indexed
- Timestamps are added by default

**Configuration** (in `raven.yaml`):
```yaml
capture:
  destination: daily      # "daily" or a file path
  heading: "## Captured"  # Append under this heading (optional)
  timestamp: false        # Prefix with time (default: false, use --timestamp flag)

# Note: auto_reindex (top-level setting, default: true) controls whether
# the file is reindexed after capture. See vault configuration section.
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

Update fields on existing objects (both file-level and embedded types):

```bash
rvn set <object_id> <field=value>...
rvn set --stdin <field=value>... [--confirm]
```

**Examples**:
```bash
# Single object
rvn set people/freya email=freya@asgard.realm
rvn set people/freya name="Freya" status=active
rvn set projects/website priority=high

# Embedded objects (use # to reference)
rvn set "daily/2026-01-09#standup" status=done
rvn set "daily/2026-01-09#planning-session" attendees='[[[people/freya]]]'

# Bulk operations via stdin (preview mode by default)
echo -e "people/freya\npeople/thor" | rvn set --stdin status=active

# Bulk with confirmation to apply
rvn query "@status(active)" --ids | rvn set --stdin status=archived --confirm

# Query with --apply for bulk set
rvn query "@status(todo)" --apply "set status=done" --confirm
```

**Behavior**:
- Validates fields against the object's type schema
- Warns (but allows) unknown fields
- Preserves existing fields not being updated
- For embedded types: updates the `::type()` declaration line in the containing file

**Bulk Operations** (`--stdin`):
- Reads object IDs from stdin (one per line)
- Without `--confirm`: shows preview of changes
- With `--confirm`: applies changes to all objects
- Supports both file-level and embedded object IDs

**JSON output** (for agents):
```json
{
  "ok": true,
  "data": {
    "file": "people/freya.md",
    "object_id": "people/freya",
    "type": "person",
    "updated_fields": {"email": "freya@asgard.realm"}
  }
}
```

### The `rvn edit` Command

Surgical text replacement in vault files:

```bash
rvn edit <path> <old_str> <new_str> [--confirm]
```

**Examples**:
```bash
# Preview an edit (default)
rvn edit "daily/2026-01-02.md" "- Churn analysis" "- [[churn-analysis|Churn analysis]]"

# Apply the edit
rvn edit "daily/2026-01-02.md" "reccommendation" "recommendation" --confirm

# Delete text (empty new_str)
rvn edit "daily/2026-01-02.md" "- old task" "" --confirm
```

**Behavior**:
- The `old_str` must appear exactly once in the file (prevents ambiguous edits)
- Without `--confirm`, shows a preview of the change
- With `--confirm`, applies the edit and reindexes the file
- Whitespace matters—match exactly including indentation

**Use cases**:
- Add wiki links to existing text
- Fix typos across files
- Add traits to existing content
- Remove lines

**JSON output** (preview mode):
```json
{
  "ok": true,
  "data": {
    "status": "preview",
    "path": "daily/2026-01-02.md",
    "line": 8,
    "preview": {
      "before": "\n- Churn analysis\n\n## Notes",
      "after": "\n- [[churn-analysis|Churn analysis]]\n\n## Notes"
    }
  }
}
```

### The `rvn move` Command

Move or rename files within the vault with automatic reference updates:

```bash
rvn move <source> <destination> [--update-refs] [--force]
rvn move --stdin <destination-directory/> [--confirm]
```

**Examples**:
```bash
# Rename a file
rvn move people/loki people/loki-archived

# Move to a different directory
rvn move inbox/task.md projects/website/task.md

# Move with reference updates (default behavior)
rvn move drafts/person.md people/freya.md --update-refs

# Bulk move via stdin (destination must be a directory ending with /)
rvn query "object:project .status:archived" --ids | rvn move --stdin archive/projects/
rvn query "object:project .status:archived" --ids | rvn move --stdin archive/projects/ --confirm
```

**Behavior**:
- Both source and destination must be within the vault (security constraint)
- By default, updates all references to the moved file (`--update-refs` defaults to true)
- Warns if moving to a type's default directory with mismatched type
- Creates destination directories if needed
- Reindexes affected files after move

**Flags**:
- `--update-refs`: Update all references to the moved file (default: true)
- `--force`: Skip confirmation prompts
- `--skip-type-check`: Skip type-directory mismatch warning
- `--stdin`: Read object IDs from stdin for bulk moves
- `--confirm`: Apply changes (preview only without this flag)

**JSON output**:
```json
{
  "ok": true,
  "data": {
    "source": "people/loki.md",
    "destination": "people/loki-archived.md",
    "refs_updated": 5
  }
}
```

### The `rvn delete` Command

Delete objects from the vault:

```bash
rvn delete <object_id> [--permanent] [--force]
rvn delete --stdin [--confirm]
```

**Examples**:
```bash
# Move to .trash/ (default, recoverable)
rvn delete people/loki

# Permanently delete (no trash)
rvn delete people/loki --permanent

# Skip confirmation
rvn delete people/loki --force

# Bulk delete via stdin
rvn query "object:project .status:archived" --ids | rvn delete --stdin
rvn query "object:project .status:archived" --ids | rvn delete --stdin --confirm
```

**Behavior**:
- By default, moves files to `.trash/` directory (recoverable)
- With `--permanent`, permanently deletes the file
- Warns about existing backlinks to the deleted object
- Updates the index after deletion

**Flags**:
- `--permanent`: Permanently delete instead of moving to trash
- `--force`: Skip confirmation prompts
- `--stdin`: Read object IDs from stdin for bulk deletes
- `--confirm`: Apply changes (preview only without this flag)

**JSON output**:
```json
{
  "ok": true,
  "data": {
    "object_id": "people/loki",
    "file": "people/loki.md",
    "action": "trashed",
    "backlinks_warned": 3
  }
}
```

### The `rvn search` Command

Full-text search across vault content:

```bash
rvn search <query> [--type <type>] [--limit <n>]
```

**Examples**:
```bash
rvn search "meeting notes"              # Find pages with both words
rvn search '"team meeting"'             # Exact phrase match
rvn search "meet*"                      # Prefix matching
rvn search "meeting AND notes"          # Boolean AND
rvn search "meeting OR notes"           # Boolean OR
rvn search "meeting NOT private"        # Boolean NOT
rvn search "api" --type project         # Filter by type
rvn search "freya" --limit 5            # Limit results
```

**Behavior**:
- Uses SQLite FTS5 for fast full-text search
- Results ranked by relevance
- Returns snippets showing matched content
- Supports prefix matching, phrases, and boolean operators

**JSON output**:
```json
{
  "ok": true,
  "data": {
    "query": "meeting notes",
    "items": [
      {
        "object_id": "daily/2026-01-02",
        "type": "date",
        "file_path": "daily/2026-01-02.md",
        "snippet": "...discussed the <b>meeting</b> <b>notes</b> from..."
      }
    ]
  },
  "meta": { "count": 1, "query_time_ms": 5 }
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
```

Queries can include `types`, `traits`, or both.

Then run them:

```bash
rvn query tasks              # Run saved query (traits)
rvn query people             # Run saved query (types)
rvn query project-summary    # Run saved query (mixed)
rvn query --list             # List all saved queries
rvn query add my-tasks --traits due,status --filter status=todo  # Create
rvn query remove my-tasks    # Remove
```

For direct ad-hoc queries, use the query string syntax:

```bash
rvn query "trait:due value:past"   # All overdue items
rvn query "trait:highlight"        # All highlights
rvn query "object:person"          # All people
rvn query "object:project"         # All projects
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
- Field value doesn't match schema type (including date/datetime format)
- Enum value not in allowed list
- Number outside min/max bounds
- Reference to non-existent object
- Wrong target type for `ref` fields (e.g., `lead: ref, target: person` pointing to a `project`)
- Duplicate object IDs
- Ambiguous short reference (multiple matches)
- Unknown frontmatter key for type
- Missing target type in schema (ref field references non-existent type)
- Duplicate alias (multiple objects using the same alias)
- Alias collision (alias conflicts with existing short name or object ID)

**Warnings** (informational):
- Undefined trait (not in schema, will be skipped)
- Stale index (files modified since last reindex)
- Unused types in schema (defined but never used)
- Unused traits in schema (defined but never used)
- Self-referential required fields (impossible to create first instance)
- Short refs that could be full paths (for clarity)

**Flags**:
- `--strict`: Treat warnings as errors (exit code 1 if any warnings)
- `--create-missing`: Interactively create missing referenced pages and undefined traits
- `--by-file`: Group issues by file path for better readability
- `--json`: Output structured JSON for programmatic use

**Output**:
```bash
$ rvn check
Checking vault: /path/to/vault
WARN:  Index may be stale (3 file(s) modified since last reindex)
       Run 'rvn reindex' to update the index.

ERROR:  projects/bifrost.md:8 - Reference [[freya]] is ambiguous (matches: people/freya, clients/freya)
ERROR:  projects/mobile.md:5 - Field 'lead' expects type 'person', but [[projects/website]] is type 'project'
WARN:   notes/random.md:23 - Undefined trait '@custom'
WARN:   [schema] Type 'vendor' is defined in schema but never used
WARN:   [schema] Trait '@archived' is defined in schema but never used

Found 2 error(s), 3 warning(s) in 847 files.
```

**Grouped output** (`--by-file`):
```bash
$ rvn check --by-file
Checking vault: /path/to/vault

schema.yaml:
  WARN: Type 'vendor' is defined in schema but never used

projects/bifrost.md (1 errors):
  Line 8: ERROR - Reference [[freya]] is ambiguous

Found 1 error(s), 1 warning(s) in 847 files.
```

**Create missing references** (`--create-missing`):

When references point to non-existent files, `rvn check --create-missing` offers to create them interactively:

```bash
$ rvn check --create-missing

--- Missing References ---

Certain (from typed fields):
  • people/baldur → person (from daily/2025-01-01#team-sync.attendees)
  • people/heimdall → person (from daily/2025-01-01#team-sync.attendees)

Create these pages? [Y/n] y
  ✓ Created people/baldur.md (type: person)
  ✓ Created people/dan.md (type: person)

Unknown type (please specify):
  ? notes/random-idea (referenced in projects/website.md:15)

Available types: meeting, person, project

Type for notes/random-idea (or 'skip'): vendor
  Type 'vendor' doesn't exist. Would you like to create it? [y/N] y
  Default path for 'vendor' files (e.g., 'vendors/', or leave empty): vendors/
  ✓ Created type 'vendor' in schema.yaml
  ✓ Created vendors/random-idea.md (type: vendor)

✓ Created 2 missing page(s).
```

**Add undefined traits to schema** (`--create-missing`):

When undefined traits are found, you'll be prompted to add them to your schema:

```bash
--- Undefined Traits ---

  @custom (3 uses, e.g., daily/2026-01-02.md:15)
    Used bare (no value) - suggesting: boolean

Add '@custom' to schema? [y/N] y
  Trait type (boolean/string/date/enum) [boolean]: 
  ✓ Added trait 'custom' to schema.yaml

✓ Added 1 trait(s) to schema.
```

Trait type is inferred: bare usage suggests `boolean`, usage with values suggests `string`.

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

7. **Reference Extractor**
   - Find all `[[ref]]` and `[[ref|display]]` patterns
   - Handle array syntax `[[[ref1]], [[ref2]]]` correctly
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
    - `rvn query` (unified query interface for objects and traits)
    - `rvn backlinks`
    - `rvn stats`
    - `rvn untyped`
    - `rvn daily`
    - `rvn date`
    - `rvn new` (create typed object)
    - `rvn add` (quick capture)
    - `rvn set` (update frontmatter fields)
    - `rvn delete` (delete object, moves to trash)
    - `rvn move` (move/rename with reference updates)
    - `rvn read` (read raw file content)
    - `rvn edit` (surgical text replacement)
    - `rvn search` (full-text search)
    - `rvn path` (print vault path)
    - `rvn vaults` (list configured vaults)
    - `rvn schema` (introspect schema)
    - `rvn schema add type/trait/field` (add to schema)
    - `rvn schema update type/trait/field` (modify schema)
    - `rvn schema remove type/trait/field` (remove from schema)
    - `rvn schema validate` (validate schema)
    - `rvn serve` (MCP server for AI agents)
    - `rvn lsp` (LSP server for editor integration)
    - `rvn watch` (file watcher with auto-reindex)

### Phase 2: Enhanced Querying ✅

1. **Query Language** ✅
   - Parse query strings like `object:meeting .attendees:[[freya]]`
   - Support field filters with JSON extraction
   - Support date ranges (`trait:due value:this-week`)
   - Support parent filters and nested queries

2. **Full-Text Search** ✅
   - FTS5 virtual table with BM25 ranking
   - Index content for text search via `rvn search`
   - Supports phrases, prefix matching, and boolean operators

3. **Output Formatting** ✅
   - JSON output for scripting (`--json`)
   - Human-readable table format (default)

### Phase 3: File Watching & Live Index ✅

1. **File Watcher** ✅
   - Uses `fsnotify` package to watch vault directory
   - Debounce rapid changes
   - Incremental reindex on file change

2. **Background Service** ✅
   - `rvn watch` runs in background
   - Keeps index always up-to-date

### Phase 4: Refactoring Tools

1. **Reference Updates** ✅
   - When an object is renamed/moved, update all references
   - `rvn move <old-path> <new-path>` command (implemented)

2. **Note Promotion** (future)
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
| Embedded type | `::type(...)` | `::` distinguishes from traits (`@`), inline for speed |
| Traits | `@trait(...)` | `@` is intuitive for annotations |
| References | `[[path/file#id]]` | Wiki-style links, `#` for fragments (standard) |

### ID Strategy

| Object Type | ID Format | Example |
|-------------|-----------|---------|
| File-level | Path without extension | `people/freya` |
| Embedded (explicit) | Path + `#` + slugified heading | `daily/2025-02-01#weekly-standup` |
| Section (auto) | Path + `#` + slugified heading | `daily/2025-02-01#morning` |

- **IDs auto-generated**: Both embedded types and sections derive their ID from the slugified heading text
- **Optional ID override**: Explicit `id` field overrides the auto-generated slug: `::meeting(id=standup)` → `#standup`
- **Duplicate handling**: Counter suffix for same-slug headings (`#team-sync`, `#team-sync-2`)
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

### CLI Query Interface

All querying is unified under `rvn query`:

1. **Ad-hoc queries**: `rvn query "<query-string>"` for one-off queries
2. **Saved queries**: Define reusable queries in `raven.yaml` and run with `rvn query <name>`
3. **Typed results**: Queries explicitly specify return type (`object:` or `trait:`)

This unified approach keeps the CLI minimal while supporting powerful queries.

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

### Consistency Model

Raven uses an **eventually consistent** architecture where plain-text files are the source of truth and the SQLite index is a derived cache.

**Key properties:**

| Property | Behavior |
|----------|----------|
| **Source of truth** | Markdown files in the vault |
| **Index** | Derived cache, can be deleted and rebuilt |
| **Consistency** | Eventually consistent (index may lag behind files) |
| **Staleness** | Index becomes stale when files are edited externally |
| **Recovery** | Run `rvn reindex` to resync index with files |

**Closed World Assumption (CWA):**

Queries operate under the Closed World Assumption—if something isn't in the index, it doesn't exist from the query's perspective:

- `!has:due` returns objects where no `@due` trait is indexed
- If the index is stale, this may include objects that actually have `@due` in their files
- Run `rvn check` to detect undefined traits and `rvn reindex` to refresh

**When to reindex:**

| Scenario | Reindex needed? |
|----------|-----------------|
| Edits via `rvn add`, `rvn edit`, `rvn set` | No (auto-reindex if enabled) |
| Edits via external editor | Yes, unless `rvn watch` is running |
| Schema changes (new types/traits) | Yes |
| Bulk file operations (move, rename outside Raven) | Yes |
| Index corruption or upgrade | Yes (delete `.raven/` and reindex) |

**Concurrency:**

The current system is designed for single-writer access:

- Multiple readers are safe (SQLite WAL mode)
- Multiple writers may cause lost updates or stale reads
- Multi-agent workflows should serialize writes or use external coordination

**Future improvements** (see FUTURE.md):
- Optimistic locking (detect external changes before writing)
- Write-ahead journaling for multi-file operations
- File watching with auto-reindex

### Formal Grammar

The Raven file format can be described with the following grammar (EBNF notation):

```ebnf
(* Document structure *)
document     = [ frontmatter ], body ;
frontmatter  = "---", newline, yaml_content, "---", newline ;
body         = { line } ;

(* Frontmatter *)
yaml_content = (* Valid YAML key-value pairs *) ;

(* Line content *)
line         = [ heading | embedded_type | content ], newline ;
heading      = { "#" }, " ", text ;
embedded_type = "::", type_name, "(", field_list, ")" ;
content      = { text | trait | reference } ;

(* Embedded type declarations *)
type_name    = identifier ;
field_list   = field, { ",", field } ;
field        = identifier, "=", field_value ;
field_value  = string | number | boolean | reference | array ;
array        = "[", [ field_value, { ",", field_value } ], "]" ;

(* Traits *)
trait        = "@", trait_name, [ "(", trait_value, ")" ] ;
trait_name   = identifier ;
trait_value  = (* Value appropriate to trait type: date, enum, string, etc. *) ;

(* References *)
reference    = "[[", ref_path, [ "|", display_text ], "]]" ;
ref_path     = path_segment, { "/", path_segment }, [ "#", fragment ] ;
path_segment = { letter | digit | "-" | "_" } ;
fragment     = identifier ;
display_text = text ;

(* Primitives *)
identifier   = letter, { letter | digit | "-" | "_" } ;
string       = '"', { character }, '"' | "'", { character }, "'" ;
number       = [ "-" ], digit, { digit }, [ ".", { digit } ] ;
boolean      = "true" | "false" ;
date         = digit, digit, digit, digit, "-", digit, digit, "-", digit, digit ;
datetime     = date, "T", digit, digit, ":", digit, digit, [ ":", digit, digit ], [ timezone ] ;
timezone     = "Z" | ( "+" | "-" ), digit, digit, ":", digit, digit ;

(* Basic elements *)
letter       = "a" | ... | "z" | "A" | ... | "Z" ;
digit        = "0" | ... | "9" ;
newline      = "\n" | "\r\n" ;
text         = { character - newline } ;
character    = (* Any Unicode character *) ;
```

**Parsing precedence for traits:**

1. `@` must be preceded by whitespace or start of line
2. Trait name is parsed greedily until `(` or whitespace
3. Trait value extends to matching `)`, handling nested parens
4. Multiple traits on same line: each parsed independently left-to-right

**Embedded type constraints:**

1. `::type()` must appear within 2 lines after a heading
2. Object ID is auto-generated from the slugified heading text
3. Optional `id` field overrides the auto-generated slug
4. IDs must be unique within the file (duplicates get `-2`, `-3` suffix)

**Reference resolution:**

1. Aliases are checked first (if `[[goddess]]` is an alias for `people/freya`)
2. Full paths (`[[people/freya]]`) resolve directly
3. Short refs (`[[freya]]`) search for unique match
4. Date shorthand (`[[2025-02-01]]`) resolves to daily note directory
5. Fragment refs (`[[file#section]]`) resolve to embedded objects

**Ambiguity handling**: If a reference matches multiple things (e.g., an alias AND a short name), it's treated as ambiguous and requires a full path to resolve.

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
| `raven_add` | Append to existing file or daily note | `text`, optional `to` (supports references) |
| `raven_open` | Open file in editor by reference | `reference` (short name, partial path, or full path) |
| `raven_delete` | Delete object (trash by default) | `object_id` |
| `raven_move` | Move or rename object (updates references) | `source`, `destination`, optional `update-refs`, `force` |
| `raven_edit` | Surgical text replacement | `path`, `old_str`, `new_str`, optional `confirm` |
| `raven_search` | Full-text search | `query`, optional `type`, `limit` |
| `raven_query` | Query objects/traits or run saved query | `query_string` (ad-hoc) or `query_name` (saved) |
| `raven_query_add` | Create saved query | `name`, optional `traits`, `types`, `filter`, `description` |
| `raven_query_remove` | Remove saved query | `name` |
| `raven_backlinks` | Find references to object | `target` |
| `raven_date` | Get activity for date | `date` |
| `raven_check` | Validate vault against schema | optional `refresh` (reindex stale files first) |
| `raven_reindex` | Rebuild the index | optional `full` (force complete rebuild) |
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
| `raven_daily` | Open or create a daily note | optional `date` |
| `raven_untyped` | List pages without explicit type | (none) |
| `raven_workflow_list` | List available workflows | (none) |
| `raven_workflow_show` | Show workflow details | `name` |
| `raven_workflow_render` | Render workflow with context | `name`, optional `input` |

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

## LSP Server (Editor Integration)

Raven provides a Language Server Protocol (LSP) server for rich editor integration in IDEs like VS Code, Neovim, and Emacs.

### Starting the Server

```bash
rvn lsp --vault-path /path/to/vault
rvn lsp --debug  # With debug logging to stderr
```

The server communicates via JSON-RPC over stdin/stdout (standard LSP transport).

### Supported Features

| Feature | Description |
|---------|-------------|
| **Completion** | Autocomplete for `[[references]]` and `@traits` |
| **Go to Definition** | Jump to referenced files and objects |
| **Hover** | Show object type and frontmatter fields on hover |
| **Diagnostics** | Real-time validation errors and warnings |

### Editor Configuration

**VS Code** (add to `.vscode/settings.json`):
```json
{
  "raven.serverPath": "/path/to/rvn",
  "raven.vaultPath": "/path/to/vault"
}
```

**Neovim** (with nvim-lspconfig):
```lua
require('lspconfig').raven.setup({
  cmd = { 'rvn', 'lsp', '--vault-path', '/path/to/vault' },
  filetypes = { 'markdown' },
})
```

---

## Appendix: Example Vault

```
vault/
├── schema.yaml
├── daily/
│   ├── 2025-02-01.md
│   └── 2025-02-02.md
├── people/
│   ├── freya.md
│   └── thor.md
├── projects/
│   ├── website-redesign.md
│   └── mobile-app.md
├── books/
│   └── poetic-edda.md
└── meetings/
    └── weekly-standup.md    # Recurring meeting series
```

### Sample: `people/freya.md`

```markdown
---
type: person
name: Freya
email: freya@asgard.realm
alias: goddess
---

# Freya

Senior engineer on the platform team.

(Note: Can also be referenced as `[[goddess]]` due to the alias field.)

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
| `people/freya` | `person` | (file-level) |
| `people/freya#early-life` | `section` | Early Life |
| `people/freya#notes` | `section` | Notes |
| `people/freya#1-1-topics` | `section` | 1:1 Topics |

### Sample: `daily/2025-02-01.md`

```markdown
---
type: daily
date: 2025-02-01
---

# Saturday, February 1, 2025

## Morning

Reviewed [[projects/website-redesign]] progress. Looking good.

## Weekly Standup
::meeting(id=standup, time=09:00, attendees=[[[people/freya]], [[people/thor]]])

Discussed Q2 priorities.

- @due(2025-02-03) Follow up on timeline
- [[people/freya]] will send updated estimates

## Afternoon

- @due(2025-02-02) @priority(high) Review PR #1234
- @remind(2025-02-02T14:00) Call with vendor

## Reading

Chapter 2 of [[books/poetic-edda]].

- @highlight The world tree Yggdrasil connects all nine realms
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
