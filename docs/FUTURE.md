# Future Enhancements

This document tracks potential future enhancements and ideas. These are not currently planned for implementation but are worth considering for later.

---

## Content Migration & Refactoring

As vaults grow and evolve, users need ways to restructure content without losing data or breaking references. These operations go beyond simple file moves.

---

### Extract Embedded Object to Standalone File

Convert an embedded (heading-based) object into its own file with proper frontmatter.

**Use case**: A meeting note has a project discussion that deserves its own page:
```markdown
# Daily Note 2025-01-04

## Bifrost Redesign ::project()
status: planning
---
This needs to be its own file...
```

**Proposed command:**
```bash
rvn extract "daily/2025-01-04#bifrost-redesign" --to "projects/bifrost-redesign"

# What happens:
# 1. Creates projects/bifrost-redesign.md with extracted content
# 2. Converts inline fields to YAML frontmatter
# 3. Replaces original section with [[projects/bifrost-redesign]]
# 4. Updates all references to the old embedded ID
```

**Challenges:**
- Determining what content belongs to the section (to next heading? to end of file?)
- Converting inline `::type()` fields to YAML frontmatter
- Preserving nested headings (demote levels?)
- Updating references like `[[daily/2025-01-04#bifrost-redesign]]` → `[[projects/bifrost-redesign]]`

**Status**: Not implemented.

---

### Promote Trait to Object Type

Convert inline trait annotations into full object files with proper schema.

**Use case**: You've been using `@task(description)` inline:
```markdown
- @task(Review PR) @due(2025-01-05)
- @task(Write docs) @due(2025-01-06)
```

Now you want tasks as first-class objects:
```markdown
// tasks/review-pr.md
---
type: task
title: Review PR
due: 2025-01-05
status: pending
---
```

**Proposed workflow:**
```bash
rvn promote "trait:task" --to-type task --interactive

# For each @task instance:
# 1. Show context (the line, surrounding content)
# 2. Prompt: [e]xtract to file | [s]kip | [q]uit
# 3. If extract: generate filename, create file, replace with reference

# Or batch mode for well-structured traits:
rvn promote "trait:task" --to-type task --auto
```

**Challenges:**
- What content comes with the trait? Just the line? The paragraph? User decision.
- Schema evolution: need to add `task` type to schema.yaml first (or auto-add?)
- Trait value → field mapping: `@task(Review PR)` → which field gets "Review PR"?
- Reference replacement: replace `@task(Review PR)` with `[[tasks/review-pr]]`

**Implementation ideas:**
- `--context paragraph` or `--context line` to control scope
- `--field-mapping "value:title,due:due"` to map trait args to fields
- Schema modification: `rvn schema add type task` first, or prompt during promotion

**Status**: Not implemented.

---

### Demote Object to Trait

The reverse operation—convert standalone object files back to inline traits.

**Use case**: Over-structured. Every small task is a file. Want to simplify to inline `@todo` traits.

```bash
rvn demote "object:task .status:done" --to-trait todo --delete-files

# For each matching task object:
# 1. Find all references [[tasks/review-pr]]
# 2. Replace with @todo(Review PR)
# 3. Optionally delete the task file
```

**Challenges:**
- Where do replaced traits go? At the reference location?
- Field → trait value mapping (reverse of promotion)
- File deletion safety (prompt? trash folder?)

**Status**: Not implemented.

---

### Split Large File

A note has grown unwieldy. Split it into multiple files at heading boundaries.

```bash
rvn split "big-note.md" --by-heading --level 2

# Creates:
# - big-note/section-1.md
# - big-note/section-2.md
# - big-note.md (becomes index/TOC with links)
```

**Challenges:**
- What becomes of the parent file? Delete? Convert to index?
- Heading level adjustment (H2 → H1 in new files?)
- Internal links between sections need updating
- Frontmatter: inherit from parent? Generate new?

**Status**: Not implemented.

---

### Merge Related Objects

Consolidate scattered notes into one comprehensive page.

```bash
rvn merge "notes/idea-1.md" "notes/idea-2.md" --into "projects/big-idea.md"

# What happens:
# 1. Create projects/big-idea.md with combined content
# 2. Update all references to the source files
# 3. Optionally delete source files
```

**Challenges:**
- Frontmatter merging (which values win?)
- Content ordering (chronological? alphabetical?)
- Heading level conflicts
- Duplicate content detection

**Status**: Not implemented.

---

### Type Conversion

Change an object from one type to another, mapping fields.

```bash
rvn convert "notes/idea.md" --to-type article --field-map "content:body"

# Updates frontmatter:
# - type: note → type: article
# - Maps fields according to schema compatibility
# - Warns about missing required fields
```

**Status**: Not implemented.

---

### Reference Rewriting Primitive

The building block that enables all of the above: update all references from one ID to another.

```bash
rvn refs update "old/path#section" --to "new/path"

# Finds all [[old/path#section]] references in vault
# Replaces with [[new/path]]
# Reports: "Updated 12 references in 8 files"
```

**Why useful:**
- `rvn move` already does this internally
- Exposing as primitive enables agent composition
- All migration operations need this

**Status**: Internal functionality exists in `rvn move`. Not exposed as standalone command.

---

### Agent-Composable Approach

Rather than building complex CLI commands for every migration pattern, expose primitives that agents can orchestrate:

1. `rvn query` - find matching content
2. `rvn read` - read source files
3. `rvn new` - create new files
4. `rvn edit` - transform content
5. `rvn move` - rename/move with reference updates
6. `rvn delete` - remove old files
7. `rvn refs update` (proposed) - update references

Agents can combine these to handle complex migrations with human-in-the-loop confirmation.

