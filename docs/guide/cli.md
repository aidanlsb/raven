# CLI Guide

This guide covers common patterns and workflows for using Raven from the command line. For the complete command reference, see `reference/cli.md`.

## Quick Start

```bash
# Create a vault
rvn init ~/notes

# Open today's daily note
rvn daily

# Quick capture to daily note
rvn add "Quick thought"

# Create a typed object
rvn new person "Freya"

# Query your data
rvn query "object:person"
```

---

## Daily Workflow

### Morning Routine

```bash
# Open today's daily note
rvn daily --edit

# See what's due today
rvn date today

# Check overdue items
rvn query "trait:due value:past"
```

### Throughout the Day

```bash
# Quick capture (goes to daily note)
rvn add "Idea for project X"

# Capture with trait
rvn add "@priority(high) Follow up with Thor"

# Capture to specific file
rvn add "Meeting notes" --to projects/website.md

# Timestamped entry
rvn add "Standup complete" --timestamp
```

### Navigation

```bash
# Open by short name
rvn open freya                    # Opens people/freya.md

# Open yesterday's note
rvn daily yesterday

# See what happened on a date
rvn date 2025-02-01
```

---

## Creating Content

### New Typed Objects

```bash
# Basic creation
rvn new person "Freya"
rvn new project "Website Redesign"

# With fields
rvn new person "Thor Odinson" --field email=thor@asgard.realm
rvn new book "Harry Potter" --field author=people/jk-rowling

# Check what fields a type needs
rvn schema type person
```

**Tip:** If a type has `name_field` configured, the title auto-populates that field. Check with `rvn schema type <name>`.

### Adding to Existing Files

```bash
# Add to daily note (default)
rvn add "Note to self"

# Add to specific file
rvn add "New task" --to projects/website.md

# Add with references
rvn add "Discussed [[projects/website]] with [[people/thor]]"

# Add with traits
rvn add "@due(2025-02-15) Submit proposal"
```

---

## Querying

### Object Queries

Find objects of a specific type:

```bash
# All projects
rvn query "object:project"

# Active projects
rvn query "object:project .status:active"

# People with email
rvn query "object:person .email:*"

# Projects without a status
rvn query "object:project !.status:*"
```

### Trait Queries

Find trait annotations:

```bash
# All due items
rvn query "trait:due"

# Overdue items
rvn query "trait:due value:past"

# Due this week
rvn query "trait:due value:this-week"

# Highlights in books
rvn query "trait:highlight on:book"
```

### Relationship Queries

```bash
# Meetings mentioning Freya
rvn query "object:meeting refs:[[people/freya]]"

# Meetings in daily notes
rvn query "object:meeting parent:date"

# Projects with any todos (including nested)
rvn query "object:project contains:todo"

# Traits anywhere inside a specific project
rvn query "trait:due within:[[projects/website]]"
```

### Full-Text Search

When structured queries aren't enough:

```bash
rvn search "meeting notes"
rvn search "project*" --type project
rvn search '"exact phrase"'
rvn search "freya OR thor"
```

### Saved Queries

Create shortcuts for queries you run often:

```bash
# Add a saved query
rvn query add overdue "trait:due value:past" --description "Overdue items"
rvn query add tasks "trait:todo" --description "All tasks"

# Run saved queries by name
rvn query overdue
rvn query tasks

# List saved queries
rvn query --list

# Remove a saved query
rvn query remove old-query
```

---

## Bulk Operations

### Preview Before Applying

All bulk operations preview by default. Add `--confirm` to apply.

```bash
# Preview setting status on overdue items
rvn query "trait:due value:past" --apply "set status=overdue"

# Apply after reviewing
rvn query "trait:due value:past" --apply "set status=overdue" --confirm
```

### Common Bulk Operations

```bash
# Set a field on query results
rvn query "object:project .status:active" --apply "set reviewed=true" --confirm

# Add text to matching files
rvn query "object:project .status:active" --apply "add @reviewed(2025-02-01)" --confirm

# Move to directory
rvn query "object:project .status:archived" --apply "move archive/projects/" --confirm

# Delete (moves to .trash/)
rvn query "object:project .status:archived" --apply "delete" --confirm
```

### Piping Workflow

For complex operations, pipe IDs between commands:

```bash
# Get IDs for piping
rvn query "object:project .status:active" --ids

# Pipe to set
rvn query "object:project .status:active" --ids | rvn set --stdin priority=high --confirm

# For trait queries, use --object-ids to get containing objects
rvn query "trait:due value:past" --object-ids | rvn add --stdin "@reviewed" --confirm
```

---

## Editing Content

### Update Frontmatter Fields

```bash
# Set a single field
rvn set people/freya email=freya@asgard.realm

# Set multiple fields
rvn set projects/website status=active priority=high

# Use short references
rvn set freya status=active
```

### Surgical Text Edits

For precise content changes:

