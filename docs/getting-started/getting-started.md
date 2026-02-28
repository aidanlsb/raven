# Getting Started

## What is Raven?

Fundamentally Raven is a CLI for personal knowledge management, with first-class support for AI agents.

You keep your data in a directory of markdown files (called a "vault"), and Raven lets you:
- Define a schema, so your notes can have concrete "types" (e.g., project, meeting, person)
- Create "traits" to add inline annotations to your notes (e.g., a `@todo` trait for task management)
- Add bidirectional links across notes using references (`[[page-to-reference]]`)
- Find and operate on notes using an efficient query language

The goal is to keep notes durable and simple while still enabling structured workflows and accurate agent operations.

## Installation and setup

Raven requires Go 1.22+.

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
rvn version
```

If `rvn` is not on your `PATH` after install, add your Go bin directory (usually `$(go env GOPATH)/bin`) to your shell profile.

Initialize your first vault:

```bash
rvn init ~/notes
cd ~/notes
```

`rvn init` creates the essential files:
- `raven.yaml` (vault behavior)
- `schema.yaml` (types, fields, traits)
- `.raven/` (derived index cache)

Quick sanity check:

```bash
rvn stats
rvn schema types
```

## Agent Setup

Raven is designed with the assumption that you will use an AI agent to interact with your vault (although this is by no means required). If you use an MCP-capable client (Codex, Claude Desktop, Cursor, etc.), install the Raven MCP entry directly from the CLI:

```bash
rvn mcp install --client claude-desktop --vault-path /path/to/vault
```

You can also install for other supported clients:

```bash
rvn mcp install --client claude-code --vault-path /path/to/vault
rvn mcp install --client cursor --vault-path /path/to/vault
```

Verify setup:

```bash
rvn mcp status
```

If you need a manual snippet (for unsupported clients), print it with:

```bash
rvn mcp show --vault-path /path/to/vault
```

Raven also ships with skills you can install for your agent(s) of choice.

```bash
rvn skill list --json
rvn skill install raven-core --target codex --confirm --json
```

For complete MCP details and tool reference, see `agents/mcp.md`.

## Daily notes

Daily notes are the simplest way to get started with Raven:
- `rvn daily` opens today's note
- The daily folder is configured with `directories.daily` in `raven.yaml`.

After you can create notes and daily entries, define your data model:
1. Read `types-and-traits/schema-intro.md` for the first safe schema changes.
2. Use `types-and-traits/schema.md` for the full reference.
3. Use `getting-started/core-concepts.md` if you want a quick refresher on objects, traits, and references.