**Example agent workflow for promotion:**
```
Agent:
  1. Query all @task traits
  2. For each, ask user: "Promote to task object?"
  3. If yes: create task file, edit source to replace trait with reference
  4. Report: "Promoted 15 tasks, skipped 3"
```

**Status**: Most primitives exist. Missing `rvn refs update` as standalone command.

---

## Migrations

### Automatic Syntax Migration
Implement `rvn migrate --syntax` to automatically convert deprecated trait syntax:
```bash
rvn migrate --syntax --dry-run  # Preview
rvn migrate --syntax            # Apply

# Transforms:
# @task(due=2025-02-01, priority=high) → @due(2025-02-01) @priority(high)
# @remind(at=2025-02-01T09:00) → @remind(2025-02-01T09:00)
```

**Current status**: Framework exists (`rvn migrate`), but automatic file transformation not yet implemented.

---

## CLI Improvements

### Configurable Query Output Formatting

Allow users to configure how query results are displayed, particularly for trait queries where content length matters for readability.

**Use case**: When using traits for task management, users need to see enough content to identify specific tasks at a glance.

**Proposed configuration** (`raven.yaml`):
```yaml
# Output formatting
output:
  # Content column width for trait queries (default: 65)
  content_width: 80
  
  # Trait column width (default: 18)
  trait_width: 20
  
  # Location column width (default: 25)
  location_width: 30
  
  # Or use "auto" to adapt to terminal width
  # content_width: auto
```

**Alternative**: Command-line flags for ad-hoc adjustment:
```bash
rvn query tasks --content-width 80
rvn query "trait:due" --wide  # Use full terminal width
```

**Status**: Not implemented. Currently hardcoded to 65 characters for content column.

---

### Custom Query Output Columns

Allow users to define what columns appear in query results, enabling richer table views with specific fields.

**Use cases:**
- Show `@due`, `@status`, and `@priority` together for task management
- Display object fields like `lead`, `status` alongside the object name
- Include custom computed columns (e.g., days until due)

#### Approach A: Column Flags

Simple flag-based column selection:

```bash
# Specify columns to display
rvn query "trait:due" --columns content,value,status,location

# For object queries, pull in field values
rvn query "object:project" --columns name,status,lead,due

# Shorthand for common patterns
rvn query tasks --columns +status,+priority  # Add to defaults
```

**Pros**: Simple, no new syntax
**Cons**: Verbose for complex cases, no persistence

---

#### Approach B: Named Views in Config

Define reusable output formats in `raven.yaml`:

```yaml
views:
  task-board:
    query: "trait:due | trait:status"
    columns:
      - name: content
        width: 50
      - name: due
        source: trait.due.value  # Extract from @due trait
        width: 12
      - name: status
        source: trait.status.value
        width: 10
      - name: priority
        source: trait.priority.value
        width: 8
    sort: due

  project-overview:
    query: "object:project"
    columns:
      - name: project
        source: id
        width: 20
      - name: status
        source: field.status
      - name: lead
        source: field.lead
      - name: due
        source: field.due
```

```bash
rvn view task-board
rvn view project-overview
```

**Pros**: Reusable, declarative, powerful
**Cons**: More complex config, new command

---

#### Approach C: SQL-like SELECT in Query Language

Extend the query language with projection:

```bash
# Select specific fields
rvn query "object:project .status:active SELECT name, status, lead, due"

# For traits, select from trait attributes
rvn query "trait:due SELECT content, value AS due, parent.status"

# With expressions
rvn query "trait:due SELECT content, value, days_until(value) AS remaining"
```

**Pros**: Powerful, familiar SQL-like syntax, composable
**Cons**: Significant parser changes, complexity

---

#### Approach D: Output Templates

Simple templates for formatting each row:

```yaml
# raven.yaml
output_templates:
  task: "{{content | truncate:50}}  @due({{due}})  {{status | default:'-'}}"
  project: "{{name}}  [{{status}}]  lead: {{lead}}"
```

```bash
rvn query tasks --template task
rvn query "object:project" --template project
```

**Pros**: Flexible formatting, readable output
**Cons**: No automatic column alignment

---

#### Approach E: Views as First-Class Objects

Views stored as markdown files with embedded query + display config:

```markdown
---
type: view
query: "trait:due | trait:status"
---

# Task Board

| Content | Due | Status | Priority |
|---------|-----|--------|----------|
{{#each results}}
| {{content}} | {{due}} | {{status}} | {{priority}} |
{{/each}}
```

```bash
rvn view task-board  # Runs the view, renders the table
```

**Pros**: Views are documents, can include descriptions, versioned in vault
**Cons**: Mixing data and presentation, template complexity

---

#### Recommended Path

**Phase 1**: Column flags (`--columns`) for ad-hoc customization
- Simple to implement
- Covers most use cases
- No config changes needed

**Phase 2**: Named views in `raven.yaml` for saved configurations
- Build on saved queries
- Add column/sort definitions
- New `rvn view` command

**Phase 3 (if needed)**: SQL-like SELECT for power users
- Only if Phase 1-2 insufficient
- Significant parser work

**Key design decision**: Should views be separate from saved queries, or extend them?

Option A - Extend saved queries:
```yaml
queries:
  tasks:
    query: "trait:due"
    columns: [content, value, status]
    sort: value
```

Option B - Separate views concept:
```yaml
queries:
  tasks:
    query: "trait:due"

views:
  task-board:
    query: tasks  # Reference saved query
    columns: [...]
```

Option A is simpler; Option B allows multiple views of the same query.

**Status**: Not implemented. Design exploration only.

---

### ~~Interactive Type/Trait Creation~~ ✅ IMPLEMENTED
Schema modification via CLI:
```bash
rvn schema add type event --default-path events/
rvn schema add trait priority --type enum --values high,medium,low
rvn schema add field person email --type string --required
rvn schema validate
```

