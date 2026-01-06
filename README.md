<p align="center">
  <img src="raven-logo.svg" width="120" alt="Raven logo">
</p>

<h1 align="center">Raven</h1>

<p align="center">
  <strong>
    Plain markdown notes with custom schemas and annotations for querying.<br>
    Built around a CLI that enables agent-driven workflows.
  </strong>
</p>

> ⚠️ **Experimental:** Raven is super early and under active development. Everything is subject to change!

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
You: "What should I focus on today?"
Agent: You have 2 overdue items blocking others:
       • Heimdall needs Bifrost access (blocks the Midgard project)
       • Proposal review for Thor (he's asked twice)
       Plus 3 items due today. Want me to add these to your daily note?

You: "Midgard call went great - security audit approved. Freya's leading it, 
      starts Monday, 50k budget."
Agent: Created projects/midgard-security-audit.md linked to [[clients/midgard]].
       Set Freya as lead, status: active. Added kickoff task @due(Monday) 
       to your daily note.
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
  - [Templates](#templates)
  - [Vault Config (raven.yaml)](#vault-config-ravenyaml)
  - [Global Config](#global-config-configravenconfigtoml)
- [CLI Reference](#cli-reference)
  - [Core Commands](#core-commands)
  - [Query Commands](#query-commands)
  - [Creating & Editing](#creating--editing)
  - [Daily Notes & Dates](#daily-notes--dates)
  - [Schema Management](#schema-management)
  - [Workflows](#workflows-1)
  - [Shell Completion](#shell-completion)
- [AI Agent Integration](#ai-agent-integration)
  - [Setting Up with Claude Desktop](#setting-up-with-claude-desktop)
  - [What Agents Can Do](#what-agents-can-do)
  - [Workflows](#workflows)
  - [Example Agent Interactions](#example-agent-interactions)
- [Design Philosophy](#design-philosophy)
- [Workflow Tips](#workflow-tips)
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

- **Types** — schema definitions for what things are (e.g., `person`, `project`, `meeting`)
- **Traits** — queryable structured annotations on content (`@due`, `@priority`)
- **References** — wiki-style links between notes (`[[people/freya]]`)

Files are **objects** — instances of types. For example, `people/freya.md` is an object of type `person`.

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

**Types** are schema definitions that describe what objects are. Each file declares its type in frontmatter, making it an **object** (instance) of that type:

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

Traits are **single-valued annotations** in content using `@trait` or `@trait(value)` syntax:

```markdown
- @due(2025-02-03) Send the proposal
- @priority(high) Review security audit
- @highlight This insight changed everything
```

**Query traits:**

```bash
rvn query "trait:due value:today"      # Items due today
rvn query "trait:due value:past"       # Overdue items
rvn query "trait:priority value:high"  # High priority items
rvn query "trait:highlight"            # All highlights
```

**Concepts emerge from traits**: For example, Raven doesn't have built-in "tasks"—but you might define tasks as anything with `@due`, or items marked `@status(todo)`. Saved queries let you codify these patterns for your workflow.

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

Raven has two foundational query types that compose together for powerful information retrieval:

#### Object Queries

Find objects (files or embedded sections) by type and filter by their fields:

```bash
rvn query "object:person"                    # All people
rvn query "object:project .status:active"    # Active projects only
rvn query "object:meeting .attendees:[[people/freya]]"  # Freya's meetings
```

#### Trait Queries

Find traits (inline annotations like `@due`, `@priority`) across your vault:

```bash
rvn query "trait:due"                    # Everything with @due
rvn query "trait:due value:today"        # Due today
rvn query "trait:due value:past"         # Overdue items
rvn query "trait:highlight"              # All highlights
```

#### Composing Queries

**Important constraint:** Each query returns items of exactly one type. `object:project` returns only projects; `trait:due` returns only `@due` traits. This constraint enables composition; you can nest queries inside each other to specify complicated query conditions.

Use `{...}` to embed one query inside another:

```bash
# Objects that HAVE certain traits
rvn query "object:project has:{trait:due value:past}"
# → Projects with overdue tasks

# Traits that appear ON certain objects
rvn query "trait:highlight on:{object:book}"
# → Highlights from books

# Traits WITHIN a hierarchy (any ancestor)
rvn query "trait:due within:{object:date}"
# → All @due items in daily notes
```

You can also filter by **relationships**:

```bash
# Objects that reference something
rvn query "object:meeting refs:[[people/freya]]"

# Objects with a specific parent type
rvn query "object:meeting parent:{object:date}"
# → Meetings embedded in daily notes
```

And combine with **boolean logic**:

```bash
# OR: active or planning projects
rvn query "object:project (.status:active | .status:planning)"

# NOT: everything except done
rvn query "trait:status !value:done"
```

#### Saved Queries

Define common queries in `raven.yaml` and run them by name:

```bash
rvn query tasks          # Run your "tasks" query
rvn query overdue        # Run your "overdue" query
rvn query --list         # List all saved queries
```

#### Full-Text Search

Search across all content (not just structured data):

```bash
rvn search "bifrost design"         # Find pages mentioning both words
rvn search '"world tree"'           # Exact phrase
rvn search "meet*" --type meeting   # Prefix match, filtered by type
```

#### Backlinks

Find what references a specific object:

```bash
rvn backlinks people/freya      # What mentions Freya?
rvn backlinks projects/bifrost  # What links to this project?
```

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

# Traits: Universal annotations (use anywhere in content)
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

### Templates

Templates provide default content when creating new notes. Define a `template` field on any type:

```yaml
# schema.yaml
types:
  meeting:
    default_path: meetings/
    template: templates/meeting.md    # File-based template
    fields:
      time: { type: datetime }
      attendees: { type: string }

  quick-note:
    template: |                        # Inline template
      # {{title}}
      
      ## Notes
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

**Template Variables:**

| Variable | Description | Example |
|----------|-------------|---------|
| `{{title}}` | Title passed to `rvn new` | "Team Sync" |
| `{{slug}}` | Slugified title | "team-sync" |
| `{{type}}` | The type name | "meeting" |
| `{{date}}` | Today's date | "2026-01-02" |
| `{{datetime}}` | Current datetime | "2026-01-02T14:30" |
| `{{year}}`, `{{month}}`, `{{day}}` | Date components | "2026", "01", "02" |
| `{{weekday}}` | Day name | "Monday" |
| `{{field.X}}` | Value of field X (from `--field`) | `{{field.time}}` |

**Daily Note Templates:**

Configure in `raven.yaml`:

```yaml
daily_directory: daily
daily_template: templates/daily.md   # Or inline template
```

With `templates/daily.md`:

```markdown
# {{weekday}}, {{date}}

## Morning

## Afternoon

## Evening
```

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

# Saved queries - use the Raven query language
queries:
  tasks:
    query: "trait:due"
    description: "All tasks with due dates"

  overdue:
    query: "trait:due value:past"
    description: "Overdue items"

  urgent:
    query: "trait:due value:this-week|past"
    description: "Due soon or overdue"

  active-projects:
    query: "object:project .status:active"
    description: "Active projects"
```

Saved queries use the same query language as `rvn query "..."`. See the [Query Language](#query-language) section for full syntax.

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
| `rvn check --strict` | Treat warnings as errors |
| `rvn check --by-file` | Group issues by file path |
| `rvn check --create-missing` | Interactively create missing pages, types, and traits |
| `rvn reindex` | Rebuild the SQLite index |
| `rvn reindex --smart` | Only reindex changed files |

### Query Commands

| Command | Description |
|---------|-------------|
| `rvn query "object:<type>"` | Query objects of a type |
| `rvn query "object:<type> .field:value"` | Query with field filter |
| `rvn query "trait:<name>"` | Query traits by name |
| `rvn query "trait:<name> value:<val>"` | Query traits with value filter |
| `rvn query <saved-name>` | Run a saved query |
| `rvn query --list` | List saved queries |
| `rvn query add <name> <query>` | Create a saved query |
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

### Workflows

| Command | Description |
|---------|-------------|
| `rvn workflow list` | List available workflows |
| `rvn workflow show <name>` | Show workflow details and inputs |
| `rvn workflow render <name>` | Render workflow with context |
| `rvn workflow render <name> --input key=value` | Provide input values |

### Advanced Commands

| Command | Description |
|---------|-------------|
| `rvn watch` | Auto-reindex on file changes (background) |
| `rvn lsp` | Start LSP server for editor integration |
| `rvn read <path>` | Read raw file content |
| `rvn path` | Print the vault path |
| `rvn vaults` | List configured vaults |

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
| `raven_new` | Create new objects of a type (e.g., `person`, `project`) |
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
| `raven_check` | Validate vault against schema |
| `raven_reindex` | Rebuild the index |
| `raven_workflow_list` | List available workflows |
| `raven_workflow_show` | Show workflow details |
| `raven_workflow_render` | Render a workflow with inputs |

### Workflows

Workflows are **reusable prompt templates** for agents. Define structured tasks once, then run them with different inputs. Each workflow can gather context from your vault before generating a prompt.

#### Defining Workflows

Add workflows to your `raven.yaml`:

```yaml
workflows:
  person-summary:
    description: "Generate a summary of a person and their work"
    inputs:
      person_id:
        type: ref
        target: person
        required: true
        description: "The person to summarize"
    context:
      person:
        read: "{{inputs.person_id}}"
      related:
        backlinks: "{{inputs.person_id}}"
    prompt: |
      Summarize this person based on their profile and related items.
      
      ## Person
      {{context.person}}
      
      ## Related Items
      {{context.related}}

  project-review:
    description: "Review active projects and suggest next steps"
    inputs:
      status:
        type: string
        default: "active"
    context:
      projects:
        query: "object:project .status:{{inputs.status}}"
    prompt: |
      Review these projects and suggest prioritized next steps.
      
      ## Projects
      {{context.projects}}

  research:
    description: "Research a topic across the vault"
    inputs:
      question:
        type: string
        required: true
    context:
      results:
        search: "{{inputs.question}}"
        limit: 10
    prompt: |
      Answer this question using the search results.
      
      Question: {{inputs.question}}
      
      ## Relevant Content
      {{context.results}}
```

You can also store workflows in external files:

```yaml
workflows:
  code-review:
    file: workflows/code-review.yaml
```

#### Context Query Types

| Type | Description |
|------|-------------|
| `read: <id>` | Read a single object by ID |
| `query: "<query>"` | Run a Raven query (type-constrained) |
| `backlinks: <id>` | Find objects that reference the target |
| `search: "<term>"` | Full-text search (unconstrained) |

#### Using Workflows

```bash
# List available workflows
rvn workflow list

# Show workflow details
rvn workflow show person-summary

# Render a workflow with inputs
rvn workflow render person-summary --input person_id=people/freya
```

When rendered, the workflow:
1. Validates inputs (reports missing required fields)
2. Runs all context queries
3. Substitutes variables in the prompt
4. Returns the complete prompt and gathered context

The result is ready for an AI agent to execute—Raven handles the context gathering, the agent handles the reasoning.

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

## Workflow Tips

Tips and tricks for using Raven effectively.

### Use Espanso for Relative Dates

When writing notes, you often want to reference dates relative to today—"due tomorrow", "meeting next Friday". [Espanso](https://github.com/espanso/espanso) is a cross-platform text expander that can automatically insert calculated dates.

Example Espanso config (`~/.config/espanso/match/raven.yml`):

```yaml
matches:
  # @due shortcuts
  - trigger: ":dtd"
    replace: "@due({{today}})"
    vars:
      - name: today
        type: date
        params:
          format: "%Y-%m-%d"

  - trigger: ":dtm"
    replace: "@due({{tomorrow}})"
    vars:
      - name: tomorrow
        type: date
        params:
          format: "%Y-%m-%d"
          offset: 86400  # +1 day in seconds

  - trigger: ":dnw"
    replace: "@due({{next_week}})"
    vars:
      - name: next_week
        type: date
        params:
          format: "%Y-%m-%d"
          offset: 604800  # +7 days

  # Quick date insertion
  - trigger: ":td"
    replace: "{{today}}"
    vars:
      - name: today
        type: date
        params:
          format: "%Y-%m-%d"
```

Now typing `:dtm` expands to `@due(2025-01-05)` (tomorrow's actual date).

---

## Documentation

| Document | Description |
|----------|-------------|
| [docs/SPECIFICATION.md](docs/SPECIFICATION.md) | Complete technical specification (data model, file format, schema, database, MCP server) |
| [docs/AGENT_GUIDE.md](docs/AGENT_GUIDE.md) | Guide for AI agents on using Raven effectively |
| [docs/WORKFLOWS_SPEC.md](docs/WORKFLOWS_SPEC.md) | Workflow system specification for reusable prompt templates |
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
└── check/      # Validation
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
