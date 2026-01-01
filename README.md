# Raven

A personal knowledge system with typed blocks, traits, and powerful querying. Built in Go for speed, with plain-text markdown files as the source of truth.

## Features

- **Typed Objects**: Define what things *are* (person, project, meeting, book)
- **Traits**: Add behavior/metadata to content (`@task`, `@remind`, `@highlight`)
- **References**: Wiki-style links between notes (`[[people/alice]]`)
- **Tags**: Lightweight categorization (`#productivity`)
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

# List all tasks
rvn tasks

# Query objects
rvn query "type:person"

# Show backlinks to a note
rvn backlinks people/alice

# Open today's daily note
rvn daily
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

```markdown
- @task(due=2025-02-03, priority=high) Send revised estimate
- @remind(2025-02-05T09:00) Follow up on this
- @highlight Key insight worth remembering
```

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
| `rvn tasks` | List tasks (alias for `rvn trait task`) |
| `rvn trait <name>` | Query any trait type |
| `rvn query "<query>"` | Query objects by type, tags, fields |
| `rvn backlinks <target>` | Show incoming references |
| `rvn stats` | Index statistics |
| `rvn untyped` | List files using fallback 'page' type |
| `rvn daily` | Open/create today's daily note |
| `rvn new --type <t> <title>` | Create a new typed note |

## Schema Configuration

Define types and traits in `schema.yaml` at your vault root:

```yaml
types:
  person:
    fields:
      name:
        type: string
        required: true
      email:
        type: string
    detect:
      path_pattern: "^people/"

  project:
    fields:
      status:
        type: enum
        values: [active, paused, completed]
        default: active

traits:
  task:
    fields:
      due:
        type: date
      status:
        type: enum
        values: [todo, in_progress, done]
        default: todo
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
- **Query-friendly**: SQLite index enables fast structured queries

## License

MIT
