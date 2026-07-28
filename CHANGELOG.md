# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Added a lightweight link-edge index for Markdown links and images targeting external files and URLs, with source positions, conservative target normalization, and no duplication of Raven object or section references.
- `rvn check` now reports indexed file links missing on disk as `broken_file_link` without fetching URLs, and `rvn move` rewrites normalized-key-matched inbound file links while preserving their authored destination style.
- Added `links(...)` predicates for type, trait, and section queries, using the shared link-field grammar to filter outgoing non-Raven file and URL links.
- Added the `link` RQL root for querying indexed outgoing link/image edges by target metadata and source type/section scope.

### Fixed
- Fixed nested section containment queries returning no matches when a `section` query used `contains(section ...)`.
- Corrected MCP docs, embedded agent guides, and packaged skills for explicit post-`init` vault targeting, canonical hyphenated flag names, bulk argument arrays and retry details, and the body-only `add` contract.
- Corrected user docs for import mapping-file keys, core-type template placement, bulk reclassification, stale active-vault failures, and bare daily-note IDs.
- Clarified portable Markdown file-link rendering and conservative URL/file `normalized_key` behavior in user and agent documentation.
- `rvn check` now reports `markdown_link_to_vault_note` when inline Markdown link/image syntax targets an in-vault `.md` note that must use a wikilink to participate in the reference graph.

### Removed
- **Breaking:** Removed the first-class asset entity, including the `asset` query root, `rvn asset import`, file identities in the Raven reference resolver, asset check issue types, and the asset index schema. Use the link model for file links and copy non-Markdown files into the vault directly; reindex to upgrade, with nothing to migrate.

## [v0.0.32] - 2026-07-25

### Fixed
- MCP `initialize` now reports the GoReleaser-injected release version, matching `rvn version`.

## [v0.0.31] - 2026-07-24

### Added
- `rvn asset import <source> <destination>` copies (or `--move`s) an external non-Markdown file into the vault asset root, with `--force`, `--dry-run`, collision failure by default, and shared ChangeSet/post-mutation indexing.
- `rvn vault focus` sets or clears an MCP server's in-memory session vault pin, so agents can switch the default vault for subsequent calls without changing CLI `active_vault` or config `default_vault`.
- Bulk `rvn reclassify` supports `--stdin` preview and `--confirm` apply.
- Failed bulk mutation responses now retain the attempted input IDs (ordered) so agents can retry without losing the selection.
- LSP quick-fix code actions for reference diagnostics, and go-to-definition / navigation for frontmatter `[[refs]]` / ref field values.
- Incremental reindex can run under a shared index lock alongside the LSP (full rebuilds still take the exclusive lock).
- Index dirty-state journal tracks interrupted post-write projection work and heals on startup.
- Shared `mutation.ChangeSet` / post-mutation path, `internal/fieldvalue`, `internal/refresolve`, read-side catalog snapshots, and related import-graph cleanups for a clearer service layer.

### Changed
- **Breaking:** CLI/MCP target arguments standardize on `reference` / `references` (drop `object` / `object_id` from the surface). `trait_id` and move `source`/`destination` are unchanged. No aliases.
- **Breaking:** `rvn config set` now accepts one or more dotted `key=value` arguments, and `rvn config unset` accepts the same dotted keys as positional arguments. Setting `default_vault` is exclusive to `rvn vault pin`; it can still be cleared with `rvn config unset default_vault`.
- `rvn config show` now reports the full effective global configuration, including resolved defaults.
- Vault-scoped MCP operations always require a per-call `vault`/`vault_path`, a session focus, or a server launch pin. MCP no longer falls back to active/default vault state.
- CLI vault resolution fails when `active_vault` names an unconfigured vault instead of warning and falling back to `default_vault`.
- Old global configs with only the removed single-path `vault` key are migrated in memory to `vaults.default`; the next config write persists the canonical shape.
- `state.toml` always lives beside `config.toml`; stale `state_file` keys are ignored and dropped on the next save.

### Removed
- **Breaking:** the global `ui.accent` and `ui.code_theme` settings, Raven's built-in Markdown style, and their CLI flags and rendering hooks. Use a stock Glamour built-in style or a Glamour style JSON via `ui.markdown_style`.
- **Breaking:** the MCP `strict_vault` setting and `rvn serve --strict-vault` flag (explicit vault targeting is always enforced).
- **Breaking:** the top-level single-path `vault` config field and its synthetic resolution behavior. Use `[vaults]` plus `default_vault` via `rvn vault` commands.
- **Breaking:** the global `state_file` setting and user-facing `--state` overrides.