**Status**: ✅ Implemented. See `rvn schema add --help` for all options.

---

## Query Enhancements

### Full-Text Search
Add FTS5 support for searching note content:
```bash
rvn search "compound interest"
```

**Status**: Mentioned in Phase 2 of spec.

---

### Transitive Reference Queries

Support querying objects that are reachable through chains of references (graph traversal).

**Use cases:**
- "All objects reachable from project X"
- "Is there a path from person A to project B?"
- "All projects in my reference graph starting from today's daily note"

**Proposed syntax:**
```bash
# All objects reachable from a starting point
rvn query "object:* reachable:[[projects/website]]"

# Meetings that reference anything that references Freya
rvn query "object:meeting refs:{object:* refs:[[people/freya]]}"

# Alternative: explicit transitive closure operator
rvn query "object:meeting refs*:[[people/freya]]"  # refs* = transitive refs
```

**Implementation approach:**
```sql
-- Use SQLite recursive CTE
WITH RECURSIVE reachable(id) AS (
    SELECT target_id FROM refs WHERE source_id = ?
    UNION
    SELECT r.target_id FROM refs r
    JOIN reachable ON r.source_id = reachable.id
)
SELECT * FROM objects WHERE id IN (SELECT id FROM reachable);
```

**Considerations:**
- Cycle detection needed (references can form cycles)
- May need depth limit for performance
- Current `refs:` only does one hop; this would add multi-hop

**Status**: Not implemented. The current query language only supports direct references via `refs:`. Adding transitive closure would require:
1. New predicate syntax (`refs*:` or `reachable:`)
2. Recursive CTE support in executor
3. Cycle detection to prevent infinite loops

---

### ~~Date Range Queries~~ ✅ IMPLEMENTED
Support relative date queries in filters:
```bash
rvn query "trait:due value:this-week"
rvn query "trait:due value:past"         # overdue items
rvn query "trait:remind value:today"
```

**Status**: ✅ Implemented with support for: `today`, `yesterday`, `tomorrow`, `this-week`, `next-week`, `past`, `future`, and specific `YYYY-MM-DD` dates.

---

### ~~OR and NOT Filter Syntax~~ ✅ IMPLEMENTED
Support compound filter expressions for more flexible queries:

```yaml
# raven.yaml
queries:
  urgent:
    traits: [due]
    filters:
      due: "this-week|past"    # OR: this week or overdue
    
  open-tasks:
    traits: [status]
    filters:
      status: "!done"          # NOT: exclude done items
      
  active:
    traits: [status]
    filters:
      status: "!done|!cancelled"  # NOT done OR NOT cancelled
```

**Syntax:**
- `value`: exact match
- `a|b`: matches `a` OR `b`
- `!value`: NOT value (excludes)
- `!a|!b`: NOT a OR NOT b
- Works with date keywords: `"this-week|past"`, `"!past"`

**Status**: ✅ Implemented. Works in saved queries and CLI `--value` flag.

---

### ~~Date Hub~~ ✅ IMPLEMENTED
Show everything related to a specific date:
```bash
rvn date              # Today's date hub
rvn date yesterday
rvn date 2025-02-01
```

**Status**: ✅ Implemented. Shows daily note, tasks due, and all objects/traits with matching date fields.

---

### ~~Date Shorthand References~~ ✅ IMPLEMENTED
Allow `[[2025-02-01]]` syntax to reference daily notes:
```markdown
See [[2025-02-01]] for the meeting notes.
```

**Status**: ✅ Implemented. Resolves to configured daily directory (e.g., `daily/2025-02-01`).

---

## Import from Other Tools

### Logseq Import
Import a Logseq graph into Raven:
```bash
rvn import logseq ~/path/to/logseq-graph
```

**What it would handle:**
- `journals/YYYY_MM_DD.md` → `daily/YYYY-MM-DD.md`
- `pages/*.md` → organized by type or flat
- `TODO`/`DONE`/`LATER`/`NOW` → `@status(todo/done/later/in_progress)`
- `property:: value` → YAML frontmatter
- Fix `[[references]]` in frontmatter (not valid YAML)
- Preserve wiki-links and tags

**Status**: Successfully migrated manually. Could be polished into a built-in command.

---

### Obsidian Import
Import an Obsidian vault:
```bash
rvn import obsidian ~/path/to/obsidian-vault
```

**Considerations:**
- Obsidian uses standard YAML frontmatter (mostly compatible)
- Daily notes format may differ
- Dataview-style inline fields need conversion

**Status**: Not implemented. Obsidian vaults may work with minimal changes.

---

## Indexing Improvements

The goal is to make manual `rvn reindex` rare or unnecessary. Currently users must remember to reindex after external edits.

### ~~Auto-Reindex on Change (File Watching)~~ ✅ IMPLEMENTED
Watch vault for file changes and update index automatically:
```bash
rvn watch          # Standalone file watcher
rvn watch --debug  # With debug output
```

**Implementation:**
- Uses `fsnotify` for cross-platform file watching
- Debounces rapid changes (100ms delay)
- Incremental update: only reindex changed files
- Shared `internal/watcher/` package used by both `rvn watch` and `rvn lsp`

**Status**: ✅ Implemented.

---

### ~~Incremental Reindexing~~ ✅ IMPLEMENTED
Incremental reindexing is now the default behavior:
```bash
rvn reindex                # Incremental (default) - only changed/deleted files
rvn reindex --dry-run      # Preview what would be reindexed
rvn reindex --full         # Force full reindex (all files)
```

**Implementation:**
- `file_mtime` column tracks when each file was last modified at index time
- Compares current mtime to stored mtime, only re-indexes files with newer mtime
- **Detects deleted files**: Compares indexed paths against filesystem, removes orphaned entries
- Much faster for large vaults with few changes
- Schema version changes automatically trigger full reindex

