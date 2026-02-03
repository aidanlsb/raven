# AGENTS.md

## Project Overview

Raven is a structured, plain-text knowledge base built in Go. Markdown files with YAML frontmatter are the sole source of truth. A SQLite index (under `.raven/`) is a derived cache that can be rebuilt at any time with `rvn reindex`. The CLI binary is `rvn`, and an MCP server exposes the same commands as tools for AI agents.

## Design Principles

**Explicit over implicit.** There should be no "magic" behavior. Do not infer user intent unless it is unambiguous. If a feature requires guessing what the user means, it needs a clearer interface instead.

**General-purpose primitives.** Outside of a few special cases (daily notes), features must not be specific to a particular workflow. Raven provides core knowledge-management primitives — types, traits, references, queries — not opinionated workflow tools.

**No duplicated functionality.** Before implementing anything, check whether an existing package already handles it. Write clean, shared implementations. If two commands need the same logic, extract it into a shared internal package rather than duplicating code.

## Architecture

```
cmd/rvn/main.go          CLI entry point (thin wrapper)
internal/
  cli/                    Cobra command implementations (one file per command)
  commands/               Command registry — single source of truth for CLI + MCP metadata
  parser/                 Markdown → structured objects (frontmatter, traits, refs, sections)
  schema/                 Schema loading, validation, type/trait definitions
  index/                  SQLite index: schema, indexing, query plumbing
  query/                  Raven Query Language (RQL) parsing and execution
  model/                  Canonical type definitions (Object, Trait, Reference, etc.)
  mcp/                    MCP server (JSON-RPC 2.0, generates tools from command registry)
  vault/                  Vault operations (walk, dates, editor integration)
  config/                 Global config (~/.config/raven/config.toml) and vault config (raven.yaml)
  workflow/               Workflow definitions and execution
  paths/                  Path normalization (trailing slashes, no leading slashes)
  resolver/               Reference resolution
  template/               Template rendering
  testutil/               Shared test helpers
docs/
  guide/                  User-facing documentation
  reference/              Schema, file format, query language, MCP reference
  design/                 Architecture and design decision docs
```

### Key Invariants

- Markdown files are the only durable data store. The SQLite index is always rebuildable.
- Object IDs are derived from file paths (plus `#fragment` for embedded objects/sections).
- Schema drives indexing: undefined traits are not indexed; unknown frontmatter keys are validation errors.
- The command registry (`internal/commands/registry.go`) is the single source of truth for command metadata, shared by both CLI and MCP.

### Data Flow

```
Markdown files → Parser → Index (SQLite) ← Query Executor
                   ↓
             Schema validation
                   ↓
             CLI / MCP output
```

## Language and Tooling

- **Go 1.22+** — all code in standard Go style
- **Build:** `go build -o rvn ./cmd/rvn` or `make build`
- **Formatting:** `gofmt -s` and `goimports` with local prefix `github.com/aidanlsb/raven`
- **Linting:** `golangci-lint` (see `.golangci.yml` for enabled linters)
- **Dependencies:** pure-Go SQLite (`modernc.org/sqlite`), Cobra for CLI, goldmark for markdown

## Building and Testing

```bash
make build              # Build the rvn binary
make test               # Unit tests (fast, -race enabled)
make test-integration   # Integration tests (builds CLI binary, -race enabled)
make test-all           # Both unit + integration
make lint               # Run golangci-lint
make fmt                # Format code (gofmt + goimports)
make check              # fmt-check + lint + test (run before submitting)
```

### Testing Conventions

- Use Go's standard `testing` package. No external test frameworks.
- **Table-driven tests** are the primary pattern — define test cases as struct slices.
- Test files go alongside implementation: `foo.go` → `foo_test.go`.
- Integration tests use build tag `//go:build integration` and live in `internal/cli/` and `internal/mcp/`.
- Integration tests use `testutil.BuildCLI(t)` and `testutil.RunCLI(t, vaultPath, args...)` to exercise the real binary.
- Shared test helpers are in `internal/testutil/`.
- Test fixtures live in `testdata/`.