## [v0.0.30] - 2026-07-21

### Added
- `rvn section rename <file#section> "<new heading text>"` renames a heading in place and rewrites inbound `#slug` references. Heading level is preserved.
- `rvn section create <file> "<title>" --level N` creates headings at EOF or at explicit `--after` / `--before` / `--under` structural anchors, with dry-run previews, strict depth checks, and slug-stability validation. Title is plain text; `--level` is required.
- `rvn section move <file#section>` reorders or reparents a section's complete subtree without changing heading text, level, slug, or identity. `--after` uses the anchor's full subtree boundary; depth mismatches are hard errors (no promote/demote).
- `rvn delete` now accepts asset IDs (single and bulk), with the same dry-run, backlink warning, and trash recovery behavior as object deletes. Sections remain rejected.
- Docs commands (`rvn docs`, `docs list`, `docs search`) lazily refresh the global docs cache when the installed CLI is newer than the cached docs version, fetching the tag that matches the running binary. Offline/fetch failures warn and keep serving the existing cache.

### Changed
- `rvn add` is body-content only: section-targeted appends still use the direct-body boundary before child headings, while section lifecycle placement uses complete subtree boundaries. Markdown heading content is rejected. A configured `capture.heading` may still target an existing heading, but add never creates a missing one.
- Starter `@todo` is an enum with `[todo, done]` (matching docs), and schema version docs now say `version: 1` (matching implementation).
- Release toolchain upgrades Go to 1.25.12 and `golang.org/x/net` to v0.57.0, and CI/release preflight now run `govulncheck`.
- Custom-wired CLI leaves (`add`, `delete`, `import`, `new`, `move`, `reclassify`, `skill`, `upsert`) bind flags from registry metadata, with generalized CLI/registry flag-parity coverage.

### Fixed
- References written before their target existed are now healed automatically. Creating the target (`rvn new`, `rvn upsert`, etc.) re-resolves pending refs and ref fields across the vault, and `rvn reindex` runs its resolution pass even when no files need reindexing. Previously such refs stayed `missing` indefinitely unless a `rvn reindex --full` ran, which made objects invisible to canonical-ID queries (e.g. `.project==project/raven`) while still matching raw-literal queries (`.project==raven`).

### Removed
- **Breaking:** `rvn add --heading` and `rvn add --create-heading` have been removed. Create headings with `rvn section create`, then append body content with `rvn add --to file#section`.
- **Breaking:** `rvn move` no longer accepts section sources (`file#slug`). Use `rvn section rename` for heading renames and `rvn section move` for reorder/reparent.
- Dead `toolexec` / `toolargs` packages and unused CLI result DTOs were deleted.

## [v0.0.29] - 2026-07-20

### Added
- `rvn schema convert trait|field` migrates a trait or field to a new type and/or value set with an exhaustive `--map-json` mapping. Preview by default; `--confirm` applies schema.yaml plus matching annotations/frontmatter. Same-type remaps omit `--type`. Array-to-array maps member-wise; collection-to-scalar is rejected. `schema update --type/--values` remains schema-only.
- Every mutating command now reports a uniform `meta.mutation.phase` of `applied` or `preview` in its response envelope, so agents can tell whether a write actually happened without inferring it from heterogeneous `data` fields (`data.status`, `data.preview`, `data.needs_confirm`). The signal is consistent across content, schema, template, saved-query, `check fix`, and skill-install commands and across the CLI and MCP surfaces; it is present on successful mutations (including preview-only and confirmation-blocked results) and omitted on read-only commands and failures. `query` with `apply` carries the phase of the write it delegates to.
- `rvn skill install` installs shipped Raven skills in one command — the full catalog by default, or a narrowed set when skill names are given — so first-run setup no longer requires chaining named `rvn skill sync ... --confirm` calls. In an interactive terminal it prints the plan and prompts `Install these skills? [y/N]` before writing. In non-interactive or `--json` runs it does not prompt: pass `--yes` to apply (with `--confirm` accepted as an alias), otherwise it returns a preview with a top-level `needs_confirm` flag and per-skill plan so agents know a confirm is still required.
- Initializing an additional vault with `rvn init` now auto-activates it and discloses the previous active vault plus the `rvn vault use` command to switch back (CLI and agents).