**Status**: ✅ Implemented. Incremental is the default, `--full` forces complete rebuild.

---

### Auto-Reindex After Mutations
Commands that modify files automatically reindex the affected file:
```bash
rvn add "New note"              # → auto reindex daily note
rvn new person "Freya"          # → auto reindex new file
rvn set people/freya email=...  # → auto reindex people/freya.md
rvn edit path "old" "new"       # → auto reindex path
```

**Configuration** (`raven.yaml`):
```yaml
# Auto-reindex after CLI operations that modify files (default: true)
auto_reindex: true
```

**Status**: ✅ Implemented. All mutation commands (`add`, `new`, `set`, `edit`) use the centralized `auto_reindex` config. Enabled by default.

---

### ~~Stale Index Detection~~ ✅ IMPLEMENTED
Warn users when index may be out of date:
```bash
$ rvn query "trait:due"
⚠ Warning: 3 files may be stale: people/freya.md, projects/website.md, daily/2026-01-02.md
  Run 'rvn reindex' or use '--refresh' to update.

• @due(2025-02-01) Send proposal
  ...
```

**Implementation:**
- On query, compares indexed `file_mtime` against current filesystem mtimes
- Shows warning if stale files detected (lists up to 3, then count)
- `--refresh` flag auto-reindexes stale files before query

```bash
rvn query "object:project" --refresh  # Refresh stale files first
```

**Status**: ✅ Implemented.

---

### Background Index Service
Long-running process that keeps index always fresh:
```bash
rvn daemon start    # Start background indexer
rvn daemon stop     # Stop it
rvn daemon status   # Check if running
```

**Why useful:**
- Index always up-to-date without manual intervention
- Could integrate with system services (launchd, systemd)
- MCP server could start this automatically

**Status**: Not implemented. `rvn watch` would be a simpler first step.

---

## Concurrency & Multi-Agent Support

As agents use Raven more heavily, concurrent access becomes a concern. Currently designed for single-user, sequential access.

### Problem Scenarios

1. **File conflicts**: Two agents try to edit the same file simultaneously
2. **Index staleness**: Agent A edits file, Agent B queries stale index
3. **Lost updates**: Both read → both modify → last write wins, first changes lost
4. **Race conditions**: Reading a file while another process is writing

### Potential Approaches

#### Option A: Optimistic Locking (Simplest)
Check file hasn't changed before writing:
```go
// Before writing:
// 1. Store original mtime/hash when reading
// 2. Before write, check current mtime/hash matches
// 3. If changed, return error: "file modified externally, please retry"
```

**Pros**: Simple, no infrastructure needed
**Cons**: Requires caller to handle retry logic

---

#### Option B: File Locking
Lock files during read-modify-write operations:
```go
// flock() or similar
lock := acquireLock(filePath)
defer lock.Release()
// ... read, modify, write ...
```

**Pros**: Prevents concurrent writes
**Cons**: Cross-platform complexity, potential deadlocks, doesn't help with index staleness

---

#### Option C: Central Mutation Daemon
Route ALL mutations through a single long-running process:
```
┌─────────┐     ┌─────────┐     ┌──────────────┐
│ Agent 1 │────▶│         │     │              │
└─────────┘     │  Raven  │────▶│  Vault Files │
┌─────────┐     │  Daemon │     │  + Index     │
│ Agent 2 │────▶│         │     │              │
└─────────┘     └─────────┘     └──────────────┘
```

**How it works:**
- Single `rvn daemon` process handles all mutations
- Agents connect via socket/IPC (or MCP)
- Daemon serializes writes, keeps index always fresh
- Queries can still be concurrent (SQLite handles this)

**Pros**: 
- Eliminates all race conditions
- Index always up-to-date
- Single source of truth for file operations

**Cons**:
- More infrastructure
- Daemon must be running
- Need graceful fallback when daemon not available

---

#### Option D: Database as Write-Ahead Log
Log intended mutations to SQLite, apply to files asynchronously:
```
Agent writes → mutation queue (SQLite) → background worker → file system
```

**Pros**: Very robust, supports offline
**Cons**: Complex, eventual consistency for file reads

---

### Recommended Path

**Phase 1 (Low effort):**
- Add optimistic locking to mutation commands
- Return clear error when file was modified externally
- Agents retry with fresh read

**Phase 2 (Medium effort):**
- Implement `rvn daemon` for file watching + indexing
- Mutations still go direct to files (optimistic locking)
- Daemon keeps index fresh automatically

**Phase 3 (If needed):**
- Route mutations through daemon
- Full serialization of writes
- Only pursue if Phase 1-2 insufficient

**Status**: Not implemented. Current design assumes single-writer.

---

## Web UI

### Local Web Server
Serve a read-only web interface for browsing notes:
```bash
rvn web    # (proposed command name)
```

**Note**: `rvn serve` is now used for the MCP server (agent integration). A future web UI would use a different command like `rvn web` or `rvn ui`.

**Status**: Mentioned in Phase 5 of spec.

---

## Editor Integration

### VS Code Extension
Syntax highlighting and autocomplete for Raven syntax in VS Code.

---

### Obsidian Plugin
Bridge between Raven and Obsidian for users who want both.

---

## Data Export

### Export to JSON
Dump entire index to JSON for external tools:
```bash
rvn export --format json > vault.json
```

---

### Calendar Export (ICS)
Export meetings to ICS format for calendar integration:
```bash
rvn export-calendar --type meeting > meetings.ics
```

---

## External Integrations

These integrations connect Raven to external services for data capture and publishing.

---

### Slack Integration

Capture Slack conversations and threads into Raven notes.

**Use cases:**
- Summarize a Slack thread → append to daily note
- Save important discussions as standalone pages
- Extract action items from conversations → `@due` traits
- Link Slack users to `[[people/...]]` references

