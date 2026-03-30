# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.0.12] - 2026-03-29

### Fixed
- `move` now rolls file changes back when a post-move backlink rewrite or strict index update fails, instead of leaving the vault in a partially updated state.
- Move and reclassify rollback regression tests now use deterministic cross-platform failure injection, which restores Windows CI coverage for these cases.

## [v0.0.11] - 2026-03-22

### Added
- Canonical `commandexec`, `commandimpl`, and `bulkops` layers for registry-driven command execution across Raven surfaces.

### Changed
- CLI, MCP, and workflow command execution now share one canonical runtime and handler registry, including schema/template and workflow command families.
- `query --apply` now plans targets and delegates to canonical mutation commands instead of using query-local mutation paths.
- MCP direct tool compatibility aliases now resolve through shared command lookup rather than MCP-local dispatch metadata.

### Removed
- Removed the legacy MCP semantic/direct-dispatch layer and its per-command direct handler implementations.

## [v0.0.10] - 2026-03-20

### Changed
- MCP install/show client resolution now preserves explicit vault selection more reliably across CLI and generated client configs.

### Fixed
- Query `refs(...)` matching now tolerates rooted and unrooted object ID variants, which fixes missed results for project-linked refs when the index contains mixed forms.
- Object queries using `has(trait:...)` now apply the full nested trait predicate instead of only `.value` filters.

## [v0.0.9] - 2026-03-18

### Changed
- MCP vault resolution now uses the native config service directly instead of shelling out to the CLI, preserving `--vault-path`, `--vault`, and active/default vault selection semantics.
- Bundled Raven skills now acknowledge Raven MCP tool equivalents when already operating through MCP, instead of assuming a CLI-only execution path.
- The project release skill now points maintainers at the full release runbook and explicitly calls out changelog, GitHub release, and Homebrew verification steps.

## [v0.0.8] - 2026-03-18

### Added
- Command registry metadata now includes command category/access/risk and lightweight canonical CLI usage strings, which also surface through MCP `raven_describe` as `cli_usage`.

### Changed
- Simplified the CLI and MCP command surface by consolidating template management under `schema template ...`, moving `stats` and `path` under `vault`, and removing obsolete root commands.
- Renamed the internal MCP compact-surface implementation files to `surface.go` / `surface_test.go` now that the compact surface is the only MCP surface.

### Removed
- Removed `last` and all associated stateful tracking machinery, along with legacy `untyped` and `schema commands` surfaces.

## [v0.0.7] - 2026-03-17

### Added
- `raven_describe` now returns an explicit `invoke` contract block (envelope shape, notes, and example) to guide compact-surface invocation.
- Agent guide response contract now includes a compact flow (`discover -> describe -> invoke`) with nested-`args` examples.

### Changed
- `raven_invoke` now requires command arguments strictly under `args`; top-level command arguments are rejected.
- MCP docs and compact tool descriptions now consistently document nested-`args` invocation as the only supported argument shape.

### Fixed
- `raven_invoke` validation errors now include a targeted hint when agents pass command parameters at top level.

## [v0.0.6] - 2026-03-17

### Added
- Shared command policy layer (`invokable`, `discoverable`, `workflow_allowed`) with tests, used by MCP compact discovery/invoke and workflow validation.
- Strict compact MCP surface implementation (`raven_discover`, `raven_describe`, `raven_invoke`) with typed command contracts and schema hash support.

### Changed
- MCP `tools/list` now exposes only the compact 3-tool surface; direct legacy `raven_*` tool calls via `tools/call` are rejected.
- `raven_invoke` now enforces strict typed arguments (removed permissive structured-string coercions), with improved contract-driven errors.
- Workflow tool execution (CLI and MCP workflow runners) now uses shared in-process semantic dispatch instead of subprocess CLI execution.

### Fixed
- MCP server startup logging now correctly reports pinned vault mode when `--vault-path` or `--vault` is provided via base args.
- Added integration coverage for live `rvn serve` JSON-RPC behavior to ensure legacy tool-name calls return `UNKNOWN_TOOL`.

## [v0.0.5] - 2026-03-08

### Fixed
- Release workflow now installs `golangci-lint` with `GOTOOLCHAIN=auto`, fixing failures when the linter requires a newer Go toolchain than the runtime default.

## [v0.0.4] - 2026-03-08

### Added
- New MCP agent-guide topics for response contract, write patterns, workflow lifecycle, and large-vault query strategy.

### Changed
- Consolidated onboarding and teaching flow by removing the standalone lesson-plan guide.
- Restructured key workflow guidance into a concise operational playbook with cross-links to focused topic guides.
- Improved guide accuracy for query examples, issue-type coverage, and error-handling semantics.

## [v0.0.3] - 2026-03-02

### Added
- Release-time changelog validation in both local `make release*` flow and GitHub release workflow.

### Changed
- Release runbook now requires a matching `CHANGELOG.md` entry per version.
- Backfilled missing changelog sections for `v0.0.1` and `v0.0.2`.

## [v0.0.2] - 2026-02-28

### Changed
- Homebrew formula name updated to `rvn` for install consistency.

## [v0.0.1] - 2026-02-28

### Added
- Initial public release
- Core CLI commands: `init`, `reindex`, `check`, `query`, `backlinks`, `stats`
- Schema system with types and traits defined in `schema.yaml`
- SQLite-based index for fast queries
- Query language with object and trait queries
- Full-text search with FTS5
- MCP server for AI agent integration
- Daily notes with templates
- Bulk operations with `--apply` flag
- File watching with auto-reindex
- Reference resolution and backlinks
- Comprehensive documentation

### Security
- Vault-scoped operations (no access outside vault)
- Symlink traversal protection
- Path validation for all file operations

### Fixed
- Release workflow tag annotation validation for tag-push events.

[Unreleased]: https://github.com/aidanlsb/raven/compare/v0.0.11...HEAD
[v0.0.11]: https://github.com/aidanlsb/raven/compare/v0.0.10...v0.0.11
[v0.0.10]: https://github.com/aidanlsb/raven/compare/v0.0.9...v0.0.10
[v0.0.9]: https://github.com/aidanlsb/raven/compare/v0.0.8...v0.0.9
[v0.0.8]: https://github.com/aidanlsb/raven/compare/v0.0.7...v0.0.8
[v0.0.7]: https://github.com/aidanlsb/raven/compare/v0.0.6...v0.0.7
[v0.0.6]: https://github.com/aidanlsb/raven/compare/v0.0.5...v0.0.6
[v0.0.5]: https://github.com/aidanlsb/raven/compare/v0.0.4...v0.0.5
[v0.0.4]: https://github.com/aidanlsb/raven/compare/v0.0.3...v0.0.4
[v0.0.3]: https://github.com/aidanlsb/raven/compare/v0.0.2...v0.0.3
[v0.0.2]: https://github.com/aidanlsb/raven/compare/v0.0.1...v0.0.2
[v0.0.1]: https://github.com/aidanlsb/raven/releases/tag/v0.0.1
