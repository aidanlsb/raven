# Raven Design Review

An external review of Raven's architecture, data model, and feature design — with comparisons to other note-taking tools (Obsidian, Logseq, Notion, Bear, Apple Notes, Dendron).

---

## Executive Summary

Raven makes a strong, opinionated bet: plain-text markdown as source of truth, a derived SQLite index, schema-driven structure, and AI agents as a first-class interface. This combination is unique in the note-taking space. The architecture is sound, the code is well-organized, and the design principles (explicit over implicit, general-purpose primitives, no duplicated functionality) are consistently applied.

The main areas for improvement are: (1) the onboarding cliff from requiring schema definition upfront, (2) missing real-time collaboration and concurrency primitives, (3) limited visualization and discovery affordances compared to graph-based tools, and (4) the risk of the query language becoming a barrier rather than a power tool.

---

## What Raven Gets Right

### 1. Markdown as Source of Truth

The single most important design decision. Files are portable, diffable, version-controllable, and readable without Raven installed. This is a genuine advantage over Notion (proprietary database), Bear (SQLite-only), and Apple Notes (opaque CoreData). Obsidian shares this philosophy but couples it to a GUI; Raven stays closer to the data.

The derived index pattern (`index.db` is rebuildable, not canonical) is clean. It avoids the dual-source-of-truth problems that plague tools like Logseq, where the database and files can drift.

### 2. Schema-Driven Structure

Raven's `schema.yaml` is a genuine differentiator. No other markdown-based tool enforces typed structure at the file level. Obsidian's Dataview plugin retrofits querying onto unstructured data; Dendron has schemas but they're limited to hierarchy validation. Raven's schemas drive:
- Validation (required fields, enum constraints, ref targets)
- Indexing (only declared traits are indexed)
- Querying (field types inform comparison operators)
- File creation (templates, default paths)

This closed-world approach for traits (only defined traits are indexed) is a smart constraint — it prevents the "tag soup" problem where Obsidian vaults accumulate hundreds of ad-hoc properties.

### 3. The Object/Trait/Reference Triple

The data model is well-decomposed:
- **Objects** represent durable entities with identity and fields
- **Traits** represent ephemeral annotations that belong to content lines
- **References** represent relationships between objects

This is cleaner than Obsidian's flat "everything is a page with properties" model. The distinction between objects (structured, schema-validated) and traits (lightweight, inline) maps well to how people actually take notes — some things deserve their own file, others are just annotations.

### 4. Embedded Objects and Section Hierarchy

The `::type(...)` syntax for embedding typed objects under headings is elegant. It avoids the "one file per thought" proliferation that plagues Notion and early Roam users, while still giving structure within a document. The automatic section-object creation from headings is a good default.

### 5. Agent-First Design via MCP

Exposing all commands as MCP tools is forward-looking. The command registry pattern (single source of truth for CLI and MCP metadata) prevents the API from drifting out of sync with the CLI. The embedded agent guide resources (`raven://guide/*`) are thoughtful — they give AI agents the context they need without requiring documentation lookups.

### 6. Go as Implementation Language

Pure-Go with no cgo (using `modernc.org/sqlite`) means trivial cross-compilation and zero native dependency issues. This is a practical advantage over tools built on Electron (Obsidian, Logseq) or web stacks (Notion). The binary is self-contained.

---

## Areas for Improvement

### 1. Onboarding Friction: The Schema Chicken-and-Egg Problem

**The issue**: Before you can meaningfully use Raven, you need to define a `schema.yaml`. This requires knowing what types you need, which you often don't know until you've been taking notes for a while. Obsidian, Logseq, and Bear all let you start capturing immediately and add structure later.

**How competitors handle this**: Obsidian lets you write anything and retroactively add Dataview queries. Logseq has built-in `TODO`/`DONE` semantics with zero configuration. Notion offers pre-built templates.

**Suggestions**:
- Ship a `rvn init` experience with optional starter schemas ("personal knowledge base", "engineering team", "GTD workflow") so users don't face a blank `schema.yaml`
- Consider a "schemaless mode" where undefined traits are still captured (as untyped strings) and can be formalized later. The current behavior of silently ignoring unregistered traits means captured data is lost from the index
- The `rvn schema add` commands help, but they require knowing what you want upfront

### 2. Query Language Learnability

**The issue**: RQL is powerful but has a steep learning curve. The syntax (`object:project .status==active has(trait:due)`) mixes prefix notation, dot-prefixed fields, and function-call predicates. Compare with Obsidian Dataview's more SQL-like syntax (`FROM "projects" WHERE status = "active"`) or Notion's GUI filters.

**Specific concerns**:
- `has(trait:due)` vs `encloses(trait:due)` — the distinction between direct children and all descendants is important but not intuitive from the names alone
- The structural predicates (`parent(...)`, `ancestor(...)`, `within(...)`, `contains(...)`, `encloses(...)`) form a rich vocabulary but require understanding the object hierarchy model
- Error messages are good (e.g., `.field==* is no longer supported; use exists(.field)`), which helps, but the learning curve is front-loaded

