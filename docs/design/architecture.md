# Architecture (design)

## High-level

Raven’s source of truth is plain markdown files. The SQLite index is a derived cache.

Key subsystems:
- `internal/parser/`: frontmatter, headings→sections, `::type(...)`, traits, refs
- `internal/schema/`: schema loading + validation
- `internal/index/`: SQLite schema + indexing + query plumbing
- `internal/query/`: Raven Query Language (RQL) parsing/validation/execution
- `internal/cli/`: CLI commands + JSON output envelope
- `internal/commands/`: command registry (metadata used by CLI + MCP)
- `internal/mcp/`: MCP server (generates tools from registry)

## Invariants

- Markdown files are the only durable data store.
- Object IDs are derived from file path (and `#fragment` for embedded/sections).
- `::type(...)` overrides the section created by the heading, but only when it appears on the next line.
- Schema drives indexing: undefined traits are not indexed; unknown frontmatter keys are validation errors.

