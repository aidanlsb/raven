<p align="center">
  <img src="raven-logo.svg" alt="Raven" width="180" />
</p>

<h1 align="center">Raven</h1>

---

Raven adds three things to markdown notes:
- A lightweight schema for defining types and fields
- Syntax for annotating content with traits
- Bidirectional linking between objects

And enables precise retrieval and powerful workflows with:
- A full-featured CLI
- An efficient query language
- First-class agent support

Everything stays local — plain text files you own and can read with any tool.

---

## Example Usage

Hermod — Odin's messenger in Norse mythology, and our PKM enthusiast for this walkthrough — tracks three things: **projects** that need resolution, **people** involved, and **meetings** where words become binding.

He uses Raven to keep it all straight. Types — `project`, `person`, `meeting` — each map to a folder of markdown files. Traits — `@todo`, `@decision` — are inline annotations that make content queryable. The rest is just markdown.

---

**Starting the day**

Each morning, Hermod opens his daily note:

```
rvn daily
```

This creates and opens `daily/2026-01-17.md` — a running log for the day.

```markdown
---
type: daily
date: 2026-01-17
---

# Saturday

@todo Bring the terms to Vanaheim before the new moon
@todo Follow up with Skirnir on his contacts there

Still waiting to hear back from Odin.
```

**Creating the project**

A diplomatic mission to Vanaheim is taking shape. Hermod opens a project to track it:

```
rvn new project vanaheim-embassy --field status=active
```

This creates `project/vanaheim-embassy.md` — a markdown file with YAML frontmatter holding the fields, with the body free for notes.

**Adding people**

Two names will recur. Skírnir has dealt with Vanaheim before; Forseti must approve any terms that touch on old grievances:

```
rvn new person skirnir --field realm=asgard --field role=envoy
rvn new person forseti --field realm=asgard --field role=arbiter
```

Now `[[person/skirnir]]` and `[[person/forseti]]` can be used as references anywhere in Hermod's notes, which link back to their own markdown files.

**Recording the meeting**

After the council, Hermod writes up the meeting:

```markdown
---
type: meeting
date: 2026-01-17
project: [[project/vanaheim-embassy]]
---

# Council at Glaðsheim

[[person/skirnir]] reports that Vanaheim is willing to negotiate, but wants concessions on the eastern trade routes.

[[person/forseti]] warns this may reopen an older land dispute.

@todo Bring the terms to Odin and await his decision
@todo Confirm Forseti has reviewed the old grievances

@decision No commitments until Odin speaks.
```

The traits `@todo` and `@decision` make the content of Hermod's note queryable, and the references connect this meeting to the people and project involved.

**Querying what's open**

Days later, Hermod checks what remains unresolved from meetings linked to the Vanaheim embassy:

```
rvn query "trait:todo within(object:meeting .project==vanaheim-embassy)"
```

```
meeting/2026-01-17-council-gladsheim.md
  @todo Bring the terms to Odin and await his decision
  @todo Confirm Forseti has reviewed the old grievances
```

Two open threads, both traceable to the meeting they came from.

**Reviewing references**

Skírnir arrives at Hermod's hall with news. Before they speak, Hermod pulls up everything connected to him:

```
rvn backlinks skirnir
```

```
meeting/2026-01-17-council-gladsheim.md
  [[person/skirnir]] reports that Vanaheim is willing to negotiate

meeting/2026-01-08-vanaheim-embassy.md
  [[person/skirnir]] negotiated the terms with Freyr's blessing

project/vanaheim-embassy.md
  Prior contact: [[person/skirnir]]
```

Everything Hermod has ever written about Skírnir, pulled up in one command.

**Asking an agent**

Odin summons him. Before the meeting, Hermod asks his agent:

> "What should I report on the Vanaheim matter?"

The agent calls Raven's MCP tools — querying open todos, recent meetings, and decisions linked to the project — and returns a synthesis:

> Two open todos from the council meeting on Jan 17. Vanaheim wants concessions on the eastern trade routes — no commitment has been made yet (@decision). Forseti's review of old grievances is still outstanding. Bottom line: one blocker before Odin can decide.

