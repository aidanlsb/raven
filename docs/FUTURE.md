# Future Enhancements

This document tracks potential future enhancements and ideas. These are not currently planned for implementation but are worth considering for later.

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

### ~~Date Range Queries~~ ✅ IMPLEMENTED
Support relative date queries in filters:
```bash
rvn tasks --due this-week
rvn tasks --due past      # overdue tasks
rvn trait remind --at today
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

### Incremental Reindexing
Only reindex files that have changed since last index:
```bash
rvn reindex           # Smart: only changed files
rvn reindex --full    # Force full reindex
```

**Implementation:**
- Store file mtime in database during indexing
- On reindex, compare current mtime to stored mtime
- Only parse and re-index files with newer mtime
- Much faster for large vaults with few changes

**Status**: Not implemented. Currently `rvn reindex` always does a full rebuild.

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

### Stale Index Detection
Warn users when index may be out of date:
```bash
$ rvn trait due
⚠ Index may be stale (5 files modified since last reindex)
Run 'rvn reindex' to update.

• @due(2025-02-01) Send proposal
  ...
```

**Implementation:**
- On query, quick-scan vault for files with mtime > last index time
- Show warning if stale files detected
- Optional: auto-reindex stale files before query

**Status**: Not implemented.

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

## Agentic Workflows

These are compound workflows that agents can perform by combining multiple Raven operations.
They showcase what's possible with the existing MCP tools and suggest potential optimizations.

---

### Workflow Recipes (Pattern)

Users may want to define reusable "recipes" for common agent workflows. This is fully emergent — 
Raven provides the primitives, users compose them however they want.

**Option A: User-defined `workflow` type**

Users who want structured, queryable workflows can add to their schema:

```yaml
# schema.yaml
types:
  workflow:
    default_path: workflows/
    fields:
      name: { type: string, required: true }
      description: { type: string }
      trigger: { type: enum, values: [manual, daily, weekly] }
```

Then create workflow files:

```markdown
---
type: workflow
name: Weekly Review
description: Generate a summary of the week's activity
trigger: manual
---

# Weekly Review

## Steps
1. Query `@highlight` items from past week
2. Query completed tasks (@status changed to done)
3. List new pages created this week
4. List items due next week

## Output
Create a `weekly-review` note in `reviews/` with sections for each category.
```

Agents discover via `raven_type(type_name="workflow")`.

**Option B: Informal pattern with tags**

Users who want something lighter can just use a `#workflow` tag:

```markdown
# Weekly Review #workflow

Steps to generate weekly review...
```

Agents discover via `raven_tag(tag="workflow")`.

**Option C: Just a folder convention**

Or simply put workflow docs in `workflows/` and agents read from there.

**Key principle:** Raven doesn't need built-in workflow support. The schema system is flexible 
enough that users can define whatever structure makes sense for their use case. Agents can read 
workflow documentation via existing tools (`raven_read`, `raven_type`, `raven_tag`, `raven_search`) 
and execute accordingly.

**Status**: No implementation needed — this is an emergent pattern using existing features.

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
- @highlight "Small habits compound over time" — [[books/atomic-habits]]
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

**Status**: Possible with current tools (`raven_backlinks`, `raven_search`, `raven_trait`). Could be a dedicated MCP tool for convenience.

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
- Finish Chapter 3 of [[books/atomic-habits]]
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
   
3. 'Great insight about compound habits'
   → @highlight Great insight about compound habits (link to [[books/atomic-habits]]?)

Apply these suggestions? [Y/n/edit]"
```

**Status**: Possible with current tools. Agent interprets captures and uses `raven_edit` to enhance them.

---

## Template System

Templates provide default content when creating new typed notes.

---

### Type Templates

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

---

### Template Variables

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

---

### Daily Note Templates

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

**Status**: Not implemented. Schema supports the field but template application not wired up.

---

## Adding New Ideas

When you think of a potential enhancement:
1. Add it to this file with a brief description
2. Note any context for why it was postponed
3. Reference the spec section if applicable
