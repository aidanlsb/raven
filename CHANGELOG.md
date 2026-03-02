# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/aidanlsb/raven/compare/v0.0.3...HEAD
[v0.0.3]: https://github.com/aidanlsb/raven/compare/v0.0.2...v0.0.3
[v0.0.2]: https://github.com/aidanlsb/raven/compare/v0.0.1...v0.0.2
[v0.0.1]: https://github.com/aidanlsb/raven/releases/tag/v0.0.1
