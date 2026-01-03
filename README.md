<p align="center">
  <img src="assets/raven-logo.svg" width="120" alt="Raven logo">
</p>

<h1 align="center">Raven</h1>

<p align="center">
  <strong>
    Plain markdown notes with custom schemas and annotations for querying.<br>
    Built around a CLI that enables agent-driven workflows.
  </strong>
</p>

---

**Write notes in any editor:**

```markdown
# Thursday, January 2, 2026

- @due(today) Send [[clients/midgard]] proposal
- @due(tomorrow) Get Bifrost access for [[people/heimdall]]
- @highlight Buffer time is the key to good estimates
```

**Query them on the command line:**

```bash
$ rvn query "trait:due value:today"
daily/2026-01-02.md:3   "Send [[clients/midgard]] proposal"   @due(today)

$ rvn backlinks clients/midgard
daily/2026-01-02.md     "Send [[clients/midgard]] proposal"
projects/bifrost.md     "Client: [[clients/midgard]]"
```

**Ask an AI agent:**

```
You: "What's due this week?"
Agent: You have 3 items due - the Bifrost proposal for Midgard Corp is highest priority.

You: "Create a project for the Asgard security audit"
Agent: Created projects/asgard-security-audit.md with due date next Friday.
```

---

## Table of Contents