### Changed
- **Breaking:** successful response envelopes now use `data.items` for their primary homogeneous collection across query/search, docs search, ambiguous `resolve`, imports, bulk preview/apply results, link traversal, and `raven_discover`. Empty primary collections serialize as `[]` instead of `null`.
- **Breaking for shell automation:** `rvn --json` commands that emit an `ok=false` envelope now exit with status 1, including startup failures, instead of reporting process success. Successful commands continue to exit 0.
- Daily notes now have a bare ISO date (`YYYY-MM-DD`) as their canonical object ID, regardless of `directories.daily`. Previously the configured daily directory was woven into the ID (e.g. `daily/2026-03-15`). `directories.daily` is now filesystem layout only: files still live under it (`daily/2026-03-15.md`, `journal/2026-03-15.md`, …), but the link/object identity is the bare date, so `[[2026-03-15]]` is unambiguous date identity across every vault layout. Legacy daily-directory-prefixed references — `[[daily/2026-03-15]]` and `[[<configured-daily-dir>/2026-03-15]]`, including section forms like `[[daily/2026-03-15#standup]]` — still resolve to the same bare-date object as compatibility aliases. Reindexing an existing vault rewrites indexed daily IDs to the bare form automatically.
- Permissive writes that introduce a reference to a missing target now emit the distinct `REF_TARGET_MISSING` warning code instead of reusing `REF_NOT_FOUND`. `REF_NOT_FOUND` remains the fatal error code for read/resolve failures, so agents can branch on the code alone. Each warning now also carries a structured `create_invoke` (`{command, args}`) alongside the existing `create_command` string, so agents can remediate via `raven_invoke` without shell-parsing.
- Agent skill commands now use the portable Agent Skills directories (`~/.agents/skills` for user scope and `.agents/skills` for project scope). The runtime-specific `--target` flag has been removed; use `--dest` for a custom install root.
- Agent setup documentation now leads with skill installation and describes all seven packaged Raven skills.
- Documentation and the LSP no longer advertise a frontmatter `id` key as an "object ID override". Object identity is path-derived and cannot be overridden in frontmatter; use `alias` to give an object an alternate name for reference resolution. `id` is no longer a reserved frontmatter key, so a stray `id:` value is now reported as `unknown_frontmatter_key` by `rvn check` instead of being silently accepted.

### Removed
- **Breaking:** the singular `--field-json` CLI flag and `field-json` MCP argument have been removed from `new`, `upsert`, and `reclassify`; use `--fields-json` / `fields-json` consistently on all field-writing commands. The former `data.results` and successful `data.matches` collection keys have also been removed; use `data.items`.
- `rvn check` no longer accepts the `--fix`, `--create-missing`, or `--confirm` flags. Repairs live solely on the subcommands `rvn check fix` and `rvn check create-missing` (and the equivalent `check_fix` / `check create-missing` MCP tools), removing the duplicate entry points and the ambiguity of parent-level `--confirm`. `rvn check` is now a read-only validation command.

## [v0.0.28] - 2026-07-08

### Added
- `rvn lsp` runs Raven as a Language Server Protocol server over stdio for editor integration: diagnostics (matching `rvn check` issue types), completion for `[[refs]]`, `@traits`, and frontmatter keys, go-to-definition, find-references, and hover. See the new "Editor Integration (LSP)" docs page for Neovim setup.
- `rvn backlinks` results include `position_start`/`position_end` column offsets when available.

### Changed
- The interactive picker now supports cursor movement while filtering (arrow keys, ctrl-n/ctrl-p, page up/down in insert mode).
- Picker rows are no longer separated by divider lines, roughly doubling the number of visible results.
- Picker table columns are sized from the full result set once, so they stay stable while filtering, and filtering large result sets is faster (search text is normalized once and extending a query narrows from the current matches).
- Picker table cells truncate by display width, keeping columns aligned with CJK and emoji content.
- A saved query with a `browse` default now degrades to normal output when run without an interactive terminal (piped output, scripts) instead of erroring; explicit `--browse` still requires a terminal.
- `rvn pick` exits with code 130 when the picker is cancelled, so pipelines can distinguish cancellation from an empty selection.