**Agent workflow example:**
```
User: "Summarize that #project-bifrost thread and save it"
Agent:
  1. Fetches thread via Slack API
  2. Identifies participants → links to existing people or creates new
  3. Summarizes discussion
  4. Extracts action items with @due traits
  5. Appends to daily note or creates dedicated page
```

**Example output:**
```markdown
## Slack: #project-bifrost (2026-01-02)
::slack-summary(id=bifrost-planning, channel="#project-bifrost", thread_ts="1704200000.000000")

Discussion about Q1 timeline with [[people/freya]] and [[people/thor]].

**Key decisions:**
- Pushing launch to March 15
- Need additional security review

**Action items:**
- @due(2026-01-15) [[people/freya]] to finalize API spec
- @due(2026-01-10) [[people/thor]] to schedule security review

[View thread](https://workspace.slack.com/archives/C123/p1704200000000000)
```

**Implementation considerations:**
- OAuth integration for Slack API access
- MCP tool: `raven_import_slack` with thread URL or channel/timestamp
- Schema trait for Slack references: `slack_thread: { type: string }`
- User mapping: match Slack display names to existing `person` objects
- Rate limiting and pagination for long threads

**Status**: Not implemented.

---

### Calendar Two-Way Sync

Bidirectional sync between Raven and calendar services (Google Calendar, Outlook, Apple Calendar).

**Inbound (Calendar → Raven):**
- Import calendar events as `meeting` objects
- Auto-create attendee references to `[[people/...]]`
- Pre-meeting: gather context about attendees and related projects
- Trigger: "You have a meeting with [[people/freya]] in 30 minutes. Here's context..."

**Outbound (Raven → Calendar):**
- Push `@due` items as calendar reminders
- Export `meeting` objects to calendar
- Sync `@remind` traits as calendar notifications

**Example workflow:**
```
User: "Sync my calendar for this week"
Agent:
  1. Fetches calendar events via API
  2. For each event:
     - Creates/updates meeting object with attendees, time, location
     - Links attendees to existing people or prompts to create
  3. Reports: "Synced 8 meetings. Created 2 new people: [[people/alex]], [[people/sam]]"
```

**Configuration** (`raven.yaml`):
```yaml
integrations:
  calendar:
    provider: google  # or outlook, apple
    sync_direction: both  # inbound, outbound, both
    create_people: prompt  # auto, prompt, skip
    reminder_trait: remind  # which trait to sync as reminders
```

**Implementation considerations:**
- OAuth flow for calendar APIs
- Conflict resolution for two-way sync
- Event ID storage for update tracking (new field on meeting type)
- Recurring event handling
- Timezone normalization

**Status**: Not implemented. One-way ICS export mentioned in Data Export section.

---

### Meeting Transcript Processing

Process meeting transcripts (Zoom, Otter.ai, Google Meet, etc.) into structured Raven notes.

**What it does:**
1. Parse transcript text or API response
2. Identify speakers → link to `[[people/...]]`
3. Extract key discussion points
4. Identify action items → create `@due` traits
5. Generate structured meeting note

**Agent workflow:**
```
User: "Process this Zoom transcript" [pastes transcript or provides file]
Agent:
  1. Parses transcript, identifies speakers
  2. Maps speakers: "I found 3 speakers. 'Freya Chen' matches [[people/freya]]. 
     Who are 'Alex' and 'Jordan'?"
  3. User: "Alex is new, Jordan is [[people/jordan-smith]]"
  4. Agent creates people/alex.md, processes transcript
  5. Creates meeting note with summary, attendees, and action items
```

**Example output:**
```markdown
---
type: meeting
time: 2026-01-02T14:00
attendees:
  - [[people/freya]]
  - [[people/alex]]
  - [[people/jordan-smith]]
transcript_source: zoom
---

# Project Kickoff - Bifrost v2

## Summary

Discussed timeline for Bifrost v2 launch. Team aligned on March 15 target date.
Main concerns around API stability and security review.

## Key Points

- [[people/freya]]: Proposed phased rollout starting with internal users
- [[people/alex]]: Raised concerns about load testing capacity
- [[people/jordan-smith]]: Volunteered to lead security review

## Action Items

- @due(2026-01-10) @assignee([[people/freya]]) Draft phased rollout plan
- @due(2026-01-08) @assignee([[people/alex]]) Set up load testing environment
- @due(2026-01-15) @assignee([[people/jordan-smith]]) Complete security review

## Raw Transcript

<details>
<summary>Full transcript (click to expand)</summary>

[00:00] Freya: Let's kick off the Bifrost v2 planning...
...
</details>
```

**Implementation considerations:**
- Support multiple transcript formats (Zoom VTT, Otter.ai JSON, plain text)
- Speaker diarization and name normalization
- LLM-powered summarization and action item extraction
- Option to store or discard raw transcript
- MCP tool: `raven_process_transcript`

**Status**: Not implemented.

---

## ~~Agentic Workflows~~ ✅ IMPLEMENTED

Workflows are now a first-class feature in Raven. They allow users to define reusable prompt templates
that gather context from the vault and render structured prompts for AI agents.