The agent doesn't need to search or guess — it queries structured data and reasons over what it finds.

---

## Installation

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
rvn version
```

Initialize a vault:

```bash
rvn init hermod-vault
cd hermod-vault
```

This creates:
- `schema.yaml` — type and trait definitions
- `raven.yaml` — vault configuration and saved queries
- `.raven/` — index and internal state (disposable, rebuildable)

> Requires Go 1.22+. See [Install Go](https://go.dev/doc/install).

---

## Agent Setup

Raven is designed to work with LLM agents. The MCP server exposes every Raven command as a tool.

**MCP Server (Claude Desktop, Cursor, etc.)**

Add to MCP configuration:

```json
{
  "mcpServers": {
    "raven": {
      "command": "rvn",
      "args": ["serve", "--vault-path", "/path/to/your/vault"]
    }
  }
}
```

Once connected, ask the agent:

> "Help me set up my vault. I want to track projects, people I work with, and meetings."

The agent will walk through schema creation, creating your first objects, and learning the query language — all through conversation.

See the full [MCP reference](docs/reference/mcp.md) for configuration options and available tools.

---

## Vault Structure

Everything is plain text.

```
hermod-vault/
├── .raven/
│   └── index.db          # SQLite index (disposable)
├── schema.yaml            # type and trait definitions
├── raven.yaml             # vault config, saved queries, workflows
├── daily/
│   └── 2026-02-17.md
├── project/
│   └── vanaheim-embassy.md
├── person/
│   ├── skirnir.md
│   └── forseti.md
└── meeting/
    └── 2026-01-17-council-gladsheim.md
```

- **Folders map to types** — `person/` holds all `person` objects
- **Files are markdown with YAML frontmatter** — structured fields up top, freeform content below
- **The SQLite index is disposable** — delete it and run `rvn reindex` to rebuild from files
- **Files are the source of truth** — edit them with any editor, sync with cloud storage or git

See the [file format reference](docs/reference/file-format.md) for the full specification.

---

## Core Concepts

### Types

Types define the kinds of objects in your vault. Each type has fields, a default folder, and a display name field.

```yaml
# schema.yaml
version: 2

types:
  project:
    name_field: name
    default_path: project/
    fields:
      name:
        type: string
        required: true
      status:
        type: enum
        values: [backlog, active, paused, done]
        default: active

  person:
    name_field: name
    default_path: person/
    fields:
      name:
        type: string
        required: true
      realm:
        type: string
      role:
        type: string
```

Edit `schema.yaml` directly, or build it from the CLI:

```bash
rvn schema add type meeting --name-field name --default-path meeting/
rvn schema add field meeting date --type date
rvn schema add field meeting attendees --type ref[] --target person
```

### Traits

Traits are inline annotations that make content queryable. They can appear anywhere in the body of a file.

```markdown
@todo Bring the terms to Vanaheim before the new moon
@decision No commitments until Odin speaks
@priority(high) The Bifrost repairs cannot wait
```

Define traits in the schema:

```bash
rvn schema add trait priority --type enum --values low,medium,high
```

### References

References (refs) connect objects using `[[type/name]]` syntax. Use them in both frontmatter and content.

```markdown
---
type: meeting
project: [[project/vanaheim-embassy]]
attendees:
  - [[person/skirnir]]
  - [[person/forseti]]
---

