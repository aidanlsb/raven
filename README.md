# Raven

A personal knowledge system with typed blocks, traits, and powerful querying. Built in Go for speed, with plain-text markdown files as the source of truth.

## Features

- **Typed Objects**: Define what things *are* (person, project, meeting, book)
- **Traits**: Single-valued annotations on content (`@due`, `@priority`, `@highlight`)
- **References**: Wiki-style links between notes (`[[people/alice]]`)
- **Tags**: Lightweight categorization (`#productivity`)
- **Saved Queries**: Define reusable queries for common workflows
- **SQLite Index**: Fast querying while keeping markdown as source of truth

## Installation

```bash
go install github.com/yourusername/raven/cmd/rvn@latest
```

Or build from source:

```bash
git clone https://github.com/yourusername/raven.git
cd raven
go build -o rvn ./cmd/rvn
```

## Quick Start

```bash
# Initialize a new vault
rvn init ~/notes

# Set as default vault
mkdir -p ~/.config/raven
echo 'vault = "/Users/you/notes"' > ~/.config/raven/config.toml

# Reindex all files
rvn reindex

# Validate your vault
rvn check

# Query traits
rvn trait due --value today        # Items due today
rvn trait due --value past         # Overdue items
rvn trait highlight                # All highlights

# Saved queries (defined in raven.yaml)
rvn query --list                   # List available queries
rvn query tasks                    # Run 'tasks' query
rvn query overdue                  # Run 'overdue' query

# Show backlinks to a note
rvn backlinks people/alice

# Daily notes
rvn daily                    # Today
rvn daily yesterday          # Yesterday
rvn daily 2025-02-01         # Specific date

# Date hub - see everything related to a date
rvn date                     # Today's date hub
rvn date yesterday           # Yesterday's date hub
```

## File Format

### Frontmatter (File-Level Type)

```markdown
---
type: person
name: Alice Chen
email: alice@example.com
---

# Alice Chen

Senior engineer on the platform team.
```

### Embedded Types

```markdown
## Weekly Standup
::meeting(id=standup, time=09:00, attendees=[[[people/alice]], [[people/bob]]])

Discussed Q2 priorities.
```

### Traits

Traits are **single-valued annotations** on content. Use multiple traits for multiple properties:

```markdown
- @due(2025-02-03) @priority(high) Send revised estimate
- @remind(2025-02-05T09:00) Follow up on this
- @highlight Key insight worth remembering
```

**"Tasks" are emergent**: Anything with `@due` or `@status` is effectively a task. Use saved queries to define what "tasks" means in your workflow.

### References & Tags

```markdown
Met with [[people/alice]] about [[projects/website]].

Some thoughts about #productivity today.
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `rvn init <path>` | Initialize a new vault |
| `rvn check` | Validate vault (broken refs, schema errors) |
| `rvn reindex` | Rebuild the SQLite index |
| `rvn trait <name>` | Query any trait type |
| `rvn query <name>` | Run a saved query |
| `rvn query --list` | List saved queries |
| `rvn backlinks <target>` | Show incoming references |
| `rvn stats` | Index statistics |
| `rvn untyped` | List files using fallback 'page' type |
| `rvn daily [date]` | Open/create a daily note |
| `rvn date [date]` | Show everything related to a date |
| `rvn new --type <t> <title>` | Create a new typed note |

## Configuration

### Schema (`schema.yaml`)

Define types and traits:

```yaml
types:
  person:
    default_path: people/
    fields:
      name:
        type: string
        required: true
      email:
        type: string

  project:
    default_path: projects/
    fields:
      status:
        type: enum
        values: [active, paused, completed]
        default: active

# Traits are single-valued annotations
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

### Vault Config (`raven.yaml`)

Configure vault behavior and saved queries:

```yaml
daily_directory: daily

queries:
  tasks:
    traits: [due, status]
    filters:
      status: "todo,in_progress,"
    description: "Open tasks"

  overdue:
    traits: [due]
    filters:
      due: past
    description: "Overdue items"
```

## Documentation

See [docs/SPECIFICATION.md](docs/SPECIFICATION.md) for the complete specification including:

- Data model and object hierarchy
- File format specification
- Schema configuration options
- Database schema
- Implementation details

## Design Philosophy

- **Plain-text first**: Markdown files are the source of truth, not the database
- **Portable**: Files sync via Dropbox/iCloud/git; index is local-only
- **Schema-driven**: Types and traits are user-defined, not hard-coded
- **Explicit over magic**: Frontmatter is the source of truth for types
- **Query-friendly**: SQLite index enables fast structured queries

## License

MIT
