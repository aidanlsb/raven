# Raven

**Markdown notes with structure to power collaboration with AI agents on your knowledge base.**

> ⚠️ **Experimental:** Raven is early and under active development.

---

Raven adds a few concepts and capabilities on stop of standard markdown:

- **Typed objects**: define types (person, project, etc.) with validated frontmatter fields
- **Traits**: custom inline annotations (e.g., `@due(2026-02-01)`) that are indexed and queryable
- **References**: `[[wiki-links]]` with validation and backlink tracking
- **Query language**: write and save queries with a rich syntax to retrieve your notes efficiently
- **CLI**: create, update, and navigate your notes from the command line
- **MCP**: expose your vault to AI agents with schema-aware tooling
- **Workflows**: combine your óstructured data with packaged prompts for agents to execute complex workflows consistentlyó

---

## Illustration

A consulting firm tracking people, clients, and projects.

### Schema

Define types and traits in `schema.yaml`:

```yaml
# schema.yaml
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

  project:
    default_path: projects/
    fields:
      status: { type: enum, values: [active, paused, completed], default: active }
      lead: { type: ref, target: person }
      client: { type: ref, target: client }
      budget_usd: { type: number }

traits:
  due: { type: date }
  priority: { type: enum, values: [low, medium, high], default: medium }
  status: { type: enum, values: [todo, in_progress, done], default: todo }
  highlight: { type: boolean }
```

### Notes

Open your daily note from the command line: `rvn daily`

```markdown
# Friday, January 9, 2026

## Midgard Security Audit
::meeting(time=09:00, attendees=[[[people/freya]]])

The call went well. Audit is approved — $50k budget, starting Monday.
Freya will lead. Need to get Heimdall to provision Bifrost access.

- @due(2026-01-12) @priority(high) Send kickoff doc to [[clients/midgard]]
- @status(in_progress) Get Bifrost access from [[people/heimdall]]

## Notes
- @highlight Freya thinks the bottleneck is review latency, not implementation.
```

A typed project file:

```markdown
---
type: project
status: active
client: "[[clients/midgard]]"
lead: "[[people/freya]]"
budget_usd: 50000
---

# Midgard Security Audit

Comprehensive security review of Midgard's infrastructure.

## Tasks
- @due(2026-01-12) @priority(high) Kickoff doc
- @due(2026-01-15) Draft audit plan
- @due(2026-01-20) Initial findings review
```

Still plain markdown — works in any editor.

### Queries

```bash
# What's overdue?
rvn query "trait:due value:past"

# All active projects
rvn query "object:project .status:active"

# High-priority tasks for Midgard
rvn query "trait:priority value:high refs:[[clients/midgard]]"

# Projects with overdue items somewhere inside them
rvn query "object:project contains:{trait:due value:past}"
```

### Agent usage (MCP)

Run as an MCP server:

```bash
rvn serve --vault-path /path/to/vault
```

Agents (Claude, Cursor, etc.) can then read, query, and update your vault:

```
You: "Midgard call went great — security audit approved. Freya's leading it,
      starts Monday, 50k budget. Capture this and create whatever pages we need."

Agent:
  → Creates projects/midgard-security-audit.md (type: project)
  → Sets lead, client, budget_usd via schema-validated fields
  → Appends tasks to today's daily note with @due traits
```

The agent operates within your schema. Missing required fields get rejected. Invalid references get caught. Automation that actually works.

---

## Install

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
```

## Quickstart

```bash
# Initialize a new vault
rvn init /path/to/notes

# Build the index
rvn reindex

# Open today's daily note
rvn daily

# See what queries are available
rvn query --list
```

---

## Documentation

**Getting started:**

- [Getting started guide](docs/guide/getting-started.md)
- [Core concepts](docs/guide/core-concepts.md)

**How-to guides:**

- [Configuration](docs/guide/configuration.md)
- [CLI usage](docs/guide/cli.md)
- [Working with agents](docs/guide/agents.md)
- [Agent guide](docs/guide/agent-guide.md)
- [Workflows](docs/guide/workflows.md)

**Reference:**

- [File format](docs/reference/file-format.md)
- [Schema (`schema.yaml`)](docs/reference/schema.md)
- [Vault config (`raven.yaml`)](docs/reference/vault-config.md)
- [Query language](docs/reference/query-language.md)
- [Bulk operations](docs/reference/bulk-operations.md)
- [Workflows spec](docs/reference/workflows.md)
- [MCP tools](docs/reference/mcp.md)

**Design docs:**

- [Architecture](docs/design/architecture.md)
- [Database / index](docs/design/database.md)
- [Migrations](docs/design/migrations.md)
- [LSP design notes](docs/design/lsp.md)
- [Future ideas](docs/design/future.md)

---

## Development

```bash
go test ./...
go build ./cmd/rvn
```

## License

MIT
