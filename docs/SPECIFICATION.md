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
| **Traits** | Add behavior/metadata to content | `@trait(...)` | No (queryable, not referenceable) | @task, @remind, @highlight |

### Types vs Sections vs Traits Mental Model

- **Types are nouns** (declared with `::` or frontmatter): A `person` is a thing. A `meeting` is a thing. They exist, have identity, can be linked to.
- **Sections are structural nouns** (auto-created from headings): Every markdown heading becomes a `section` object automatically. This ensures the entire document structure is captured in the object model.
- **Traits are adjectives/verbs** (declared with `@`): `@task` marks content as having task-like behavior. `@highlight` marks something as important. They modify content, don't create new entities.

### Built-in Types

| Type | Purpose | Auto-created? |
|------|---------|---------------|
| `page` | Fallback for files without explicit type | Yes, when no type declared and no detection rule matches |
| `section` | Fallback for headings without explicit type | Yes, for every heading without `::type()` |

**Why built-in?** These types ensure every structural element is represented in the object model. Without them, files and headings without explicit types would have no type, breaking queries and parent resolution.

**Not in schema.yaml**: These types are added programmatically and don't need to appear in your `schema.yaml`. They have no special behavior beyond being fallback types—they simply provide a type label so the system works consistently.

**Customizable**: If you add `page` or `section` definitions to your `schema.yaml`, your definitions will be used, allowing you to add custom fields or detection rules.

### Files as Source of Truth

- All data lives in plain markdown files
- The SQLite index is a derived, disposable cache
- Delete the index, run reindex, everything is restored
- Files can be edited with any text editor
- Files sync via Dropbox/iCloud/git without conflicts (index is local-only)

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

- @task(due=2025-02-03) Send estimate    ← trait, parent is the meeting
- Regular bullet point                    ← just content, not a trait

## Random Notes

- @task(due=2025-02-05) Unrelated task   ← trait, parent is the file root
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

Inline annotations using `@name(...)` syntax:

```markdown
- @task(due=2025-02-01, priority=high) Complete the report
- @remind(2025-02-05T09:00) Follow up on this
- @highlight This is an important insight
```

**Rules**:
- Traits can appear anywhere in content
- The annotated content is the text between surrounding carriage returns (the line or paragraph)
- Traits without parentheses are boolean: `@highlight` is equivalent to `@highlight()`
- Multiple traits can appear on one line: `@task(due=2025-02-01) @highlight Fix the bug`
- Undefined traits (not in `schema.yaml`) trigger a warning during `rvn check` and are skipped

**Positional arguments**: Traits can define positional fields. Positional args must come before named args:
```markdown
@remind(2025-02-05T09:00)                    # positional: 'at' field
@remind(2025-02-05T09:00, recurring=true)    # positional + named
```

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
| Syntax | `#name` | `@name(...)` |
| Attaches to | Object (aggregated) | Specific line/content |
| Has fields | No | Yes |
| Use case | Categorization | Behavior/metadata |
| Example | `#productivity` | `@task(due=2025-02-01)` |

### Implementation Notes

1. **Extraction**: Parse `#([\w-]+)` patterns from content, plus `tags:` array from frontmatter
2. **Aggregation**: Collect all tags within an object's scope, plus inherited tags from children
3. **Deduplication**: Store unique tags only
4. **Storage**: Add to object's `fields.tags` as JSON array during indexing

---

## Schema Configuration

### File: `schema.yaml`

Located at vault root. Defines all types and traits.

```yaml
types:
  # Built-in: Fallback type for files without explicit type or detection match
  page:
    fields: {}

  # Built-in: Auto-created for every heading without explicit ::type()
  section:
    fields:
      title:
        type: string
      level:
        type: number
        min: 1
        max: 6

  person:
    fields:
      name:
        type: string
        required: true
      email:
        type: string
      company:
        type: ref
        target: company
    detect:
      path_pattern: "^people/"

  project:
    fields:
      status:
        type: enum
        values: [active, paused, completed, abandoned]
        default: active
      lead:
        type: ref
        target: person
      due:
        type: date
      technologies:
        type: string[]      # Array of strings

  daily:
    fields:
      date:
        type: date
        derived: from_filename
    detect:
      path_pattern: "^daily/\\d{4}-\\d{2}-\\d{2}\\.md$"

  meeting:
    fields:
      time:
        type: datetime
      attendees:
        type: ref[]
        target: person
      recurring:
        type: ref
        target: meeting_series
      
  book:
    fields:
      title:
        type: string
      author:
        type: ref
        target: person
      status:
        type: enum
        values: [to_read, reading, finished, abandoned]
      rating:
        type: number
        min: 1
        max: 5

traits:
  task:
    fields:
      due:
        type: date
      priority:
        type: enum
        values: [low, medium, high]
        default: medium
      assignee:
        type: ref
        target: person
      status:
        type: enum
        values: [todo, in_progress, done]
        default: todo
    cli:
      alias: tasks                              # Creates `rvn tasks` command
      default_query: "status:todo OR status:in_progress"

  remind:
    fields:
      at:
        type: datetime
        positional: true  # First arg without key: @remind(2025-02-01T09:00)
    cli:
      alias: reminders                          # Creates `rvn reminders` command
      default_query: "at:>=now"

  highlight:
    fields:
      color:
        type: enum
        values: [yellow, red, green, blue]
        default: yellow
    # No cli alias - use `rvn trait highlight`
```

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

### Detection Rules