**Suggestions**:
- Saved queries in `raven.yaml` partially address this, but consider shipping default saved queries with common patterns (overdue items, recent notes, unlinked references)
- An interactive `rvn query --explain` mode that shows how a query is parsed and executed would help debugging
- Consider whether the structural predicates need clearer naming. `has()` reads as "this object has this trait" but actually means "this object's direct child sections have this trait." That's a subtle but important distinction

### 3. No Visualization or Graph View

**The issue**: Obsidian's graph view, Logseq's graph, and Roam's network visualization are key discovery tools. They help users find clusters of related notes, identify orphan pages, and serendipitously discover connections. Raven has `backlinks` and `outlinks` commands but no spatial or visual representation of the knowledge graph.

**Why it matters**: The reference graph is one of Raven's core data structures, but it's only queryable textually. For vaults with hundreds of objects, text-based backlink lists don't surface the same patterns that a visual graph does.

**Suggestions**:
- A `rvn graph` command that exports DOT/Graphviz format would be a low-effort way to enable visualization without building a UI
- The planned `rvn web` could include an interactive graph view
- Even a simple `rvn stats` command showing top-connected objects, orphan pages, and reference counts would add discovery value

### 4. Concurrency and Multi-Writer Safety

**The issue**: The codebase acknowledges this in `docs/design/future.md` — there's no isolation between concurrent writers. With the MCP server enabling AI agents to modify files, this becomes a practical concern, not just theoretical. Two agents running `raven_edit` on the same file can silently lose each other's changes.

**How competitors handle this**: Notion has real-time collaboration built in. Obsidian Sync handles conflict resolution with file-level merging. Logseq relies on Git.

**Suggestions**:
- Optimistic locking (check mtime before write) should be priority — it's low-cost and prevents the worst case (silent data loss)
- The MCP server is a natural place to add write serialization since it's already a single process handling requests
- `internal/atomicfile/` already exists for safe writes, but the check-then-write race condition is the real problem

### 5. No Mobile or Capture-Anywhere Story

**The issue**: Every successful note-taking tool has a quick-capture mechanism. Bear has a share extension. Apple Notes has widgets. Obsidian has a mobile app. Notion has a web clipper. Raven's `rvn add` is desktop-CLI-only.

**Why it matters**: Notes are most valuable when capture friction is minimal. If someone has an idea on their phone, they need a path to get it into the vault.

**Suggestions**:
- A lightweight HTTP API mode (distinct from MCP) that accepts POST requests for quick capture would enable Shortcuts/Tasker integration
- Alternatively, document a "capture via Git" workflow (edit daily note on phone via Working Copy or similar, sync via Git)
- The `capture` config in `raven.yaml` is well-designed — it just needs more entry points beyond the CLI

### 6. Limited Content Types

**The issue**: Raven is text-only. No support for images, PDFs, attachments, or embeds. Obsidian handles images and PDFs. Notion embeds anything. Even Logseq supports assets.

**Why it matters**: Knowledge management often involves non-text artifacts — screenshots of whiteboards, PDFs of papers, diagrams.

**Suggestions**:
- An `assets/` convention (store files, reference via relative links) would be trivial to implement
- The index doesn't need to parse binary files, but awareness of them (tracking which objects reference which assets) would be valuable for `rvn check` and cleanup operations

### 7. The Trait Model Limitations

**The issue**: Traits are single-valued and line-scoped. A trait like `@assignee([[people/freya]])` annotates one line. There's no way to annotate a paragraph or a section with a trait, and no multi-valued traits.

**Specific limitations**:
- Can't express "this entire section is @status(in-progress)" — only individual lines
- Can't have `@tag(infrastructure, security)` — would need `@tag(infrastructure) @tag(security)` on the same line
- Traits on bullet items work well; traits on prose paragraphs are awkward

**Comparison**: Logseq's block-level properties apply to the whole block. Notion's database properties apply to the whole page. Raven's line-level traits are more granular but less flexible.

**Suggestions**:
- Section-level traits (applied to all content under a heading) would address the scoping issue without changing the file format dramatically — a trait on a heading line could be interpreted as applying to the section
- The `schema.yaml` already supports array field types (`string[]`), but traits don't have an equivalent. Consider whether multi-valued traits are worth the parsing complexity

### 8. No Undo/History Beyond Git

**The issue**: `rvn delete` moves to `.trash`, which is good. But `rvn set` and `rvn edit` modify files in place with no built-in undo. The assumption is that users have Git, but not all will.

**Comparison**: Obsidian has file recovery and version history. Notion has page history. Apple Notes has undo.

**Suggestions**:
- Even without full version history, a simple "backup before mutate" pattern (copy to `.raven/backups/` before any write) would add safety
- The `atomicfile` package handles write safety but not rollback

