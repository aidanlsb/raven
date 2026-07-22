# AGENTS.md

## Project Overview

Raven is a structured, plain-text knowledge base built in Go. Markdown files with YAML frontmatter are the sole source of truth. A SQLite index (under `.raven/`) is a derived cache that can be rebuilt at any time with `rvn reindex`. The CLI binary is `rvn`, and an MCP server exposes the same commands as tools for AI agents.

## Quick Start for Agents

- Read `internal/commands/registry.go` before adding/changing commands; it is the CLI + MCP metadata source of truth.
- Prefer existing shared packages in `internal/` over adding command-local logic.
- Preserve compatibility contracts: stable error/warning codes and the JSON response envelope.
- If command behavior changes, update both implementation and registry metadata in the same change.
- Run `make test` for code changes; also run `make test-integration` for parsing/index/query/CLI behavior changes.
- Keep docs in sync when behavior changes (registry-derived help, MCP guide, and relevant `docs/` pages).
- Before submitting, run `make fmt`, `make lint`, and relevant tests.

## Raven Design Principles

**Explicit over implicit.** There should be no "magic" behavior. Raven should not infer user intent unless it is unambiguous. If a feature requires guessing what the user means, it needs a clearer interface instead.

**General-purpose primitives.** Outside of a few special cases (daily notes), features must not be specific to a particular workflow. Raven provides core knowledge-management primitives — types, traits, references, queries — not opinionated workflow tools.

**No duplicated functionality.** Before implementing anything, check whether an existing package already handles it. Write clean, shared implementations. If two commands need the same logic, extract it into a shared internal package rather than duplicating code.

## When Unsure, Ask (Do Not Guess)

If requested behavior is not unambiguous, stop and ask a clarifying question instead of implementing inferred behavior. Examples:

- Behavior changes that could break CLI/MCP compatibility or automation expectations.
- Potentially destructive repository actions where scope is unclear.
- Conflicting interpretations of command semantics, flags, or error-code behavior.

## Architecture

### Key Invariants

- Markdown files are the only durable data store. The SQLite index is always rebuildable.
- Object IDs are derived from file paths; section IDs use `#fragment`.
- Schema drives indexing: undefined traits are not indexed; unknown frontmatter keys are validation errors.
- The command registry (`internal/commands/registry.go`) is the single source of truth for command metadata, shared by both CLI and MCP.

### Data Flow

```
Markdown files → Parser → Index (SQLite) → Query Executor
                   ↓
             Schema validation
                   ↓
             CLI / MCP output
```

## Language and Tooling

- **Go 1.25.12+** — all code in standard Go style
- **Build:** `go build -o rvn ./cmd/rvn` or `make build`
- **Formatting:** `gofmt -s` and `goimports` with local prefix `github.com/aidanlsb/raven`
- **Linting:** `golangci-lint v2` (see `.golangci.yml` for enabled linters)
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

- Use structured error codes defined in `internal/codes` (e.g., `codes.ErrTypeNotFound`, `codes.ErrRefAmbiguous`). These codes are stable and agents depend on them.
- All CLI commands support `--json` output with a standard envelope: `{ ok, data, error?, warnings?, meta? }`.
- Warning codes are also defined in `errors.go` (e.g., `WarnBacklinks`, `WarnUnknownField`).

#### Schema/config load-failure policy

A missing `schema.yaml` is never a failure — `schema.Load` returns a default (empty) schema with a nil error, and continuing is always safe. A load *failure* means the file exists but is unreadable or invalid. Handle load failures explicitly per operation; never silently continue when the operation relies on schema:

- **Fatal** when the operation relies on schema for safety or correctness — reference-resolving mutations (`delete`, `edit`, `move`, and other mutations that need validation/ref checks). These return `SCHEMA_INVALID` (or `CONFIG_INVALID` for `raven.yaml`) and must not proceed with a corrupt schema.
- **Warning with degraded behavior** when continuing is intentional and safe (read/render paths). Emit the stable `SCHEMA_LOAD_FAILED` warning code rather than swallowing the error. `readsvc.NewRuntime` records the failure on `Runtime.SchemaLoadErr` (when `RuntimeOptions.RequireSchema` is false) so callers can surface it; set `RequireSchema` to make it fatal.
- **Intentionally schema-free** only where truly appropriate, and never silently. The long-running LSP tolerates a broken schema (diagnostics degrade to the built-in schema) but logs the degradation to stderr instead of keeping stale state silently.

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

## High-Risk Areas

Changes in the areas below require extra validation before submitting:

- Parser (`internal/parser/`, markdown/frontmatter handling):
  - Run `make test` and `make test-integration`.
  - Verify schema validation and object ID derivation behavior.
- Index/query engine (`internal/index/`, query execution paths):
  - Run `make test` and `make test-integration`.
  - Check rebuild invariants (`rvn reindex`) and query result stability.
- Path/reference handling (`internal/paths/`, ref resolution):
  - Add/adjust table-driven tests for edge cases.
  - Validate path normalization and reference updates for move/reclassify flows.
- Command surface (`internal/cli/`, flags/outputs/errors):
  - Confirm `--json` envelope and stable error/warning codes.
  - Ensure `internal/commands/registry.go` metadata matches behavior exactly.

## Documentation

Keep documentation in sync with code. There are three main documentation surfaces:

### Command Registry (single source of truth)

`internal/commands/registry.go` defines all command metadata: descriptions, arguments, flags, examples, and agent use cases. This registry drives both the CLI and MCP tool schemas — update it once, and both stay in sync.

When adding or modifying a command:
1. Update the `Registry` map in `internal/commands/registry.go`
2. MCP tools are regenerated automatically from the registry

### MCP Agent Guide

`internal/mcp/agent-guide/` contains markdown files that are embedded at compile time and served as MCP resources (`raven://guide/*`). These provide agent-specific guidance:

| File | Purpose |
|------|---------|
| `index.md` | Navigation and topic discovery |
| `critical-rules.md` | Safety rules agents must follow |
| `quickstart.md` | One-pass mental model and first-command sequence |
| `onboarding.md` | Interactive setup and teaching sequence for first-session vault creation |
| `getting-started.md` | First steps in a new vault |
| `core-concepts.md` | Types, traits, references explained |
| `response-contract.md` | JSON envelope, error codes, warnings, and preview/apply semantics |
| `write-patterns.md` | Choosing safe write primitives (`new`, `add`, `upsert`, `set`, `edit`) |
| `querying.md` | RQL reference and query strategy |
| `query-cheatsheet.md` | Common query patterns |
| `query-at-scale.md` | Pagination and narrowing strategy for large result sets |
| `key-flows.md` | End-to-end operational playbook |
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

- `docs/getting-started/` — first-session flow, core concepts, configuration
- `docs/types-and-traits/` — schema intro, schema reference, file format, templates
- `docs/querying/` — query language reference
- `docs/vault-management/` — bulk operations
- `docs/agents/` — MCP reference

When making significant changes, check if the relevant docs section needs updating (especially `agents/mcp.md` for MCP changes).

## PR Output Contract

When reporting completed work, include:

- Files changed and the behavioral impact per file.
- Commands/tests run and their outcomes.
- Any residual risks, assumptions, or follow-up checks not yet executed.
- Explicit note if docs were updated or intentionally not updated.

## What to Check Before Submitting

1. `make fmt` — code must be formatted
2. `make lint` — no linter errors
3. `make test` — all unit tests pass
4. If parsing/index/query/CLI behavior changed, run checks from **High-Risk Areas**.
5. If command behavior/flags changed, verify `internal/commands/registry.go` and related docs in **Documentation** are updated.
6. If you added a new package, ensure no circular imports.

## Cursor Cloud specific instructions

Build/lint/test commands are documented above (see **Building and Testing**). Notes specific to running in this cloud environment:

- **Go toolchain:** `go.mod` requires `go 1.25.12`. `GOTOOLCHAIN` auto-downloads the pinned toolchain on first `go` invocation, so no manual Go install is needed.
- **Dev tools for lint/fmt:** `make lint` and `make fmt` need `golangci-lint` (pinned `v2.9.0`) and `goimports`, installed via `make tools` into `$(go env GOPATH)/bin` (`/home/ubuntu/go/bin`), which is already on `PATH`. The startup/update script runs `make tools`, so these are available without re-running it.
- **No long-running services:** `rvn` is a single self-contained binary with an embedded pure-Go SQLite index (no external DB/server/port). There is nothing to keep running to test the product — build with `make build`, then invoke `./rvn <command>`.
- **Commands need a vault:** any command that reads/writes vault data requires either `--vault-path /abs/path`, `--vault <name>`, or an active vault (`rvn vault use`). Without one, commands fail with error code `VAULT_NOT_SPECIFIED`. Create a throwaway vault for manual testing with `rvn init /tmp/some-vault`.
- **MCP server:** `rvn serve --vault-path <path>` speaks JSON-RPC 2.0 over stdin/stdout (stdio transport, not a network port). It is optional and only needed to exercise the agent-facing path.
