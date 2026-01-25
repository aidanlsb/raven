<p align="center">
  <img src="raven.svg" alt="Raven" width="200" />
</p>

<h1 align="center">Raven</h1>

<p align="center">
  <strong>A structured, plain-text knowledge base for agent collaboration.</strong>
</p>

<p align="center">
  ⚠️ <em>Experimental: Raven is early and under active development.</em>
</p>

---

## What is Raven?
A Raven "vault" is a collection of markdown files with some additional syntax to define:
* **Types**: custom schemas with metadata for your notes (e.g., `projects`, `people`)
* **Traits**: inline annotations to enable efficient retrieval of content (e.g., `@todo`)
* **References**: bidirectional links `[[link-to-another-note]]` for networking your notes

Along with this syntax, Raven provides the following capabilities for interacting with your notes:
* **CLI**: capture information and manipulate your notes from the terminal
* **Queries**: leverage your schema for fast retrieval across files
* **MCP**: connect AI agents to your knowledge base 
* **Workflows**: compose all the above into reusable, multi-step operations


## Example Usage

### 1. Define a schema

```yaml
# schema.yaml
version: 2

types:
  person:
    name_field: name
    default_path: people/
    fields:
      name: { type: string, required: true }
      realm: { type: string }

  client:
    name_field: name
    default_path: clients/
    fields:
      name: { type: string, required: true }
    
  project:
    name_field: name
    default_path: projects/
    fields:
      name: { type: string, required: true }
      status: { type: enum, values: [active, paused, completed], default: active }
      lead: { type: ref, target: person }
      client: { type: ref, target: client }
      budget_usd: { type: number }

  meeting:
    default_path: meetings/
    fields:
      attendees: { type: ref[], target: person}
      project: { type: ref, target: project }

traits:
  due: { type: date }
  priority: { type: enum, values: [low, medium, high], default: medium }
  status: { type: enum, values: [todo, in_progress, done], default: todo }
  highlight: { type: boolean }
```

### 2. Write notes with structure

Open the automatically created daily note with `rvn daily` (opens in your editor of choice):

```markdown
# Friday, January 9, 2026

## Midgard Call Notes
::meeting(attendees=[[[Freya]]], project=[[Midgard Security Audit]])

- @due(2026-01-12) Get Bifrost access from [[Heimdall]]
- @highlight [[Freya]] thinks the bottleneck is review latency, not implementation.
```

Create a new project: `rvn new project "Midgard Security Audit"`:

```markdown
---
type: project
name: Midgard Security Audit
status: active
client: clients/midgard
lead: people/freya
---

Comprehensive security review of Midgard's infrastructure.

# Tasks
- @due(2026-01-12) @priority(high) Send kickoff doc to [[clients/midgard]]
- @due(2026-01-15) Draft audit plan
- @due(2026-01-20) Initial findings review
```


### 3. Query your knowledge

```bash
# High-priority tasks for Midgard 
rvn query "trait:priority .value==high refs:[[clients/midgard]]"

# All meetings with Freya that had a highlight
rvn query "object:meeting .attendees==[[Freya]] has:{trait:highlight}"
```

### 4. Use AI to manage your vault

Raven has a first-class MCP server that Claude, Cursor, etc. can use to read, query, and update your notes:

```
You: "Midgard call went great — security audit approved. Freya's leading it,
      starts Monday. Next step is talk to Heimdall by Tuesday. Capture this and create whatever pages we need."

Agent:
  → Creates projects/midgard-security-audit.md (type: project)
  → Sets lead and client via schema-validated fields
  → Appends notes with @due traits
```

The agent operates within your schema. Missing required fields get rejected. Invalid references get caught.

---

## Install

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest

# Initialize a new vault
rvn init /path/to/notes

```
---

## Documentation

**Getting started:**

- [Getting started guide](docs/guide/getting-started.md)
- [Core concepts](docs/guide/core-concepts.md)

**How-to guides:**

- [Configuration](docs/guide/configuration.md)
- [CLI usage](docs/guide/cli.md)
- [Workflows](docs/guide/workflows.md)

**Reference:**

- [File format](docs/reference/file-format.md)
- [Schema (`schema.yaml`)](docs/reference/schema.md)
- [Vault config (`raven.yaml`)](docs/reference/vault-config.md)
- [Query language](docs/reference/query-language.md)
- [Bulk operations](docs/reference/bulk-operations.md)
- [Workflows spec](docs/reference/workflows.md)
- [MCP tools & agents](docs/reference/mcp.md)
- [Agent guide](internal/mcp/agent-guide/index.md)


**Design docs:**

- [Architecture](docs/design/architecture.md)
- [Database / index](docs/design/database.md)
- [Migrations](docs/design/migrations.md)
- [LSP design notes](docs/design/lsp.md)
- [Future ideas](docs/design/future.md)

---

## License

MIT
