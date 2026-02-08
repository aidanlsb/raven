<p align="center">
  <img src="raven.svg" alt="Raven" width="180" />
</p>

<h1 align="center">Raven</h1>

<p align="center">
  <strong>A plain-text knowledge base built for agent collaboration.</strong><br>
  <em>⚠️ Early & experimental</em>
</p>

---

Markdown notes with a schema and queries, CLI, and MCP integration.

## Quick Start

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
rvn init ~/notes && cd ~/notes
rvn daily  # Open today's note
```

---

## Example Usage

### 1. Create a typed note

Run `rvn new project "Bifrost"` to create a project with structured metadata:

```markdown
---
type: project
name: Bifrost
status: active
lead: people/freya
---

Cross-realm connectivity infrastructure.

## Goals
- Establish secure endpoints in each realm
- Sub-second latency for all connections
```

Types are defined in your schema — Raven validates fields and indexes everything for queries.

### 2. Use daily notes to capture info

Run `rvn daily` to open today's note. Reference projects, add todos, capture context:

```markdown
# Monday, January 12, 2026

## Bifrost sync
Met with [[Freya]] about [[Bifrost]] timeline.

- @todo @due(2026-01-15) Draft architecture proposal
- @todo @due(2026-01-14) Get Heimdall's input on security reqs
- @highlight Freya wants to prioritize Asgard-Midgard link first
```

**`[[references]]`** create bidirectional links. **`@traits`** annotate content with structured data.

### 3. Query across your vault

Find todos for Bifrost, wherever they live:

```bash
rvn query 'trait:todo refs(Bifrost)'
```

```
daily/2026-01-12.md:8   - @todo @due(2026-01-15) Draft architecture proposal
daily/2026-01-12.md:9   - @todo @due(2026-01-14) Get Heimdall's input on security reqs
```


```bash
# What's due this week?
rvn query 'trait:due .value<=2026-01-17'

# All highlights mentioning Freya
rvn query 'trait:highlight refs([[Freya]])'
```

### 4. Let AI agents help

Connect Claude, Cursor, or any MCP-compatible agent to your vault:

```
You: "Bifrost sync went well. Freya wants to fast-track the Asgard-Midgard 
      link. I need to loop in Heimdall on security by Wednesday. Update
      the project and add the todo."

Agent:
  → Opens projects/bifrost.md
  → Adds "Prioritizing Asgard-Midgard link per Freya" to notes
  → Appends "@todo @due(2026-01-14) Loop in Heimdall on security requirements"
```

The agent operates within your schema — required fields are enforced, references are validated.

---

## Table of Contents

- [Getting Started](#getting-started) — installation, first notes, basic commands
- [Configuration](#configuration) — global settings, vault config, schema
- [Core Concepts](#core-concepts) — types, traits, references, objects
- [Query Language](#query-language) — finding content across your vault
- [CLI Reference](#cli-reference) — command-line usage
- [MCP Integration](#mcp-integration) — connecting AI agents

---

## Getting Started

### Installation

Raven requires Go 1.22+.

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
```

### Initialize a Vault

```bash
rvn init ~/notes
cd ~/notes
```

This creates:

```
~/notes/
├── .raven/          # Index and internal state (gitignore this)
├── raven.yaml       # Vault configuration
└── schema.yaml      # Type and trait definitions
```

### Your First Notes

```bash
rvn daily                    # Open today's daily note
rvn new project "Bifrost"    # Create a typed object
rvn query 'trait:todo'       # Find all todos
```

### Writing Notes

Use standard markdown with Raven's additions:

```markdown
---
type: project
name: My Project
status: active
---

# My Project

## Tasks
- @todo @due(2026-01-15) First task
- @todo Second task

## Notes
- Met with [[Freya]] today
- @highlight Key insight worth remembering
```

### Basic Commands

| Command | Description |
|---------|-------------|
| `rvn daily` | Open today's note |
| `rvn new <type> "name"` | Create a new object |
| `rvn open <path>` | Open a file in your editor |
| `rvn read <path>` | Print file contents |
| `rvn query '<query>'` | Search your vault |
| `rvn check` | Validate files against schema |
| `rvn reindex` | Rebuild the index |

---

## Configuration

Raven uses three configuration files:

| File | Scope | Purpose |
|------|-------|---------|
| `~/.config/raven/config.toml` | Global | Editor, vault paths, default vault |
| `raven.yaml` | Per-vault | Daily notes, saved queries, workflows |
| `schema.yaml` | Per-vault | Types and traits |

### Global Config (`~/.config/raven/config.toml`)

Settings that apply across all vaults.

```toml
# Your preferred editor
editor = "cursor"  # or "vim", "code", "nvim", etc.

# Default vault (used when not in a vault directory)
default_vault = "work"

# Named vaults for quick access
[vaults]
work = "/path/to/work-notes"
personal = "/path/to/personal-notes"
```

With named vaults, run commands from anywhere:

```bash
rvn --vault work daily
rvn --vault personal query 'trait:todo'
```

### Vault Config (`raven.yaml`)

Settings for a specific vault.

```yaml
# Daily notes
daily_directory: daily
daily_template: |
  # {{weekday}}, {{date}}

  ## Tasks

  ## Notes

# Auto-reindex after CLI operations
auto_reindex: true

# Quick capture settings
capture:
  destination: daily       # or a file path like "inbox.md"
  heading: "## Captured"
  timestamp: true          # prefix with HH:MM

# Deletion behavior
deletion:
  behavior: trash          # or "permanent"
  trash_dir: .trash

# Saved queries (run with `rvn query <name>`)
queries:
  overdue:
    query: "trait:due .value==past"
    description: "Items past their due date"
  active-projects:
    query: "object:project .status==active"

# Directory organization (optional)
directories:
  object: object/          # Root for typed objects
  page: page/              # Root for untyped pages
```

**Daily template variables:** `{{date}}`, `{{weekday}}`, `{{year}}`, `{{month}}`, `{{day}}`

### Schema (`schema.yaml`)

Define the structure of your vault — types and traits.

```yaml
version: 2

types:
  project:
    name_field: name
    default_path: projects/
    fields:
      name: { type: string, required: true }
      status: { type: enum, values: [active, paused, completed], default: active }
      lead: { type: ref, target: person }

  person:
    name_field: name
    default_path: people/
    fields:
      name: { type: string, required: true }
      email: { type: string }

traits:
  todo: { type: boolean }
  due: { type: date }
  priority: { type: enum, values: [low, medium, high] }
  highlight: { type: boolean }
```

After changing the schema, rebuild the index:

```bash
rvn reindex --full
```

### Field Types

| Type | Description | Example |
|------|-------------|---------|
| `string` | Plain text | `name: { type: string }` |
| `number` | Numeric value | `priority: { type: number, min: 1, max: 5 }` |
| `bool` | Boolean | `archived: { type: bool, default: false }` |
| `date` | Date (YYYY-MM-DD) | `due: { type: date }` |
| `datetime` | Date and time | `time: { type: datetime }` |
| `enum` | Value from list | `status: { type: enum, values: [a, b, c] }` |
| `ref` | Reference to object | `owner: { type: ref, target: person }` |
| `string[]`, `ref[]`, etc. | Arrays | `tags: { type: string[] }` |

### Trait Types

| Type | Usage | Example |
|------|-------|---------|
| `boolean` | Presence-based | `@highlight` |
| `date` | Date value | `@due(2026-01-15)` |
| `datetime` | Date and time | `@remind(2026-01-15T09:00)` |
| `enum` | Value from list | `@priority(high)` |
| `string` | Free text | `@note(Remember to follow up)` |

### Schema CLI

```bash
# View schema
rvn schema types              # List all types
rvn schema type project       # Show type details
rvn schema traits             # List all traits

# Modify schema
rvn schema add type book --name-field title --default-path books/
rvn schema add trait priority --type enum --values high,medium,low
rvn schema add field person email --type string

# Validate
rvn schema validate           # Check schema consistency
rvn check                     # Validate files against schema
```

---

## Core Concepts

### Files and Objects

Every markdown file in your vault is an **object**. Objects have:
- A **type** (defined in frontmatter, defaults to `page`)
- **Fields** (frontmatter metadata validated against the schema)
- **Content** (the markdown body)

```markdown
---
type: project
name: Bifrost
status: active
---

Project content here...
```

### Sections and Embedded Objects

Headings create **section objects** automatically:

```markdown
# Project Name

## Tasks        ← Creates section "project-name#tasks"

## Meetings     ← Creates section "project-name#meetings"
```

You can also embed typed objects under headings with `::type(...)`:

```markdown
## Weekly Standup
::meeting(time=09:00, attendees=[[[people/freya]], [[people/thor]]])

Notes from the meeting...
```

### Types

Types define the schema for your objects. Each type specifies:
- **Fields** — what metadata the object can have
- **default_path** — where new files are created
- **name_field** — which field serves as the display name

Built-in types:
- `page` — default for files without explicit type
- `date` — daily notes
- `section` — auto-generated for headings

### Traits

Traits are inline annotations written as `@name` or `@name(value)`:

```markdown
- @todo @due(2026-01-15) Send the proposal
- @highlight This insight is worth remembering
- @priority(high) Urgent item
```

Traits must be defined in your schema to be indexed and queryable.

### References

Wiki-style links connect objects:

```markdown
Discussed [[Bifrost]] with [[Freya]] yesterday.
```

References are bidirectional — you can query both outgoing refs and backlinks.

**Reference formats:**
- `[[name]]` — resolved by name_field or filename
- `[[path/to/file]]` — explicit path
- `[[file#section]]` — link to a section
- `[[2026-01-15]]` — date reference (links to daily note)

---

## Query Language