## [v0.0.27] - 2026-07-02

### Changed
- Single-object write commands now apply immediately by default while retaining explicit dry-run support.
- Picker item semantics and Cursor Cloud agent guidance are clearer.

### Fixed
- `rvn check create-missing` now infers date targets correctly.
- Wikilink targets preserve backticks.
- `rvn schema type` includes allowed enum values in human-readable output.
- `rvn add --heading` accepts visible single-word heading text.
- Query parse failures provide actionable syntax suggestions and complete query-root guidance.

## [v0.0.26] - 2026-06-19

### Added
- Writes stay permissive but now surface missing reference targets: `rvn new`, `upsert`, `set`, `add`, and `edit` succeed when a `ref` field or body `[[ref]]` points at a target that does not exist yet, returning a `REF_NOT_FOUND` warning plus `missing_refs`/`missing_ref_items`. The interactive CLI offers to create the missing pages, and `rvn check create-missing` remains the batch remediation path.
- `rvn backlinks` and `rvn outlinks` accept `--stdin` to traverse multiple targets/sources and return grouped results.

### Changed
- `rvn check` reports directory type mismatches for objects stored outside their configured directory.
- Type templates are enforced as body-only content.
- Interactive picker command adapters are consolidated and reference picker matching is improved.

### Fixed
- Resolved stale query index errors.
- Fixed interactive picker typing responsiveness and height handling.

## [v0.0.25] - 2026-06-14

### Added
- Interactive picker rows now support preview overlays, including `rvn pick --preview`.

### Fixed
- Picker styling now degrades cleanly when stdout is redirected.

## [v0.0.24] - 2026-06-14

### Added
- Raven's interactive picker now powers docs navigation, query browsing, reference disambiguation, backlinks/outlinks browsing, and pipe-friendly selection workflows.
- `rvn pick` provides a Raven-native selector for pipe-based workflows, including multi-select output for downstream commands.
- `rvn backlinks` and `rvn outlinks` support `--browse` to select and open a reference location.
- `ui.markdown_style` configures full Glamour Markdown rendering styles, with automatic light/dark styling as the default fallback.

### Changed
- Query browse and non-browse result tables now share dynamic, content-aware column sizing and row presentation.
- The Raven picker has modal filtering, Vim-style movement, forward/back navigation, shortcut gutters, multi-select support, and clearer selected-row styling.
- `rvn docs`, bare reference pickers, and ambiguous-reference flows no longer depend on fzf.

## [v0.0.23] - 2026-06-07

### Added
- Packaged `raven-onboarding` skill for guided first-session vault setup and Raven concepts walkthroughs.
- `rvn template write --edit` opens template content in the configured editor before writing it back through Raven's template validation flow.

## [v0.0.22] - 2026-06-07

### Added
- RQL now has first-class `asset` and `section` query roots, including section-aware scope predicates and inverse `refd(section)` matching.
- Trait value queries now support array-valued traits.
- `rvn unset` removes frontmatter fields from file-backed objects.
- Documentation search supports pagination.

### Changed
- Inline typed-object declarations have been removed in favor of file-backed typed objects plus heading-derived sections.
- The query vocabulary now uses `oneof(...)` for scalar membership and `includes(...)` for string containment, while scope predicates use `in(...)`, `within(...)`, `has(...)`, and `contains(...)`.
- Asset configuration is directory-only, and retrieval table rendering is centralized across output paths.
- Packaged Raven skills now provide clearer vault admin, maintenance, template, and schema migration guidance.

## [v0.0.21] - 2026-06-04

### Changed
- Interactive fzf pickers (`rvn read`, `rvn open`, `rvn docs`, ambiguous-reference selection) no longer hardcode their appearance. Raven's cosmetic defaults (`--layout=reverse --height=80% --border`) are now applied through `FZF_DEFAULT_OPTS`, so any `FZF_DEFAULT_OPTS` you set overrides them. Behavioral flags (`--select-1`, `--exit-0`) are preserved.

## [v0.0.20] - 2026-05-27

### Added
- Optional fzf-driven interactive selection for bare `rvn search` and ambiguous `rvn open` references, with deterministic non-interactive fallbacks.

## [v0.0.19] - 2026-05-17