```bash
# Preview the edit
rvn edit "daily/2025-02-01.md" "- Old text" "- New text"

# Apply it
rvn edit "daily/2025-02-01.md" "- Old text" "- New text" --confirm

# Add a wiki link
rvn edit "notes.md" "Freya mentioned" "[[people/freya|Freya]] mentioned" --confirm

# Delete text (empty replacement)
rvn edit "notes.md" "- Obsolete item\n" "" --confirm
```

---

## File Organization

### Moving Files

```bash
# Rename in place
rvn move people/loki people/loki-archived

# Move to new directory
rvn move inbox/task.md projects/website/task.md

# Move without updating references
rvn move old-file.md archive/old-file.md --update-refs=false
```

### Deleting Files

```bash
# Check what references the file first
rvn backlinks projects/old-project

# Delete (moves to .trash/)
rvn delete projects/old-project

# Force delete without confirmation
rvn delete projects/old-project --force
```

---

## Schema Management

### Understanding Your Schema

```bash
# List all types
rvn schema types

# See type details
rvn schema type person

# List all traits
rvn schema traits

# See trait details  
rvn schema trait due
```

### Adding to Schema

```bash
# Add a new type
rvn schema add type book --name-field title --default-path books/

# Add a trait
rvn schema add trait priority --type enum --values high,medium,low

# Add a field to a type
rvn schema add field person company --type ref --target company
```

### Modifying Schema

```bash
# Update type settings
rvn schema update type person --name-field name

# Add trait to type
rvn schema update type meeting --add-trait due

# Update enum values
rvn schema update trait priority --values critical,high,medium,low
```

### Schema Cleanup

```bash
# Validate schema
rvn schema validate

# Remove unused type (files become 'page' type)
rvn schema remove type old-type

# Remove trait (annotations stop being indexed)
rvn schema remove trait unused-trait
```

---

## Vault Health

### Check for Issues

```bash
# Run full vault validation
rvn check

# Check a specific file (by path or reference)
rvn check people/freya.md
rvn check freya

# Check a directory
rvn check projects/

# Check all objects of a type
rvn check --type project

# Check all usages of a trait
rvn check --trait due

# Only check specific issue types
rvn check --issues missing_reference,unknown_type

# Exclude certain issue types
rvn check --exclude unused_type,unused_trait

# Only show errors (skip warnings)
rvn check --errors-only

# Common issues:
# - unknown_type: File uses undefined type
# - missing_reference: Broken [[link]]
# - undefined_trait: @trait not in schema
# - missing_required_field: Required field not set
# - unknown_frontmatter_key: Field not defined for type
# - invalid_enum_value: Value not in allowed list
```

### Fix Common Issues

```bash
# Add missing type
rvn schema add type book

# Create missing reference target
rvn new person "Freya"

# Add undefined trait
rvn schema add trait toread --type bool

# Set missing required field
rvn set people/freya name="Freya"
```

### Reindexing

```bash
# Incremental reindex (changed files only)
rvn reindex

# See what would be reindexed
rvn reindex --dry-run

# Full reindex (after schema changes)
rvn reindex --full
```

### Vault Statistics

```bash
# Overview of vault contents
rvn stats

# Find untyped pages
rvn untyped
```

---

## Workflows

Workflows are reusable prompt templates. See `guide/workflows.md` for details.

```bash
# List available workflows
rvn workflow list

# See workflow details
rvn workflow show meeting-prep

# Run a workflow
rvn workflow render meeting-prep --input meeting_id=meetings/team-sync
```

---

## Multiple Vaults

### Configuration

In `~/.config/raven/config.toml`:

```toml
default_vault = "work"
editor = "code"

[vaults]
work = "/path/to/work-notes"
personal = "/path/to/personal-notes"
```

### Switching Vaults

```bash
# Use named vault
rvn --vault personal daily

# Use explicit path
rvn --vault-path /path/to/vault daily

# List configured vaults
rvn vaults
```

---

## Tips & Patterns

### Capture Workflow

Set up a capture destination in `raven.yaml`:

```yaml
capture:
  destination: daily      # or "inbox.md" for a single file
  heading: "## Captured"  # Optional: append under this heading
  timestamp: false        # Optional: prefix with time
```

### Reference Resolution

Short references work when unambiguous:

```bash
rvn open freya           # Opens people/freya.md
rvn set freya status=active
rvn backlinks freya
```

Date references resolve to daily notes:

```bash
rvn open 2025-02-01      # Opens daily/2025-02-01.md
```

### JSON Output

For scripting, use `--json`:

```bash
rvn query "object:project" --json | jq '.results[].id'
rvn stats --json | jq '.object_count'
```

### Shell Completion

Enable tab completion:

```bash
# Bash
rvn completion bash > /etc/bash_completion.d/rvn

# Zsh
rvn completion zsh > "${fpath[1]}/_rvn"

# Fish
rvn completion fish > ~/.config/fish/completions/rvn.fish
```