## Code Conventions

### Package Structure

- All implementation code lives under `internal/`. Nothing is exported outside the module.
- Each package has a single responsibility. No circular dependencies.
- `internal/model/` defines canonical types used across all layers — do not define parallel types elsewhere.
- CLI commands in `internal/cli/` map one-to-one to `.go` files.

### Error Handling

- Use structured error codes defined in `internal/cli/errors.go` (e.g., `ErrTypeNotFound`, `ErrRefAmbiguous`). These codes are stable and agents depend on them.
- All CLI commands support `--json` output with a standard envelope: `{ ok, data, error?, warnings?, meta? }`.
- Warning codes are also defined in `errors.go` (e.g., `WarnBacklinks`, `WarnUnknownField`).

### CLI Patterns

- Every command must be registered in `internal/commands/registry.go` with full metadata (description, args, flags, examples, use cases). This registry drives both Cobra command generation and MCP tool schema generation.
- Bulk operations use `--stdin` to read IDs from stdin. They return a preview by default; changes require `--confirm`.
- Always pass `--json` in non-interactive / agent contexts.

### Path Conventions

- Internal paths use trailing slashes for directories, no leading slashes.
- All path manipulation goes through `internal/paths/`.

### Import Ordering

Imports should be grouped in this order (enforced by `goimports`):

1. Standard library
2. Third-party packages
3. Local packages (`github.com/aidanlsb/raven/...`)

## Documentation

Keep documentation in sync with code. There are three main documentation surfaces:

### Command Registry (single source of truth)

`internal/commands/registry.go` defines all command metadata: descriptions, arguments, flags, examples, and agent use cases. This registry drives both the CLI (`--help`) and MCP tool schemas — update it once, and both stay in sync.

When adding or modifying a command:
1. Update the `Registry` map in `internal/commands/registry.go`
2. MCP tools are regenerated automatically from the registry

### MCP Agent Guide

`internal/mcp/agent-guide/` contains markdown files that are embedded at compile time and served as MCP resources (`raven://guide/*`). These provide agent-specific guidance:

| File | Purpose |
|------|---------|
| `index.md` | Navigation and topic discovery |
| `critical-rules.md` | Safety rules agents must follow |
| `getting-started.md` | First steps in a new vault |
| `core-concepts.md` | Types, traits, references explained |
| `querying.md` | RQL reference and query strategy |
| `query-cheatsheet.md` | Common query patterns |
| `key-workflows.md` | Creation, editing, bulk operations |
| `error-handling.md` | Interpreting tool errors |
| `issue-types.md` | `raven_check` issue reference |
| `best-practices.md` | Operating principles |
| `examples.md` | Example agent conversations |

When adding a new guide topic:
1. Create the markdown file in `internal/mcp/agent-guide/`
2. Add an entry to `guideTopics` in `internal/mcp/agent_guide.go`
3. Rebuild — files are embedded at compile time

### User Documentation

`docs/` contains user-facing documentation:

- `docs/guide/` — getting started, CLI usage, workflows
- `docs/reference/` — schema format, query language, MCP API
- `docs/design/` — architecture and design decisions

When making significant changes, check if `docs/reference/` needs updating (especially `mcp.md` for MCP changes, `cli.md` for CLI changes).

## What to Check Before Submitting

1. `make fmt` — code must be formatted
2. `make lint` — no linter errors
3. `make test` — all unit tests pass
4. If you changed CLI commands or flags, verify `internal/commands/registry.go` is updated to match
5. If you added a new package, make sure it doesn't introduce circular imports
6. If you touched parsing, indexing, or query execution, run `make test-integration`
7. If you changed command behavior, check if `internal/mcp/agent-guide/` docs need updating
8. If you added MCP resources or changed tool schemas, update `docs/reference/mcp.md`
