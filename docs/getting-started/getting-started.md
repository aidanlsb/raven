# Getting Started

## What is Raven?

Raven is a structured, plain-text knowledge base.

You keep your real data in Markdown files, then Raven gives you:
- a schema for structure (`type`, fields, traits)
- references between notes (`[[...]]`)
- fast local queries over an index you can always rebuild

The goal is simple: keep notes durable and human-readable without giving up structured workflows.

## Installation

Raven requires Go 1.22+.

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
rvn version
```

If `rvn` is not on your `PATH` after install, add your Go bin directory (usually `$(go env GOPATH)/bin`) to your shell profile.

## Agent setup

If you use an MCP-capable client (Codex, Claude Desktop, Cursor, etc.), run Raven as an MCP server:

```bash
rvn serve --vault-path /path/to/vault
```

For Claude Desktop, add:

```json
{
  "mcpServers": {
    "raven": {
      "command": "rvn",
      "args": ["serve", "--vault-path", "/path/to/vault"]
    }
  }
}
```

Optional: install Raven's bundled agent skill pack for your runtime:

```bash
rvn skill list --json
rvn skill install raven-core --target codex --confirm --json
```

For complete MCP details and tool reference, see `agents/mcp.md`.

## Setting up your vault

Create a new vault and enter it:

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

## Daily notes

Daily notes are the fastest capture loop in Raven:

```bash
rvn daily
rvn daily yesterday
rvn daily 2026-02-22
```

- `rvn daily` opens or creates today's note.
- The daily folder is configured with `directories.daily` in `raven.yaml`.


After you can create notes and daily entries, define your data model:
1. Read `types-and-traits/schema-intro.md` for the first safe schema changes.
2. Use `types-and-traits/schema.md` for the full reference.
3. Use `getting-started/core-concepts.md` if you want a quick refresher on objects, traits, and references.
