# Future Enhancements

This document tracks potential future enhancements and ideas. These are not currently planned for implementation but are worth considering for later.

---

## CLI Improvements

### Interactive Type Creation
Allow users to define new types interactively from the CLI:
```bash
rvn schema add-type
# Prompts for: type name, fields, default_path, etc.
# Writes to schema.yaml
```

**Why postponed**: Schema editing is infrequent; users can edit YAML directly.

---

### Interactive Trait Creation
Similar to type creation, but for traits:
```bash
rvn schema add-trait
```

---

## Query Enhancements

### Full-Text Search
Add FTS5 support for searching note content:
```bash
rvn search "compound interest"
```

**Status**: Mentioned in Phase 2 of spec.

---

### Date Range Queries
Support relative date queries in filters:
```bash
rvn tasks --due this-week
rvn tasks --due overdue
rvn trait remind --at today
```

**Status**: Partially supported, needs robust implementation.

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

## Adding New Ideas

When you think of a potential enhancement:
1. Add it to this file with a brief description
2. Note any context for why it was postponed
3. Reference the spec section if applicable