- [Getting Started](#getting-started)
- [Core Concepts](#core-concepts)
  - [File Format](#file-format)
  - [Schema Definition](#schema-definition)
  - [Types](#types)
  - [Traits](#traits)
  - [References](#references)
  - [Querying](#querying)
- [Configuration](#configuration)
  - [Schema (schema.yaml)](#schema-schemayaml)
  - [Vault Config (raven.yaml)](#vault-config-ravenyaml)
  - [Global Config](#global-config-configravenconfigtoml)
- [CLI Reference](#cli-reference)
  - [Core Commands](#core-commands)
  - [Query Commands](#query-commands)
  - [Creating & Editing](#creating--editing)
  - [Daily Notes & Dates](#daily-notes--dates)
  - [Schema Management](#schema-management)
  - [Shell Completion](#shell-completion)
- [AI Agent Integration](#ai-agent-integration)
  - [Setting Up with Claude Desktop](#setting-up-with-claude-desktop)
  - [What Agents Can Do](#what-agents-can-do)
  - [Example Agent Interactions](#example-agent-interactions)
- [Design Philosophy](#design-philosophy)
- [Documentation](#documentation)
- [Development](#development)
- [License](#license)

---

## Getting Started

### Installation

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
```

Or build from source:

```bash
git clone https://github.com/aidanlsb/raven.git
cd raven
go build -o rvn ./cmd/rvn
go install ./cmd/rvn  # Install to $GOPATH/bin
```

### Initialize a Vault

```bash
rvn init ~/notes
```

This creates:
- `raven.yaml` — vault configuration
- `schema.yaml` — your types and traits definitions
- `.raven/` — index directory (gitignored)
- `.gitignore` — ignores derived files

### Configure Your Default Vault

Create a global config file so `rvn` knows where your vault is:

```bash
mkdir -p ~/.config/raven
```

Then create `~/.config/raven/config.toml`:

```toml
default_vault = "/Users/you/notes"
editor = "cursor"  # or "code", "vim", etc.
```

Now you can run `rvn` commands from anywhere without specifying `--vault`.

### Start Writing

```bash
# Open today's daily note
rvn daily

# Quick capture a thought
rvn add "Call Odin about the Bifrost ceremony"

# Build the index
rvn reindex
```

That's it! Now let's understand how Raven files work.

---

## Core Concepts

Raven extends plain markdown with three ideas:

- **Types** — what things are (person, project, meeting)
- **Traits** — queryable structured annotations on content (`@due`, `@priority`)
- **References** — wiki-style links between notes (`[[people/freya]]`)

Types and traits are defined in your `schema.yaml`. 

### File Format

A Raven file is just markdown with optional YAML frontmatter:

```markdown
---
type: project
status: active
client: "[[clients/midgard]]"
---

# Bifrost Reconstruction

Rebuilding the rainbow bridge between realms.

- @due(2025-02-15) Finalize rune designs
- @due(2025-03-01) Design review with [[people/freya]]
- @highlight The crystalline approach is working well
```

**What's happening here:**
- `type: project` — this file is a "project" (defined in your schema)
- `status`, `client` — fields defined on the project type
- `@due(...)` — inline traits (queryable annotations)
- `@highlight` — a boolean trait (no value needed)
- `[[...]]` — references to other notes

### Schema Definition

Your `schema.yaml` defines what types and traits exist in your vault:

```yaml
version: 2

types:
  person:
    default_path: people/      # Where new persons are created
    fields:
      name: { type: string, required: true }
      email: { type: string }

  project:
    default_path: projects/
    fields:
      name: { type: string, required: true }
      client: { type: ref, target: client }
      status:
        type: enum
        values: [active, paused, completed]
        default: active

traits:
  due:
    type: date                 # Queryable with date filters

  priority:
    type: enum
    values: [low, medium, high]
    default: medium

  highlight:
    type: boolean              # Just @highlight, no value
```

The schema is the source of truth. Types and traits not defined here won't be indexed or queryable. Run `rvn check --create-missing` to interactively add undefined items to your schema.

### Types

Types define what objects **are**. They live in frontmatter:

```markdown
---
type: person
name: Freya
email: freya@asgard.realm
---

# Freya

Senior engineer on the platform team.
```

**Built-in types:**
- `page` — fallback for files without explicit type
- `date` — daily notes (auto-assigned to `YYYY-MM-DD.md` files)

Create typed objects with the CLI:

```bash
rvn new person "Freya"        # → people/freya.md
rvn new project "Bifrost Rebuild"  # → projects/bifrost-rebuild.md
```

### Traits

Traits are **single-valued annotations** on content. They appear inline:

```markdown
- @due(2025-02-03) Send the proposal
- @priority(high) Review security audit
- @highlight This insight changed everything
```

Or in frontmatter (for types that declare them):

```markdown
---
type: project
due: 2025-06-30
priority: high
---
```

**Query traits:**

```bash
rvn query "trait:due value:today"      # Items due today
rvn query "trait:due value:past"       # Overdue items
rvn query "trait:priority value:high"  # High priority items
rvn query "trait:highlight"            # All highlights
```

**"Tasks" are emergent**: Raven doesn't have a built-in task type. Anything with `@due` or `@status` is effectively a task. Define what "tasks" means in your workflow using saved queries.

### References

**References** link notes together using wiki-style syntax:

```markdown
Met with [[people/freya]] about [[projects/bifrost]].
```

References auto-slugify: `[[people/Thor Odinson]]` resolves to `people/thor-odinson.md`.

```bash
rvn backlinks people/freya       # What references Freya?
```

### Querying

Raven indexes your vault into SQLite, making structured data queryable. The main query patterns:

**By trait** — find content with specific annotations:

```bash
rvn query "trait:due"                   # Everything with @due
rvn query "trait:due value:today"       # Due today
rvn query "trait:due value:past"        # Overdue
rvn query "trait:priority value:high"   # High priority
```

**By type** — list objects of a kind:

```bash
rvn query "object:person"               # All people
rvn query "object:project"              # All projects
rvn query "object:project .status:active"  # Active projects only
```

**By reference** — find connections:

```bash
rvn backlinks people/freya       # What mentions Freya?
```

**Full-text search** — search content:

```bash
rvn search "bifrost design"      # Search all content
```

**Query language** — powerful queries using the Raven query language:

```bash
# Object queries
rvn query "object:project .status:active"
rvn query "object:meeting has:due"
rvn query "object:meeting parent:date"
rvn query "object:meeting refs:[[people/freya]]"

# Trait queries  
rvn query "trait:due value:past"
rvn query "trait:highlight on:{object:book .status:reading}"

# Saved queries (defined in raven.yaml)
rvn query tasks                  # Run your "tasks" query
rvn query overdue                # Run your "overdue" query
```

Query syntax:
- `object:<type>` — query objects of a type
- `trait:<name>` — query traits by name
- `.field:value` — filter by field value
- `has:trait` — filter objects that have a trait
- `refs:[[target]]` — filter by what objects reference
- `on:type` / `within:type` — filter traits by parent object
- `parent:type` / `ancestor:type` — filter by hierarchy
- `!pred` — negate a predicate
- `pred1 | pred2` — OR predicates

See [docs/QUERY_LOGIC.md](docs/QUERY_LOGIC.md) for full query language documentation.

---

## Configuration

### Schema (`schema.yaml`)

Full schema example with types, traits, and field types:

```yaml
version: 2

types:
  person:
    default_path: people/
    fields:
      name: { type: string, required: true }
      email: { type: string }

  client:
    default_path: clients/
    fields:
      name: { type: string, required: true }
      contact: { type: ref, target: person }

  project:
    default_path: projects/
    fields:
      client: { type: ref, target: client }
      status:
        type: enum
        values: [active, paused, completed]
        default: active
    traits:
      due: { required: true }  # Projects must have due dates
      priority: {}             # Optional

traits:
  due:
    type: date

  priority:
    type: enum
    values: [low, medium, high]
    default: medium

  status:
    type: enum
    values: [todo, in_progress, done]
    default: todo

  highlight:
    type: boolean
```

**Field types:** `string`, `date`, `datetime`, `enum`, `bool`, `ref`

**Trait types:** `date`, `datetime`, `enum`, `boolean`, `string`

### Vault Config (`raven.yaml`)

Configure vault behavior and saved queries:

```yaml
daily_directory: daily

# Auto-reindex after CLI operations (default: true)
# Commands like 'rvn add', 'rvn new', 'rvn set', 'rvn edit' will
# automatically update the index when they modify files.
auto_reindex: true

# Quick capture settings
capture:
  destination: daily      # or a file path like "inbox.md"
  heading: "## Captured"  # Optional heading to append under
  timestamp: false        # Prefix captures with time

# Deletion behavior
deletion:
  behavior: trash         # "trash" (default) or "permanent"
  trash_dir: .trash       # Directory for trashed files

# Saved queries
queries:
  tasks:
    traits: [due, status]
    filters:
      status: "!done"              # NOT done
    description: "Open tasks"

  overdue:
    traits: [due]
    filters:
      due: past
    description: "Overdue items"

  urgent:
    traits: [due]
    filters:
      due: "this-week|past"        # OR: this week or overdue
    description: "Due soon or overdue"
```

**Filter Syntax:**
- Simple value: `status: done` → exact match
- OR with pipe: `due: "this-week|past"` → matches either
- NOT with bang: `status: "!done"` → excludes value
- Date keywords: `today`, `yesterday`, `tomorrow`, `this-week`, `next-week`, `past`, `future`

### Global Config (`~/.config/raven/config.toml`)

Configure default vault and editor:

```toml
default_vault = "work"
editor = "code"  # or "cursor", "vim", "nano", etc.

[vaults]
work = "/Users/you/work-notes"
personal = "/Users/you/personal-notes"
```

Use named vaults: `rvn --vault work stats`

---

## CLI Reference

### Core Commands

| Command | Description |
|---------|-------------|
| `rvn init <path>` | Initialize a new vault |
| `rvn check` | Validate vault (broken refs, schema errors) |
| `rvn check --create-missing` | Interactively create missing pages, types, and traits |
| `rvn reindex` | Rebuild the SQLite index |

### Query Commands

| Command | Description |
|---------|-------------|
| `rvn query "object:<type>"` | Query objects of a type |
| `rvn query "object:<type> .field:value"` | Query with field filter |
| `rvn query "trait:<name>"` | Query traits by name |
| `rvn query "trait:<name> value:<val>"` | Query traits with value filter |
| `rvn query <saved-name>` | Run a saved query |
| `rvn query --list` | List saved queries |
| `rvn query add <name>` | Create a saved query |
| `rvn query remove <name>` | Remove a saved query |
| `rvn backlinks <target>` | Show incoming references |
| `rvn search <query>` | Full-text search across vault |
| `rvn stats` | Index statistics |
| `rvn untyped` | List files using fallback 'page' type |

### Creating & Editing

| Command | Description |
|---------|-------------|
| `rvn new <type> [title]` | Create a new typed note |
| `rvn new <type> <title> --field key=value` | Create with field values |
| `rvn add <text>` | Quick capture to daily note |
| `rvn add <text> --to <file>` | Append to existing file |
| `rvn set <object_id> field=value...` | Update frontmatter fields |
| `rvn edit <path> <old> <new>` | Surgical text replacement (preview by default) |
| `rvn edit ... --confirm` | Apply the edit |
| `rvn delete <object_id>` | Delete an object (moves to trash) |
| `rvn delete <object_id> --force` | Delete without confirmation |
| `rvn move <source> <dest>` | Move/rename file within vault |
| `rvn move ... --update-refs` | Update all references (default: true) |

### Daily Notes & Dates

| Command | Description |
|---------|-------------|
| `rvn daily [date]` | Open/create a daily note |
| `rvn date [date]` | Show everything related to a date |

### Schema Management

| Command | Description |
|---------|-------------|
| `rvn schema` | Show schema overview |
| `rvn schema types` | List all types |
| `rvn schema traits` | List all traits |
| `rvn schema commands` | List available commands (for agents) |
| `rvn schema add type <name>` | Add a new type |
| `rvn schema add trait <name>` | Add a new trait |
| `rvn schema add field <type> <field>` | Add a field to a type |
| `rvn schema update type <name>` | Update a type (default path, add/remove traits) |
| `rvn schema update trait <name>` | Update a trait (type, values, default) |
| `rvn schema update field <type> <field>` | Update a field (type, required, default) |
| `rvn schema remove type <name>` | Remove a type (files become 'page') |
| `rvn schema remove trait <name>` | Remove a trait (instances remain) |
| `rvn schema remove field <type> <field>` | Remove a field from a type |
| `rvn schema validate` | Validate schema for errors |

### Shell Completion

Enable tab-completion for types and commands:

```bash
# Zsh (~/.zshrc)
source <(rvn completion zsh)

# Bash (~/.bashrc)
source <(rvn completion bash)
```

Then `rvn new per<TAB>` completes to `rvn new person`.

---

## AI Agent Integration

Raven is designed for seamless AI agent workflows via the **Model Context Protocol (MCP)**. Run Raven as an MCP server and let your AI assistant manage your knowledge base.

### Setting Up with Claude Desktop

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "raven": {
      "command": "/path/to/rvn",
      "args": ["serve", "--vault-path", "/path/to/your/notes"]
    }
  }
}
```

### What Agents Can Do

| Tool | Description |
|------|-------------|
| `raven_new` | Create new typed objects (person, project, meeting) |
| `raven_add` | Quick capture to daily note or existing files |
| `raven_set` | Update frontmatter fields on existing objects |
| `raven_read` | Read raw file content for context |
| `raven_delete` | Delete objects (moves to trash by default) |
| `raven_move` | Move/rename files within the vault (updates refs) |
| `raven_edit` | Surgical text replacement in vault files |
| `raven_search` | Full-text search across vault content |
| `raven_query` | Query objects/traits (ad-hoc or saved queries) |
| `raven_query_add` | Create a new saved query |
| `raven_query_remove` | Remove a saved query |
| `raven_backlinks` | Find what references an object |
| `raven_date` | Get all activity for a specific date |
| `raven_daily` | Open or create the daily note |
| `raven_stats` | Vault statistics |
| `raven_schema` | Discover types, traits, and available commands |
| `raven_schema_add_*` | Add types, traits, fields to schema |
| `raven_schema_update_*` | Update existing types, traits, fields |
| `raven_schema_remove_*` | Remove types, traits, fields (with safety checks) |
| `raven_schema_validate` | Validate schema correctness |

### Example Agent Interactions

> "Add Loki as a person, he's the trickster god"

Agent uses `raven_new` → gets "missing required field: name" → asks you → retries with the field value → creates `people/loki.md`

> "What tasks do I have due this week?"

Agent uses `raven_query` with `this-week` filter → returns structured results

> "Add a note to the Bifrost project about the new rune designs"

Agent uses `raven_add` with `--to projects/bifrost.md` → appends to existing file

### Structured JSON Output

All commands support `--json` for machine-readable output:

```bash
rvn query "trait:due value:today" --json
```

```json
{
  "ok": true,
  "data": {
    "results": [
      {
        "id": "...",
        "trait_type": "due",
        "value": "2025-01-01",
        "file_path": "projects/bifrost.md",
        "line": 12
      }
    ]
  },
  "meta": {
    "count": 1,
    "query_time_ms": 5
  }
}
```

---

## Design Philosophy

- **Plain-text first**: Markdown files are the source of truth, not the database
- **Portable**: Files sync via Dropbox/iCloud/git; index is local-only
- **Schema-driven**: Types and traits are user-defined, not hard-coded
- **Explicit over magic**: Frontmatter is the source of truth for types
- **Query-friendly**: SQLite index enables fast structured queries
- **Agent-native**: Built for AI workflows with structured JSON output and MCP

---

## Documentation

| Document | Description |
|----------|-------------|
| [docs/SPECIFICATION.md](docs/SPECIFICATION.md) | Complete technical specification (data model, file format, schema, database, MCP server) |
| [docs/AGENT_GUIDE.md](docs/AGENT_GUIDE.md) | Guide for AI agents on using Raven effectively |
| [docs/FUTURE.md](docs/FUTURE.md) | Planned and potential future enhancements |
| [docs/MIGRATIONS.md](docs/MIGRATIONS.md) | Schema and database migration guide |

---

## Development

### Building from Source

```bash
go build -o rvn ./cmd/rvn
go test ./...
```

### Architecture

```
internal/
├── commands/   # Command registry (single source of truth for CLI/MCP)
├── cli/        # Cobra command handlers
├── mcp/        # MCP server (generates tools from registry)
├── parser/     # Markdown parsing (frontmatter, traits, refs)
├── schema/     # Schema loading and validation
├── index/      # SQLite indexing and queries
├── config/     # Configuration loading (vault, global)
├── pages/      # Page creation logic
├── vault/      # Vault utilities (dates, file walking)
├── resolver/   # Reference resolution
├── check/      # Validation
└── audit/      # Audit logging
```

The **command registry** (`internal/commands/registry.go`) is the single source of truth for all CLI commands. This ensures:
- MCP tools are automatically generated from the same definitions
- `rvn schema commands` is always accurate
- Adding a new command is a single edit + handler

### Testing

```bash
go test ./...                    # All tests
go test ./internal/mcp/... -v    # MCP parity tests
go test ./internal/commands/...  # Registry tests
```

---

## License

MIT
