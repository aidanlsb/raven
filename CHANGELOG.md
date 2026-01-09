# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial public release
- Core CLI commands: `init`, `reindex`, `check`, `query`, `backlinks`, `stats`
- Schema system with types and traits defined in `schema.yaml`
- SQLite-based index for fast queries
- Query language with object and trait queries
- Full-text search with FTS5
- MCP server for AI agent integration
- LSP server for editor integration
- Daily notes with templates
- Bulk operations with `--apply` flag
- File watching with auto-reindex
- Reference resolution and backlinks
- Comprehensive documentation

### Security
- Vault-scoped operations (no access outside vault)
- Symlink traversal protection
- Path validation for all file operations

## [0.1.0] - Unreleased

Initial release.

[Unreleased]: https://github.com/aidanlsb/raven/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/aidanlsb/raven/releases/tag/v0.1.0
