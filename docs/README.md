# Documentation

Raven’s docs are organized by intent:

- **Guide**: task-oriented usage and workflows.
- **Reference**: canonical specs for syntax/config/contracts.
- **Design**: implementation notes and internal design docs.

## Guide

- `guide/getting-started.md` — install, init a vault, first commands
- `guide/core-concepts.md` — types, traits, refs, sections, embedded types
- `guide/configuration.md` — `schema.yaml` + `raven.yaml` + templates/directories (how to)
- `guide/cli.md` — CLI usage patterns (not exhaustive)
- `guide/agents.md` — MCP setup + how to use Raven with agents
- `guide/agent-guide.md` — guidance for AI agents using Raven tools
- `guide/workflows.md` — defining and running workflows

## Reference

- `reference/file-format.md` — markdown + frontmatter + `::type()` rules
- `reference/schema.md` — `schema.yaml` reference
- `reference/vault-config.md` — `raven.yaml` reference (saved queries, workflows, etc)
- `reference/query-language.md` — Raven Query Language (RQL)
- `reference/bulk-operations.md` — `--ids`, `--stdin`, `--apply`, preview/confirm
- `reference/workflows.md` — workflow file format + CLI/MCP surfaces
- `reference/mcp.md` — MCP tool catalog and argument/response contracts

## Design

- `design/architecture.md` — code layout, invariants, high-level architecture
- `design/database.md` — SQLite schema + indexing strategy
- `design/migrations.md` — upgrade/migration philosophy + rules
- `design/lsp.md` — LSP implementation notes
- `design/future.md` — future ideas and non-committed explorations

