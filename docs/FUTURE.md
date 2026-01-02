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

## File Watching

### Auto-Reindex on Change
Watch vault for file changes and update index automatically:
```bash
rvn watch
```

**Status**: Mentioned in Phase 3 of spec.

---

## Web UI

### Local Web Server
Serve a read-only web interface for browsing notes:
```bash
rvn serve
```

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

## Adding New Ideas

When you think of a potential enhancement:
1. Add it to this file with a brief description
2. Note any context for why it was postponed
3. Reference the spec section if applicable