### Added
- First-class asset resources for vault-local non-Markdown files, including `directories.assets` configuration, indexing, reference resolution, backlinks/outlinks, and RQL `refs(...)` graph support.
- Markdown links/images and Raven wikilinks can now reference assets, including unambiguous short asset references such as `[[paper]]`.
- `rvn check` now reports missing and orphaned assets; `rvn move` can relocate assets while updating Markdown links/images and wikilinks.

### Changed
- CLI and MCP documentation now describe assets consistently across core concepts, configuration, references, querying, common commands, and agent guidance.
- Stable response code definitions are centralized across CLI/MCP command execution paths.
- MCP command contracts now mark bulk-preview commands consistently.

## [v0.0.18] - 2026-05-04

### Changed
- `rvn skill install` has been replaced by `rvn skill sync`, including registry metadata and examples for syncing packaged Raven skills.
- Dependency update CI now groups Charmbracelet-related updates together.

### Fixed
- MCP validation errors now include argument schemas, making malformed `raven_invoke` calls easier to correct.
- MCP agent guidance now more clearly describes immediate delete semantics.

## [v0.0.17] - 2026-05-03

### Added
- `check` and `check fix` now detect and repair non-canonical layouts. New `non_canonical_path` (error) issues flag files that live outside the configured directory root for their type, and auto-fix moves them via the move service so all references stay in sync. New `non_canonical_ref` (warning) issues flag wikilinks that unnecessarily include the configured root prefix (e.g. `[[type/person/jane]]`); auto-fix rewrites them to canonical form only when the stripped target still resolves to the same object.
- `raven_describe` now exposes a `description` field alongside `summary`, surfacing the registry's long-form command guidance (such as RQL syntax examples for `query`) so agents can avoid an extra round trip for command-specific syntax.

### Changed
- The packaged Raven skills (`raven-core`, `raven-query`, `raven-maintenance`, `raven-schema`, `raven-templates`, `raven-vault-admin`) are now explicitly CLI-focused. "Prefer MCP when in-session" framing has been replaced with a short scoping note so skills consistently teach `rvn ... --json`. The `rvn docs *` pointer has been moved out of `raven-maintenance` and into `raven-core` as a small "look things up" section.
- `check fix` now splits text fixes and move fixes, and bulk moves continue past per-item failures so a single collision no longer aborts the batch.

### Fixed
- `rvn search` and RQL `content(...)` now handle identifiers with dots like `inputs.project` by quoting dotted literal tokens before passing search text to SQLite FTS, instead of failing with FTS syntax errors.
- Schema commands now accept array-form enum values from JSON callers (such as MCP `raven_invoke` payloads), matching the documented schema-edit shape.
- `TestOpenInEditorFallsBackToEditorEnv` now writes a `.cmd` batch script on Windows instead of a POSIX `.sh`, restoring the Windows CI test job.

## [v0.0.16] - 2026-04-14

### Changed
- Raven Query Language now uses `type:<name>` as the only type-root syntax. Legacy `object:<name>` roots, including nested subqueries and saved-query examples, are rejected with a targeted hint to switch to `type:`.
- Query results now report `query_kind: type|trait` instead of `query_type: object|trait`, and first-party CLI/MCP/docs guidance now consistently teaches `type:` plus item/type/trait terminology.
- Vault directory config now uses `directories.type` instead of `directories.object`; `rvn init`, vault-config output, and first-party docs now default to `type/`, and legacy `object` keys are rejected explicitly.

### Fixed
- Query trait date filters and object-field reference predicates now evaluate more reliably, including ambiguous ref-field checks across nested/grouped predicates and clearer errors for unsupported `_` self-reference usage.

## [v0.0.15] - 2026-04-10

### Added
- Support vault overrides when reading MCP resources, so resource fetches can target the same vault as command calls.
- Structured vault configuration commands for inspecting and updating config without hand-editing `config.toml`.

### Changed
- CLI and MCP execution paths now share more canonical planning and validation around query apply, object creation, schema field checks, and vault resolution.
- Query/date/reference internals now centralize more shared logic, reducing duplicated parsing and repeated resolver/index work.

### Fixed
- `rvn docs fetch` and `rvn docs list` no longer fail after successful execution in human-readable mode.
- Query pipe parse errors now point users toward shell pipelines and `jq` instead of surfacing a cryptic unexpected-token message.
- Schema updates reject null and unsupported field/trait definitions more consistently during validation.
- Reindex, read, and cleanup paths now preserve cancellation, transactional cleanup, and accurate rebuild bookkeeping more reliably.
- Several command/runtime error paths now preserve clearer, more stable failure codes and messages across add/read/reclassify flows.