Auto-detect type without explicit `type:` field:

```yaml
detect:
  path_pattern: "^daily/\\d{4}-\\d{2}-\\d{2}\\.md$"  # Regex on file path
  attribute: { status: active }                       # Match frontmatter attributes
```

**Detection methods** (only these two are supported):
- `path_pattern`: Regex matched against file path from vault root
- `attribute`: Match specific frontmatter fields/values

**Priority**: Explicit `type:` in frontmatter always takes precedence over detection rules. Detection is a convenience fallback.

**Conflict handling**: If a file has explicit `type:` that differs from what detection would infer, `rvn check` emits a warning (in case it's a mistake).

**Fallback**: Files that don't match any detection rule and have no explicit type are assigned the `page` type.

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
@task(due=2025-02-01, priority=high) Complete the report
  │    │               └── named argument
  │    └── positional argument (if schema defines positional field)
  └── trait name
```

### Value Syntax

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
| Boolean (implicit) | `@name` | `@highlight` (means highlight=true) |

### Argument Order

Positional arguments must come before named arguments (Python-style):

```markdown
@remind(2025-02-05T09:00)                    # positional only
@remind(2025-02-05T09:00, recurring=true)    # positional + named
@remind(at=2025-02-05T09:00, recurring=true) # all named (also valid)
```

### Complete Example

```markdown
---
type: daily
date: 2025-02-01
tags: [work]
---

# Saturday, February 1, 2025

Morning coffee, reviewed [[projects/website-redesign]].

## Weekly Standup
::meeting(id=standup, time=09:00, attendees=[[[people/alice]], [[people/bob]]], recurring=[[meetings/weekly-standup]])

Discussed Q2 roadmap. [[people/alice]] raised concerns about timeline.

- @task(due=2025-02-03, assignee=[[people/alice]]) Send revised estimate
- Agreed to revisit next week
- @highlight Key insight: we need more buffer time

## 1:1 with Bob
::meeting(id=one-on-one-bob, time=14:00, attendees=[[[people/bob]]])

Talked about his career growth.

- @task(due=2025-02-10) Write up promotion case
- He's interested in the tech lead role on [[projects/mobile-app]]

## Reading

Started [[books/atomic-habits]] by [[people/james-clear]].

- @highlight Habits are compound interest for self-improvement #productivity
- @task(due=2025-02-15) Finish chapter 3

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
├── config/
│   └── config.go            # Load ~/.config/raven/config.toml
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
│   └── queries.go           # Query builder
├── check/
│   └── validator.go         # Vault-wide validation (rvn check)
└── cli/
    ├── root.go              # Cobra root command setup
    ├── init.go              # rvn init
    ├── reindex.go           # rvn reindex
    ├── check.go             # rvn check
    ├── tasks.go             # rvn tasks
    ├── trait.go             # rvn trait
    ├── query.go             # rvn query
    ├── backlinks.go         # rvn backlinks
    ├── stats.go             # rvn stats
    ├── untyped.go           # rvn untyped
    ├── daily.go             # rvn daily
    └── new.go               # rvn new
```

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

# Reindex all files
rvn reindex

# Query traits (generic form)
rvn trait <name> [filters]
rvn trait task                           # All tasks
rvn trait task --status todo             # Filter by field
rvn trait task --due this-week           # Date range filter
rvn trait remind --at today              # Reminders due today
rvn trait highlight --color red          # Highlights by color

# Schema-defined alias for tasks (defined via cli.alias in schema.yaml)
rvn tasks                                # Alias: rvn trait task (with schema's default_query)
rvn tasks --all                          # Include completed tasks
rvn tasks --due today
rvn tasks --assignee [[people/alice]]

# User-defined aliases (via schema.yaml cli.alias)
rvn reminders                            # If configured in schema

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

# Open/create today's daily note
rvn daily

# Create a new typed note
rvn new --type person "Alice Chen"
rvn new --type project "Website Redesign"

# Watch for changes and auto-reindex (future)
rvn watch

# Start local web UI (future)
rvn serve
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

### Trait CLI Aliases

Aliases provide ergonomic shortcuts for common trait queries. The `task` trait has a built-in alias:

```bash
rvn tasks                    # Built-in alias
```

Additional aliases can be defined in `schema.yaml` (see Schema Configuration).

### Trait CLI Configuration

Traits can define CLI shortcuts via the `cli` key:

```yaml
traits:
  my_trait:
    fields: { ... }
    cli:
      alias: mytraits              # Creates `rvn mytraits` command
      default_query: "field:value" # Default filter (can be overridden)
```

| Property | Description |
|----------|-------------|
| `alias` | Creates a top-level command `rvn <alias>` |
| `default_query` | Default filter applied when using the alias |

**Note**: CLI aliases are defined in the schema. The default `schema.yaml` created by `rvn init` includes `task` with `cli.alias: tasks`.

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
- Detection rule would infer different type than explicit `type:`
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
    - `rvn tasks`
    - `rvn backlinks`
    - `rvn query`
    - `rvn stats`
    - `rvn untyped`

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

### Detection Priority

1. Explicit `type:` in frontmatter (always wins)
2. `path_pattern` detection rules
3. `attribute` detection rules
4. Fallback to `page` type

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
- @task(due=2025-02-01) Send her the API docs

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

- @task(due=2025-02-03) Follow up on timeline
- [[people/alice]] will send updated estimates

## Afternoon

- @task(due=2025-02-02) Review PR #1234
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
