<p align="center">
  <img src="raven.svg" alt="Raven" width="180" />
</p>

<h1 align="center">Raven</h1>

**A CLI for plain-text knowledge management, with first-class support for AI agents.**

Raven keeps your notes in Markdown and adds a few features:
- Typed notes: define a `project` type with required yaml frontmatter fields, specified in a schema 
- Traits for annotations: create a `@priority` trait in your schema that you can use to tag important notes
- References: link your notes together with to create a `[[graph]]`

## Installation

Install with Homebrew:

```bash
brew tap aidanlsb/tap
brew install aidanlsb/tap/rvn
rvn version
```

Or install with Go:

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
rvn version
```

## Start With Plain Markdown

Initialize a vault:

```bash
rvn init ~/notes
cd ~/notes
```

Raven creates:

```text
notes/
├── .raven/       # derived cache and local metadata
├── raven.yaml    # vault configuration
└── schema.yaml   # types, fields, and traits
```

Your notes stay as normal Markdown files. The SQLite index under `.raven/` is disposable and can always be rebuilt with `rvn reindex`.

## Add Structure Only Where You Need It

Define types and fields in `schema.yaml`:

```yaml
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
```

Create typed objects from the CLI:

```bash
rvn new project website-redesign --field status=active
```

Or edit the file directly:

```markdown
---
type: project
name: Website Redesign
status: active
---
```

Use inline traits and references inside note content:

```markdown
@todo Confirm launch date with [[person/alex]]
@decision Delay rollout until analytics is fixed
```

Traits make content queryable. References create two-way links between objects.

## Query the Vault Precisely

Raven has separate query modes for objects and traits.

```bash
rvn query 'object:project .status==active'
rvn query 'trait:todo within(object:project)'
rvn search "analytics"
rvn backlinks person/alex
```

You can also operate on query results in bulk:

```bash
rvn query 'trait:todo .value==todo' --apply 'update done' --confirm
```

This makes Raven useful both as a personal CLI and as a reliable retrieval layer for agents.

## Work Naturally From the CLI

Common workflows stay simple:

```bash
rvn daily
rvn add "Follow up with finance" --to project/website-redesign
rvn open project/website-redesign
```

Everything still resolves back to files you can edit with any editor, sync with Git, or store however you want.

## Use Raven With Agents

Raven exposes its command surface through MCP, so agents can query and update the vault through structured tools instead of guessing from raw files.

Install into a supported MCP client:

```bash
rvn mcp install --client claude-desktop --vault-path /path/to/your/vault
rvn mcp install --client claude-code --vault-path /path/to/your/vault
rvn mcp install --client cursor --vault-path /path/to/your/vault
```

Check status or print a manual config snippet:

```bash
rvn mcp status
rvn mcp show --vault-path /path/to/your/vault
```

See the full [MCP reference](docs/agents/mcp.md) for configuration and tool details.

## Automate Repeatable Work

Raven workflows combine deterministic tool steps with agent reasoning.

```yaml
description: Prepare a meeting brief
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

  - id: brief
    type: agent
    prompt: |
      Summarize the meeting and list open actions.
      {{steps.meeting.data.content}}
```

Run a workflow from the CLI:

```bash
rvn workflow run meeting-prep --input meeting_id=meeting/2026-01-17-kickoff
```

See the [workflows reference](docs/workflows/workflows.md) for the full format.

## Documentation

- [Getting Started](docs/getting-started/getting-started.md)
- [Core Concepts](docs/getting-started/core-concepts.md)
- [Schema Introduction](docs/types-and-traits/schema-intro.md)
- [Schema Reference](docs/types-and-traits/schema.md)
- [File Format](docs/types-and-traits/file-format.md)
- [Query Language](docs/querying/query-language.md)
- [Bulk Operations](docs/vault-management/bulk-operations.md)
- [Workflows](docs/workflows/workflows.md)
- [MCP Reference](docs/agents/mcp.md)

You can also browse the docs from the CLI:

```bash
rvn docs
```