## [v0.0.14] - 2026-04-04

### Added
- Dedicated saved-query management commands: `query saved list`, `query saved get`, `query saved set`, and `query saved remove`, along with support for running saved queries directly through `query`.
- `rvn mcp install`, `rvn mcp show`, and `rvn mcp remove` now support Codex client configuration alongside Claude Code, Claude Desktop, and Cursor.

### Changed
- `create` and `reclassify` now share the same typed field pipeline, so frontmatter writes preserve exact value semantics across object mutations. The extra create-then-rewrite pass is removed and `reclassify` gains a matching typed JSON field path.
- `raven_discover` now always returns the full command catalog; passing filter arguments (category/mode/risk) is an error. The command list is short enough that filtering added confusion without benefit.
- Query guidance, examples, and MCP error adaptation now steer ambiguous requests toward concrete follow-up queries more consistently.

### Fixed
- Query `list` contract and several `add`/`resolve` regressions introduced after the v0.0.13 refactor.
- Query issue regressions: error propagation, regexp handling, predicate evaluation for object fields, and split-query execution are all restored to correct behaviour.

## [v0.0.13] - 2026-03-31

### Removed
- Removed the workflows feature, including its dedicated runtime, command surface, default config, MCP resources, and user-facing documentation, to keep Raven focused on core knowledge-management primitives.

### Changed
- Restored the generic agent playbook as the `key-flows` guide after removing workflow-specific guidance.

### Fixed
- Unexpected reference-resolution failures now surface their underlying cause more clearly and map to a stable internal error code in `new` command responses.

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
- Automatic post-command reindexing
- Reference resolution and backlinks
- Comprehensive documentation

### Security
- Vault-scoped operations (no access outside vault)
- Symlink traversal protection
- Path validation for all file operations

### Fixed
- Release workflow tag annotation validation for tag-push events.

[Unreleased]: https://github.com/aidanlsb/raven/compare/v0.0.32...HEAD
[v0.0.32]: https://github.com/aidanlsb/raven/compare/v0.0.31...v0.0.32
[v0.0.31]: https://github.com/aidanlsb/raven/compare/v0.0.30...v0.0.31
[v0.0.30]: https://github.com/aidanlsb/raven/compare/v0.0.29...v0.0.30
[v0.0.29]: https://github.com/aidanlsb/raven/compare/v0.0.28...v0.0.29
[v0.0.28]: https://github.com/aidanlsb/raven/compare/v0.0.27...v0.0.28
[v0.0.27]: https://github.com/aidanlsb/raven/compare/v0.0.26...v0.0.27
[v0.0.26]: https://github.com/aidanlsb/raven/compare/v0.0.25...v0.0.26
[v0.0.25]: https://github.com/aidanlsb/raven/compare/v0.0.24...v0.0.25
[v0.0.24]: https://github.com/aidanlsb/raven/compare/v0.0.23...v0.0.24
[v0.0.23]: https://github.com/aidanlsb/raven/compare/v0.0.22...v0.0.23
[v0.0.22]: https://github.com/aidanlsb/raven/compare/v0.0.21...v0.0.22
[v0.0.21]: https://github.com/aidanlsb/raven/compare/v0.0.20...v0.0.21
[v0.0.20]: https://github.com/aidanlsb/raven/compare/v0.0.19...v0.0.20
[v0.0.19]: https://github.com/aidanlsb/raven/compare/v0.0.18...v0.0.19
[v0.0.18]: https://github.com/aidanlsb/raven/compare/v0.0.17...v0.0.18
[v0.0.17]: https://github.com/aidanlsb/raven/compare/v0.0.16...v0.0.17
[v0.0.16]: https://github.com/aidanlsb/raven/compare/v0.0.15...v0.0.16
[v0.0.15]: https://github.com/aidanlsb/raven/compare/v0.0.14...v0.0.15
[v0.0.14]: https://github.com/aidanlsb/raven/compare/v0.0.13...v0.0.14
[v0.0.13]: https://github.com/aidanlsb/raven/compare/v0.0.12...v0.0.13
[v0.0.12]: https://github.com/aidanlsb/raven/compare/v0.0.11...v0.0.12
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