Raven has two query types: **object queries** and **trait queries**.

### Object Queries

Find objects of a specific type:

```bash
# All projects
rvn query 'object:project'

# Active projects
rvn query 'object:project .status==active'

# Projects with overdue tasks
rvn query 'object:project encloses(trait:due .value==past)'
```

### Trait Queries

Find trait instances:

```bash
# All todos
rvn query 'trait:todo'

# Overdue items
rvn query 'trait:due .value==past'

# Todos on active projects
rvn query 'trait:todo within(object:project .status==active)'
```

### Field Predicates

| Syntax | Meaning |
|--------|---------|
| `.field==value` | Equals |
| `.field!=value` | Not equals |
| `.field>value` | Greater than |
| `.field<value` | Less than |
| `exists(.field)` | Field has value |
| `!exists(.field)` | Field is empty |

### Structural Predicates

| Predicate | Meaning |
|-----------|---------|
| `has(trait:...)` | Object has trait directly |
| `encloses(trait:...)` | Object or descendants have trait |
| `refs([[target]])` | References target |
| `refd([[source]])` | Referenced by source |
| `parent(object:...)` | Direct parent matches |
| `ancestor(object:...)` | Some ancestor matches |
| `content("term")` | Full-text search |

### Trait Predicates

| Predicate | Meaning |
|-----------|---------|
| `.value==val` | Trait value equals |
| `on(object:...)` | Trait is on object |
| `within(object:...)` | Trait is within object subtree |
| `at(trait:...)` | Co-located with another trait |
| `refs([[target]])` | Line references target |

### Boolean Operators

```bash
# AND (space)
rvn query 'object:project .status==active has(trait:due)'

# OR (|)
rvn query 'object:project (.status==active | .status==paused)'

# NOT (!)
rvn query 'object:project !.status==completed'
```

### Date Values

For date traits, use special values in queries:

| Value | Meaning |
|-------|---------|
| `past` | Before today |
| `today` | Today |
| `tomorrow` | Tomorrow |
| `this-week` | This week |
| `next-week` | Next week |

```bash
rvn query 'trait:due .value==past'      # Overdue
rvn query 'trait:due .value==this-week' # Due this week
```

---

## CLI Reference

### Daily Notes

```bash
rvn daily              # Open today's note
rvn daily yesterday    # Yesterday's note
rvn daily 2026-01-15   # Specific date
```

### Creating Objects

```bash
rvn new project "Bifrost"                              # Create with name
rvn new person "Freya" --field email=freya@asgard.io   # With fields
rvn new person "Thor"                                  # Prompts for required fields
```

### Querying

```bash
rvn query 'object:project'           # Find objects
rvn query 'object:person' --ids      # Just IDs (for piping)
rvn query 'object:project' --json    # JSON output (includes count in metadata)
rvn query overdue                    # Run a saved query
rvn query --list                     # List saved queries
```

### Reading and Editing

```bash
rvn read projects/bifrost            # Print file content
rvn open projects/bifrost            # Open in editor
rvn backlinks projects/bifrost       # Show what links here
```

### Bulk Operations

```bash
# Preview changes (default)
rvn query 'object:project .status==active' --ids | rvn set --stdin status=paused

# Apply changes
rvn query 'object:project .status==active' --ids | rvn set --stdin status=paused --confirm
```

### Validation

```bash
rvn check                  # Validate all files
rvn check projects/        # Validate directory
rvn reindex                # Rebuild index
rvn reindex --full         # Full rebuild (after schema changes)
```

---

## MCP Integration

Raven includes an MCP server that exposes all commands as tools for AI agents.

### Setup for Claude Desktop

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "raven": {
      "command": "rvn",
      "args": ["mcp", "--vault", "/path/to/your/vault"]
    }
  }
}
```

### Setup for Cursor

Add to `.cursor/mcp.json` in your project:

```json
{
  "mcpServers": {
    "raven": {
      "command": "rvn",
      "args": ["mcp", "--vault", "/path/to/your/vault"]
    }
  }
}
```

### Available Tools

The MCP server exposes these tools:

| Tool | Description |
|------|-------------|
| `raven_new` | Create a new typed object |
| `raven_add` | Append content to a file or daily note |
| `raven_daily` | Open or create a daily note |
| `raven_set` | Set frontmatter fields |
| `raven_edit` | Surgical text replacement in files |
| `raven_delete` | Delete an object (moves to trash) |
| `raven_move` | Move or rename an object |
| `raven_query` | Query objects and traits using RQL |
| `raven_search` | Full-text search across vault |
| `raven_read` | Read a file (raw or enriched) |
| `raven_backlinks` | Find objects that reference a target |
| `raven_check` | Validate vault against schema |
| `raven_schema` | Introspect the schema |
| `raven_reindex` | Rebuild the index |

### Agent Guide

The MCP server also exposes guidance resources at `raven://guide/*` that help agents understand how to use the tools effectively.

---

