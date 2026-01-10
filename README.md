<p align="center">
  <img src="raven-logo.svg" width="120" alt="Raven logo">
</p>

<h1 align="center">Raven</h1>

<p align="center">
  <strong>
    Plain markdown notes with schemas, traits, and querying.<br>
    Built around a CLI that also exposes MCP tools for agents.
  </strong>
</p>

> ⚠️ **Experimental:** Raven is early and under active development.

## A compelling end-to-end example

Raven is just markdown files plus:
- **Schema** (`schema.yaml`) for typed frontmatter + embedded objects
- **Traits** (`@due(...)`, `@priority(...)`) for queryable annotations in text
- **Querying** (`rvn query ...`) across your whole vault
- **Agentic workflows** via MCP (`rvn serve`) so an assistant can safely operate on your vault

### 1) Define a schema

```yaml
# schema.yaml
version: 2

types:
  person:
    default_path: people/
    fields:
      name: { type: string, required: true }
      email: { type: string }
      alias: { type: string } # optional: enables [[alias]] resolution

  client:
    default_path: clients/
    fields:
      name: { type: string, required: true }

  project:
    default_path: projects/
    fields:
      status:
        type: enum
        values: [active, paused, completed]
        default: active
      lead: { type: ref, target: person }
      client: { type: ref, target: client }
      budget_usd: { type: number }

traits:
  due: { type: date }
  priority: { type: enum, values: [low, medium, high], default: medium }
  status: { type: enum, values: [todo, in_progress, done], default: todo }
  highlight: { type: boolean }
```

### 2) Write plain markdown with traits + refs

```markdown
# Friday, January 9, 2026

## Midgard Security Audit
::meeting(time=09:00, attendees=[[[people/freya]]])

- @due(2026-01-12) @priority(high) Send kickoff doc to [[clients/midgard]]
- @status(in_progress) Get Bifrost access from [[people/heimdall]]

## Notes
- @highlight Freya thinks the bottleneck is review latency, not implementation.
```

And a typed object file:

```markdown
---
type: project
status: active
client: "[[clients/midgard]]"
lead: "[[people/freya]]"
budget_usd: 50000
---

# Midgard Security Audit

## Tasks
- @due(2026-01-12) @priority(high) Kickoff doc
- @due(2026-01-15) Draft audit plan
```

### 3) Query it

```bash
# Overdue items anywhere in the vault
rvn query "trait:due value:past"

# All active projects
rvn query "object:project .status:active"

# Projects that contain overdue items anywhere in their hierarchy
rvn query "object:project contains:{trait:due value:past}"

# Tasks that mention Midgard (refs on the same line as the trait)
rvn query "trait:due refs:[[clients/midgard]]"
```

### 4) Use an agent (MCP)

Run Raven as an MCP server so an assistant can safely read/query/update your vault:

```bash
rvn serve --vault-path /path/to/vault
```

Example interaction:

```
You: Midgard call went great — security audit approved. Freya’s leading it,
     starts Monday, 50k budget. Capture this and create whatever pages we need.

Agent:
  - creates `projects/midgard-security-audit.md` (type: project)
  - sets lead = [[people/freya]], client = [[clients/midgard]], status = active, budget_usd = 50000
  - appends kickoff tasks to today’s daily note with @due(...) and links
  - suggests a meeting-prep workflow render if you have one configured
```

## Install

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
```

## Quickstart

```bash
rvn init /path/to/notes
rvn reindex
rvn daily
rvn query --list
```

## Documentation

Start here: `docs/README.md`.

## Development

```bash
go test ./...
go build ./cmd/rvn
```

## License

MIT