### 9. Search Ranking and Relevance

**The issue**: FTS5 provides full-text search, but there's no indication of relevance ranking, recent-first ordering, or title-boosting in the search results. The `fts_query.go` file handles search but the ranking model isn't visible.

**Comparison**: Obsidian's Quick Switcher ranks by recency and name match quality. Notion's search considers page visits and edits. Bear ranks by recency and title match.

**Suggestions**:
- FTS5 supports `rank` — surface it in search results
- Title matches should rank higher than body matches
- Recency bias (recently modified files rank higher) improves search usability significantly

### 10. Workflow System Scope

**The issue**: The workflow system is essentially a prompt template engine — it gathers context via queries and renders a prompt. This is useful but doesn't handle multi-step execution, branching, or error handling. The name "workflow" implies more automation than it delivers.

**Comparison**: Notion has simple automations (if trigger, then action). Logseq has no equivalent. Obsidian has Templater for template execution.

**Suggestions**:
- Clarify in docs that workflows are "context-gathering prompt templates" rather than executable automation
- Consider whether the agent-composable approach (documented in `future.md`) makes dedicated workflow execution unnecessary — if agents can chain `query → read → edit` themselves, the workflow system's main value is as a prompt library

---

## Architectural Observations

### Things That Scale Well

- **Package structure**: Clean internal-only packages with no circular dependencies. Each package has a clear responsibility.
- **Command registry**: Single source of truth for CLI and MCP. Adding a new command automatically makes it available in both interfaces.
- **SQLite as derived cache**: The index is disposable and rebuildable. Schema migrations are handled by dropping and recreating, which is fine for a cache.
- **Parser design**: Goldmark AST-based parsing is solid. The document parser cleanly separates frontmatter, sections, traits, and references.

### Things That May Not Scale

- **Single-file indexing model**: Each file is parsed independently. Cross-file analysis (like detecting orphan references or computing graph metrics) requires a full index scan. As vaults grow to thousands of files, incremental reindexing helps but graph-level queries will get expensive.
- **Reference resolution at query time**: The resolver tries multiple strategies (full path, short name, slug, name field, alias). With many objects, the fallback chain could get slow. Consider precomputing a resolution cache in the index.
- **JSON output envelope**: Every command wraps output in `{ok, data, error, warnings, meta}`. This is good for MCP but means the CLI always pays the serialization cost even for human-readable output. (Minor concern — Go's JSON encoding is fast.)

### Code Quality Notes

- The custom `itoa` in `model/trait.go` to avoid importing `strconv` is unusual. It saves an import but adds 15 lines of code that `strconv.Itoa` handles. This is a micro-optimization that hurts readability.
- The `indexLock` using `syscall.Flock` is Linux/macOS only. The build tags and goreleaser config target Windows, but `syscall.Flock` won't work there. Worth verifying Windows behavior.
- Test coverage gaps noted in `future.md` (query executor, check validator, CLI commands) are honest. The integration test pattern (build binary, run commands) is good but should be expanded.

---

## Comparison Matrix

| Capability | Raven | Obsidian | Logseq | Notion | Dendron |
|---|---|---|---|---|---|
| Plain-text files | Yes | Yes | Yes | No | Yes |
| Schema validation | Yes | No | No | Yes (DB) | Partial |
| Typed objects | Yes | No | No | Yes (DB) | No |
| Inline traits/annotations | Yes | No | Block props | No | No |
| Query language | RQL | Dataview plugin | Advanced queries | Filter UI | None |
| AI/Agent integration | MCP (native) | Plugin | Plugin | Built-in | None |
| Graph visualization | No | Yes | Yes | No | No |
| Mobile app | No | Yes | Yes | Yes | No |
| Real-time collaboration | No | Sync | No | Yes | No |
| Self-hosted | Yes (local) | Yes | Yes | No | Yes |
| Attachments/media | No | Yes | Yes | Yes | Yes |
| Version history | Git (external) | Built-in | Git | Built-in | Git |

---

## Prioritized Recommendations

1. **Onboarding**: Ship starter schemas and consider a "discovery mode" that captures unregistered traits for later formalization
2. **Concurrency**: Add optimistic locking to mutation commands — this is the cheapest protection against the most damaging failure mode
3. **Search quality**: Add FTS5 ranking, title boosting, and recency bias
4. **Quick capture**: Expose a lightweight HTTP endpoint for mobile/shortcut capture
5. **Graph export**: A `rvn graph --format dot` command for external visualization
6. **Saved query defaults**: Ship common saved queries so new users get value from querying immediately
7. **Asset awareness**: Track file references to non-markdown files for completeness checking
8. **Query explain mode**: `rvn query --explain` to help users understand query execution
9. **Section-level traits**: Allow traits on headings to annotate entire sections
10. **Windows file locking**: Verify `syscall.Flock` behavior on Windows or add a platform-appropriate fallback