**Status**: ✅ Implemented. See [WORKFLOWS_SPEC.md](WORKFLOWS_SPEC.md) for the full specification and
the [README](../README.md#workflows) for usage examples.

**Key features:**
- Define workflows in `raven.yaml` (inline or file-based)
- Input parameters with types, defaults, and validation
- Context queries: `read`, `query`, `backlinks`, `search`
- Simple `{{var}}` template substitution
- CLI: `rvn workflow list`, `rvn workflow show`, `rvn workflow render`
- MCP tools: `raven_workflow_list`, `raven_workflow_show`, `raven_workflow_render`

---

### Weekly Review Generation

Automatically generate a weekly review note summarizing activity.

**What the agent does:**
```
User: "Generate my weekly review"
Agent:
  1. Query items with @highlight from past week
  2. Query completed tasks (items where @status changed to done)
  3. Query new pages created this week
  4. Query upcoming items for next week
  5. Compile into structured weekly review
```

**Example output:**
```markdown
---
type: weekly-review
week: 2026-W01
---

# Weekly Review: Dec 30, 2025 - Jan 5, 2026

## Highlights
- @highlight "The Norns weave fate at Urd's Well" — [[books/poetic-edda]]
- @highlight Bifrost API design finalized — [[projects/bifrost]]

## Completed (7 items)
- ✓ Send [[clients/midgard]] proposal
- ✓ Security review for [[projects/bifrost]]
- ✓ 1:1 with [[people/freya]]
...

## Created This Week
- [[people/alex]] (person)
- [[projects/asgard-security-audit]] (project)

## Upcoming Next Week
- @due(2026-01-08) Load testing setup
- @due(2026-01-10) Phased rollout plan
- @due(2026-01-10) 1:1 with [[people/thor]]

## Focus Areas
Based on your activity, you spent most time on:
1. [[projects/bifrost]] (mentioned 12 times)
2. [[clients/midgard]] (mentioned 8 times)
```

**Implementation notes:**
- Could be a saved query + template, or a dedicated MCP tool
- Temporal filters (`--created-since`, `--modified-since`) would help
- Consider adding `weekly-review` as a built-in type

**Status**: Possible with current tools, but requires agent orchestration. Could be optimized with dedicated command.

---

### 1:1 Meeting Prep

Gather context before a 1:1 meeting with someone.

**What the agent does:**
```
User: "Prep for my 1:1 with [[people/freya]]"
Agent:
  1. Get all backlinks to people/freya (recent mentions)
  2. Query open tasks assigned to or mentioning Freya
  3. Find last 1:1 meeting notes with Freya
  4. Check any pending action items from previous 1:1s
  5. Compile into prep document
```

**Example output:**
```markdown
# 1:1 Prep: [[people/freya]] — 2026-01-02

## Recent Activity
- Mentioned in [[daily/2026-01-02]]: "Discussed timeline with Freya"
- Mentioned in [[projects/bifrost]]: Lead engineer
- Last 1:1: [[daily/2025-12-19#freya-1-1]] (2 weeks ago)

## Open Items Involving Freya
- @due(2026-01-10) Freya to finalize API spec — [[projects/bifrost]]
- @due(2026-01-15) Review Freya's promotion case — [[daily/2025-12-19]]

## Pending from Last 1:1
- [ ] Discuss tech lead role interest (from [[daily/2025-12-19]])
- [ ] Follow up on conference budget

## Suggested Topics
- Bifrost timeline pressure
- Career growth / tech lead path
- Holiday break debrief
```

**Status**: Possible with current tools (`raven_backlinks`, `raven_search`, `raven_query`). Could be a dedicated MCP tool for convenience.

---

### Daily Digest

Morning briefing of what's relevant today.

**What the agent does:**
```
User: "What's my day look like?" (or triggered automatically)
Agent:
  1. Query @due(today) items
  2. Query @remind(today) items  
  3. Check calendar events (if integrated)
  4. Find items from yesterday that weren't completed
  5. Surface any highlights from yesterday
```

**Example output:**
```
Good morning! Here's your day:

📅 **Meetings** (3)
- 09:00 Weekly Standup with [[people/freya]], [[people/thor]]
- 14:00 1:1 with [[people/alex]]
- 16:00 Client call with [[clients/midgard]]

✅ **Due Today** (4)
- Send revised estimate to [[clients/midgard]]
- Review PR #1234
- Finish Chapter 3 of [[books/poetic-edda]]
- Submit expense report

⏰ **Reminders**
- 10:00 Check if designs are ready
- 14:00 Prep for Alex 1:1

⚠️ **Carried Over** (2 items from yesterday)
- Security review feedback (was due yesterday)
- Update project timeline

💡 **Yesterday's Highlight**
"Buffer time is the key to good estimates" — [[daily/2026-01-01]]
```

**Status**: Possible with current tools. Natural extension of `rvn date`.

---

### Research Assistant

Research a topic and add findings to a project or note.

**What the agent does:**
```
User: "Research Kubernetes security best practices and add to [[projects/infrastructure]]"
Agent:
  1. Search web for topic
  2. Synthesize findings
  3. Append to specified page with sources
  4. Add @highlight for key insights
```

**Example addition to projects/infrastructure.md:**
```markdown
## Kubernetes Security Research (added 2026-01-02)

Key findings from security best practices research:

- @highlight Use network policies to restrict pod-to-pod communication
- Enable Pod Security Standards (replacing deprecated PodSecurityPolicy)
- Implement RBAC with least-privilege principle
- Use secrets management (Vault, Sealed Secrets) instead of plain K8s secrets
- Regular vulnerability scanning with Trivy or similar

**Sources:**
- [CNCF Security Whitepaper](https://...)
- [NSA Kubernetes Hardening Guide](https://...)
- [Kubernetes Security Best Practices - OWASP](https://...)
```

**Status**: Requires web search capability (not in current Raven tools). Agent could use external search + Raven write tools.

---

### Capture Processing

Process inbox items captured throughout the day.

**What the agent does:**
```
User: "Process my captures from today"
Agent:
  1. Read today's daily note
  2. For each captured item:
     - Identify if it's a task (has due date or action verb)
     - Identify people mentioned → suggest [[people/...]] links
     - Identify projects mentioned → suggest [[projects/...]] links
     - Suggest appropriate traits (@due, @priority)
  3. Present suggestions for user approval
  4. Apply approved changes
```

**Example interaction:**
```
Agent: "I found 5 captures from today. Here's what I suggest:

1. 'Call Odin about the Bifrost ceremony'
   → @due(tomorrow) Call [[people/odin]] about [[projects/bifrost]] ceremony
   
2. 'Remember to review the Midgard proposal'  
   → @due(this-week) @priority(high) Review [[clients/midgard]] proposal
   
3. 'Great insight about the Norns'
   → @highlight Great insight about the Norns (link to [[books/poetic-edda]]?)

Apply these suggestions? [Y/n/edit]"
```

**Status**: Possible with current tools. Agent interprets captures and uses `raven_edit` to enhance them.

---

## ~~Template System~~ ✅ IMPLEMENTED

Templates provide default content when creating new typed notes.

---

### ~~Type Templates~~ ✅ IMPLEMENTED

Define templates for new notes of each type. When `rvn new meeting "Team Sync"` runs,
the file is created with frontmatter + template content.

**Template location: File-based (recommended)**

Templates live in a `templates/` directory, referenced from the schema:

```yaml
# schema.yaml
types:
  meeting:
    default_path: meetings/
    template: templates/meeting.md
    fields:
      time: { type: datetime }
      attendees: { type: ref[], target: person }
```

With `templates/meeting.md`:
```markdown
# {{title}}

**Time:** {{field.time}}

## Attendees

## Agenda

## Notes

## Action Items
```

**Template location: Inline (for short templates)**

Small templates can be inline in the schema:

```yaml
types:
  quick-note:
    template: |
      # {{title}}
      
      ## Notes
```

**Status**: ✅ Implemented. Add `template` field to any type definition in schema.yaml.

---

### ~~Template Variables~~ ✅ IMPLEMENTED

Simple `{{var}}` substitution — no conditionals or loops. Keep it minimal.

| Variable | Description | Example |
|----------|-------------|---------|
| `{{title}}` | Title passed to `rvn new` | "Team Sync" |
| `{{slug}}` | Slugified title | "team-sync" |
| `{{type}}` | The type name | "meeting" |
| `{{date}}` | Today's date | "2026-01-02" |
| `{{datetime}}` | Current datetime | "2026-01-02T14:30" |
| `{{year}}` | Current year | "2026" |
| `{{month}}` | Current month (2 digit) | "01" |
| `{{day}}` | Current day (2 digit) | "02" |
| `{{weekday}}` | Day name | "Monday" |
| `{{field.X}}` | Value of field X (from `--field`) | `{{field.time}}` |

**Escaping:** Use `\{{literal}}` if you need literal `{{` in output.

**Example template:**
```markdown
# {{title}}

Created: {{date}}

## Notes
```

**No conditionals for v1.** If users need conditional content, they can use agent workflows 
to post-process the created file.

**Status**: ✅ Implemented.

---

### ~~Daily Note Templates~~ ✅ IMPLEMENTED

The `date` type is built-in and special. To customize daily note structure,
add `daily_template` to `raven.yaml`:

```yaml
# raven.yaml
daily_directory: daily
daily_template: templates/daily.md
```

With `templates/daily.md`:
```markdown
# {{date}}

## Morning

## Afternoon  

## Evening

## Reflections
```

Or inline:
```yaml
daily_template: |
  # {{date}}
  
  ## Tasks
  
  ## Notes
```

**Variables for daily templates:**
- `{{date}}` — the date (YYYY-MM-DD)
- `{{year}}`, `{{month}}`, `{{day}}` — components
- `{{weekday}}` — day name ("Monday", "Tuesday", etc.)

**Status**: ✅ Implemented. Add `daily_template` to raven.yaml.

---

### Implementation Notes

1. **Template resolution order:**
   - Check for `template` field on type definition
   - If path, read from `templates/` directory
   - If inline string, use directly

2. **Template application:**
   - `rvn new` creates frontmatter from type fields + passed values
   - Template content is appended after frontmatter
   - Variables are substituted

3. **Required fields interaction:**
   - User is prompted for required fields (existing behavior)
   - Those values become available as `{{field.X}}` in template

4. **MCP support:**
   - `raven_new` applies templates the same way
   - Template content included in response

5. **Error handling:**
   - Missing template file → warning, create without template
   - Unknown variable → leave as literal `{{unknown}}`

**Status**: ✅ Implemented in `internal/template/` package.

---

## Test Coverage Improvements

Areas identified for additional test coverage. These require more complex test fixtures or integration testing.

### Query Executor (`internal/query/executor.go`)

The following functions have limited or no coverage:
- `buildAncestorPredicateSQL` (0%) - requires multi-level hierarchy fixtures
- `buildChildPredicateSQL` (0%) - requires parent-child relationship fixtures
- `buildSourcePredicateSQL` (0%) - requires frontmatter vs inline trait fixtures
- `buildWithinPredicateSQL` (0%) - requires nested object fixtures
- `buildOrPredicateSQL` (0%) - requires complex boolean query fixtures
- `buildGroupPredicateSQL` (0%) - requires complex grouped query fixtures
- `Execute` (0%) - main execute method, tested indirectly through CLI

**Status**: These tests require SQLite database fixtures with realistic hierarchical data. Could be added as integration tests.

### Check Validator (`internal/check/validator.go`)

- `MissingRefs` (0%) - summary method
- `UndefinedTraits` (0%) - summary method
- `trackMissingRef` (42%) - reference tracking
- `trackUndefinedTrait` (0%) - trait tracking
- `containsHash` (0%) - utility function

**Status**: These could be unit tested with mock data or tested through integration tests.

### CLI Commands (`internal/cli/`)

The CLI package has no direct tests. Commands are tested indirectly through manual testing and the MCP tools tests.

**Options for improvement:**
1. Add integration tests that invoke CLI commands
2. Refactor command logic into testable functions
3. Use testable patterns (e.g., dependency injection for IO)

**Status**: Lower priority - CLI behavior is tested indirectly via MCP and manual testing.

### Editor/Watcher (`internal/vault/editor.go`, `internal/watcher/`)

These packages involve external processes (opening editors, file system watching) which are difficult to unit test.

**Status**: Deferred - may require mocking OS interactions.

---

## Theoretical Design Considerations

Issues identified through theoretical analysis of the system design.

### Type System

**No subtyping/inheritance**: Types are flat with no hierarchy. Cannot query "all content types" where `book`, `article`, `paper` are subtypes of `content`.

- **Status**: Design choice for simplicity. Workaround: enumerate types explicitly or use traits.
- **Future option**: Add optional `extends:` field to type definitions.

#### Proposed: Type Inheritance

Allow types to inherit fields and traits from a parent type:

```yaml
# schema.yaml
types:
  content:
    fields:
      title: { type: string, required: true }
      created: { type: date }
    traits: [status, priority]

  book:
    extends: content    # Inherits title, created, status, priority
    fields:
      author: { type: ref, target: person }
      rating: { type: number, min: 1, max: 5 }

  article:
    extends: content
    fields:
      url: { type: string }
      source: { type: string }
```

**Query implications:**
- `object:book` matches only books
- `object:content` could match all subtypes (books, articles, etc.)
- Alternatively, add `object:content+` for "content and subtypes"

**Implementation considerations:**
- Single inheritance only (no diamond problem)
- Field override semantics: child can add fields, not redefine parent fields
- Trait inheritance: child inherits parent traits, can add more
- Built-in types (`page`, `section`, `date`) cannot be extended

**Status**: Not implemented. Would require schema loader changes and query executor updates.

---

**No cross-type unions in queries**: `object:(book | article)` is not valid—each query returns exactly one type.

- **Status**: Design choice for type safety. Workaround: use OR at predicate level or run separate queries.

#### Proposed: Wildcard Type Queries

Allow querying across all types when needed:

```bash
# All objects with a due trait
object:* has:due

# All objects referencing a person
object:* refs:[[people/freya]]
```

**Implementation**: `object:*` would omit the `type = ?` filter in SQL, returning all object types. Results would be grouped by type in output.

**Status**: Not implemented. Simple extension that doesn't break existing semantics.

---

### Trait Patterns (Composite Traits)

Currently, "tasks" are emergent—anything with `@due` or `@status` is effectively a task. This works but has limitations:

1. No way to query "all task-like things" without knowing which traits define a task
2. Users must remember which trait combinations constitute a "task"
3. Schema doesn't document these emergent patterns

#### Proposed: Trait Patterns

Define named patterns that represent common trait combinations:

```yaml
# schema.yaml
trait_patterns:
  task:
    description: "Items with due dates and/or status tracking"
    requires_any: [due, status]   # At least one of these
    # or: requires_all: [due, status]  # Must have both

  actionable:
    description: "Things that need action"
    requires_all: [due]
    requires_any: [status, priority]

  reviewable:
    description: "Items awaiting review"
    requires_all: [status]
    where:
      status: [pending, in_review]   # Additional value constraints
```

**Query syntax:**
```bash
# All items matching the "task" pattern
pattern:task

# Task pattern on projects only
pattern:task on:{object:project}

# Combined with value filters
pattern:task value:past   # Overdue tasks
```

**Benefits:**
- Documents emergent patterns in schema
- Single query for common concepts
- Schema introspection shows what patterns exist
- Agents can discover patterns via `rvn schema patterns`

**Implementation considerations:**
- Patterns are query macros, not new data structures
- `pattern:task` expands to `(trait:due | trait:status)` at parse time
- OR: patterns query the `traits` table with `trait_type IN (...)`
- Value constraints (`where:`) add additional filters

**Alternative: Saved query approach**

Instead of schema-level patterns, use saved queries:

```yaml
# raven.yaml
queries:
  tasks:
    description: "All task-like items"
    query: "trait:due | trait:status"
```

This already works today but:
- Less discoverable (not in schema introspection)
- Doesn't validate trait existence
- Query syntax more verbose than `pattern:task`

**Status**: Not implemented. The saved query approach covers most use cases. Pattern syntax would be a convenience layer.

### Query Semantics

**Negation-as-failure vs. explicit negation**: `!has:due` matches objects with no indexed `@due` trait. If the index is stale, this may include false positives.

- **Status**: Documented in consistency model. Users should run `rvn reindex` when needed.

**No aggregations**: Cannot express "projects with more than 5 tasks" or "count by status".

- **Status**: Deferred. Agents can compute aggregations from query results.

### Object Identity

**Path-based identity**: Object IDs are derived from file paths (`people/freya`). Renaming a file changes the object's identity.

- **Status**: Design choice for simplicity. The `rvn move` command updates references, but external renames break links.
- **Alternative considered**: UUID-based identity. Rejected due to migration complexity and reference syntax changes.

### Reference Graph

**No cycle detection**: References can form cycles (`A → B → A`). Queries don't infinite loop (bounded by SQL), but no formal DAG constraint.

- **Status**: Acceptable for personal knowledge management. Cycles are rare and harmless.

**Dangling references allowed**: `[[nonexistent]]` is syntactically valid but warns during `rvn check`.

- **Status**: Open-world assumption for references (things might be created later). Closed-world for traits.

### Concurrency

**No isolation guarantees**: Multiple writers can cause lost updates. See "Concurrency & Multi-Agent Support" section above.

- **Status**: Optimistic locking proposed but not yet implemented.

---

## Adding New Ideas

When you think of a potential enhancement:
1. Add it to this file with a brief description
2. Note any context for why it was postponed
3. Reference the spec section if applicable