[[person/skirnir]] reported back from his talks with Freyr.
```

Every ref creates a two-way link. Use `rvn backlinks person/skirnir` to see everything that mentions Skírnir.

See [Core Concepts](docs/guide/core-concepts.md) for a deeper introduction, and the [schema reference](docs/reference/schema.md) for the full specification.

---

## Creating Objects

**New typed objects**

```bash
rvn new project "Vanaheim Embassy" --field status=active
rvn new person Skirnir --field realm=asgard --field role=envoy
```

**Daily notes**

Dates are a built-in type that powers the daily note workflow.

```bash
rvn daily              # today
rvn daily yesterday    # yesterday's note
rvn daily 2026-02-14   # specific date
```

**Quick capture**

Append to any file from the command line. Defaults to the daily note.

```bash
rvn add "Heard rumors of unrest in Niflheim — worth investigating"
rvn add "@todo Confirm Forseti reviewed the old grievances" --to project/vanaheim-embassy
```

**Editing files**

Files are plain markdown — edit them with any editor:

```bash
rvn open project/vanaheim-embassy    # opens in $EDITOR
```

---

## Querying

The query language has two modes: **object queries** find files, **trait queries** find annotations.

**Object queries**

```bash
rvn query 'object:project'                           # all projects
rvn query 'object:project .status==active'            # active projects
rvn query 'object:meeting refs([[person/skirnir]])'     # meetings mentioning Skírnir
```

**Trait queries**

```bash
rvn query 'trait:todo'                                # all todos
rvn query 'trait:todo .value==todo'                   # open todos only
rvn query 'trait:todo within(object:meeting)'         # todos inside meetings
rvn query 'trait:decision refs([[project/vanaheim-embassy]])'  # decisions on a project
```

**Full-text search**

```bash
rvn search "trade routes"                             # search all content
rvn search "Freyr" --type meeting                     # search within a type
```

**Bulk operations on query results**

```bash
rvn query 'trait:todo .value==todo' --apply 'update value=done' --confirm
```

**Saved queries**

Define common queries in `raven.yaml`:

```yaml
queries:
  open-todos:
    query: "trait:todo .value==todo"
    description: Open todos
  active-projects:
    query: "object:project .status==active"
    description: Active projects
```

```bash
rvn query open-todos
rvn query active-projects
```

See the [query language reference](docs/reference/query-language.md) for the full syntax including boolean composition, nested queries, and date predicates.

---

## Workflows

Workflows are multi-step pipelines that combine Raven tools with agent reasoning. Raven executes deterministic `tool` steps, then hands off to the agent at `agent` steps.

```yaml
# workflows/meeting-prep.yaml
description: Prepare a brief for a meeting
inputs:
  meeting_id:
    type: ref
    target: meeting
    required: true

steps:
  - id: meeting
    type: tool
    tool: raven_read
    arguments:
      path: "{{inputs.meeting_id}}"
      raw: true

  - id: compose
    type: agent
    prompt: |
      Prepare me for this meeting.
      {{steps.meeting.data.content}}
    outputs:
      markdown:
        type: markdown
        required: true
```

Run via CLI or agent:

```bash
rvn workflow run meeting-prep --input meeting_id=meeting/2026-01-17-council-gladsheim
```

See the [workflows guide](docs/guide/workflows.md) and [workflows reference](docs/reference/workflows.md) for the full specification.

---

## Documentation

Raven ships its documentation inside the binary. Browse it with `rvn docs`, or read it on GitHub:

**Guides** (learning path):

1. [Getting Started](docs/guide/getting-started.md) — first-session flow and verification
2. [Core Concepts](docs/guide/core-concepts.md) — types, traits, references
3. [Schema Introduction](docs/guide/schema-intro.md) — practical `schema.yaml` basics
4. [Configuration](docs/guide/configuration.md) — `raven.yaml` and `config.toml`
5. [CLI Basics](docs/guide/cli-basics.md) — everyday commands
6. [CLI Advanced](docs/guide/cli-advanced.md) — bulk operations and power workflows
7. [Templates](docs/guide/templates.md) — type and daily templates
8. [Workflows](docs/guide/workflows.md) — multi-step agent pipelines

**References** (lookup):

- [CLI Reference](docs/reference/cli.md) — every command and flag
- [Query Language](docs/reference/query-language.md) — full RQL syntax
- [Schema Reference](docs/reference/schema.md) — `schema.yaml` specification
- [File Format](docs/reference/file-format.md) — markdown + frontmatter spec
- [Vault Config](docs/reference/vault-config.md) — `raven.yaml` reference
- [MCP Reference](docs/reference/mcp.md) — agent integration
- [Workflows Reference](docs/reference/workflows.md) — pipeline specification
- [Bulk Operations](docs/reference/bulk-operations.md) — patterns for operating at scale

Interactive learning:

```bash
rvn learn         # guided lessons on Raven concepts
rvn docs          # browse the full documentation
```

---
