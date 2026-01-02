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

### Auto-Reindex on Change (File Watching)
Watch vault for file changes and update index automatically:
```bash
rvn watch
```

**Implementation notes:**
- Use `fsnotify` for cross-platform file watching
- Debounce rapid changes (e.g., 100ms delay)
- Incremental update: only reindex changed files
- Could run as background daemon or integrate with editors

**Status**: Mentioned in Phase 3 of spec.

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
Commands that modify files should automatically reindex the affected file:
```bash
rvn add "New note"              # → auto reindex daily note
rvn new person "Alice"          # → auto reindex new file
rvn set people/alice email=...  # → auto reindex people/alice.md
rvn edit path "old" "new"       # → auto reindex path
```

**Status**: Partially implemented. Some commands (`rvn add`, `rvn edit`) do trigger reindex. Should audit all mutation commands.

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

## Refactoring Tools

### Move/Rename with Reference Updates
Move files and automatically update all references:
```bash
rvn mv people/alice.md people/alice-chen.md
```

**Status**: Mentioned in Phase 4 of spec.

---

### Promote Embedded to File
Convert an embedded object to a standalone file:
```bash
rvn promote daily/2025-02-01#standup --to meetings/standup-2025-02-01.md
```

**Status**: Mentioned in Phase 4 of spec.

---

## Task Workflow Enhancements

✅ **IMPLEMENTED**: Atomic traits model. Tasks are now emergent from atomic traits like `@due`, `@priority`, `@status`. Saved queries in `raven.yaml` define what "tasks" means.

```markdown
- @due(2025-02-01) @priority(high) @status(todo) Send proposal
```

### ~~CLI Task Mutation Commands~~ ✅ PARTIALLY IMPLEMENTED
Commands to modify trait values without manually editing files:
```bash
rvn set people/alice email=alice@example.com   # Updates frontmatter field
rvn set projects/website status=active         # Updates frontmatter field
```

**Status**: ✅ `rvn set` is implemented for frontmatter fields. Inline trait mutation (changing `@status(todo)` to `@status(done)` within content) is not yet implemented.

---

### Stable Trait IDs for Cross-Session References
Allow agents/users to assign stable IDs to traits for persistent references:
```markdown
- @due(2025-02-01) @id(review-contract) Review the contract
```

Then reference or update by ID:
```bash
rvn trait get review-contract
rvn trait update review-contract --set status=done
```

**Why postponed**: Most agent workflows are synchronous within a single session. The agent queries, gets file:line coordinates, and mutates immediately. Cross-session persistence can use content-based re-querying. Add this if external system integration or user-named traits become a real need.

**Alternative considered**: Content-hashing to auto-generate stable IDs. Rejected as "magic" that's complex to implement and debug.

---

### Trait Instance IDs (Referencing)
Allow referencing specific trait instances in links:
```markdown
See [[daily/2025-02-01#due:1]] for the original due date.
```

**Why postponed**: Adds complexity. Most use cases don't need to reference individual traits.

---

### Checkbox Syntax Sync (Editor Integration)
Sync markdown checkboxes with status trait:
```markdown
- [ ] @due(2025-02-01) Send proposal  → infers @status(todo)
- [x] @due(2025-02-01) Send proposal  → infers @status(done)
```

**Why postponed**: Creates two sources of truth. Better solved via editor plugins that understand Raven syntax.

---

## Template System

### Type Templates
Define templates for new notes of each type:
```yaml
types:
  meeting:
    template: |
      ## Attendees
      
      ## Agenda
      
      ## Notes
      
      ## Action Items
```

---

## Agent Integration

### Raw File Commands
Low-level file access for edge cases the structured commands can't handle:
```bash
rvn file read daily/2025-02-01.md --json    # Get raw markdown content
rvn file write daily/2025-02-01.md --content "..."  # Overwrite file
rvn file append daily/2025-02-01.md --content "..."  # Append to file
rvn file append daily/2025-02-01.md --section "## Notes" --content "..."  # Append under heading
```

**Why postponed**: Structured commands (`rvn object get/update`, `rvn trait create/update/delete`, `rvn add`) should cover most use cases. Add raw file access only if we hit cases the abstraction can't handle.

**Existing coverage**:
- `rvn add` — append content to files
- `rvn object get --json` — includes content in response
- `rvn object update` — modify frontmatter fields
- `rvn trait create/update/delete` — manage inline traits

---

## Agent Enhancements

### Temporal Query Filters
Filter queries by creation/modification timestamps:
```bash
rvn trait due --created-after 2025-01-20 --json
rvn type person --modified-today --json
rvn query tasks --created-since "2 days ago" --json
```

**Why postponed**: Audit log infrastructure exists, but temporal filters aren't wired into query commands. Add when there's a concrete use case.

---

### Rich Context Responses (`--depth`, `--slim`)
Control how much context is included in JSON responses:
```bash
rvn trait due --json --slim          # Minimal: just IDs and values
rvn trait due --json --depth 2       # Include parent + parent's parent + resolved refs
```

**Why postponed**: Current responses are sufficient for MVP. Add if agents need more or less context.

---

### Dry Run Mode
Preview changes without committing:
```bash
rvn new person "Bob" --dry-run --json
rvn set people/alice email=new@email.com --dry-run --json
```

**Why postponed**: Not critical for MVP. Useful for cautious agents.

---

### `rvn log` Command
Query the audit log directly:
```bash
rvn log --since yesterday --json
rvn log --id people/alice --json
rvn log --op create --entity trait --json
```

**Why postponed**: Audit log exists but no CLI to query it. Add when temporal introspection is needed.

---

### `rvn validate` Command
Pre-flight validation for inputs:
```bash
rvn validate --type object --input '{"type": "person", "fields": {"name": "Bob"}}' --json
```

**Why postponed**: Error messages from actual commands are clear enough. Add if agents need to check before attempting.

---

### Batch Operations
Execute multiple operations atomically:
```bash
rvn batch --input operations.json --json
```

With support for `atomic`, `stop_on_error`, and `dry_run` options.

**Why postponed**: Single operations cover most use cases. Add when agents need multi-step transactions.

---

## Adding New Ideas

When you think of a potential enhancement:
1. Add it to this file with a brief description
2. Note any context for why it was postponed
3. Reference the spec section if applicable
