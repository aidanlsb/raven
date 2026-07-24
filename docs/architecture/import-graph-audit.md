# Import-graph and layering audit

This is an audit-only snapshot of Raven's production Go package graph, based on
`main` at `3058c675087c`. It recommends follow-up boundaries; it does not propose
changing command behavior or wire contracts as part of this document.

## Method and target shape

The graph was generated with `go list` for `./internal/...` and `./cmd/...`,
then production import edges were traced to the files and symbols that create
them. Package roles and intended boundaries were checked against `AGENTS.md`,
package comments, and `docs/querying/internals.md`.

The useful dependency direction is:

1. value/domain types and leaf utilities (`model`, `codes`, `dates`, `paths`);
2. schema, configuration, and reference semantics;
3. parsing;
4. index/query contracts and implementations;
5. vault runtime and domain services;
6. command orchestration;
7. CLI, MCP, and LSP adapters.

Cross-cutting workflows can depend on several lower layers. Lower layers should
not depend on workflows, peer services should not be imported just to borrow a
small policy or DTO, and adapters should not reproduce index lifecycle or
consistency rules.

### Baseline

- 68 packages and 372 internal import edges were found.
- The graph is a DAG: there are no import cycles or multi-package strongly
  connected components.
- `cmd/rvn` imports only `internal/cli`, which is a healthy composition root.
- The main risk is dense hubs and near-cycles, not an existing cycle.

| Measure | Leading packages |
| --- | --- |
| Direct fan-in | `paths` 27, `codes` 25, `schema` 25, `svcerr` 22, `parser` 19, `config` 18 |
| Direct fan-out | `commandimpl` 42, `cli` 38, `objectsvc` 22, `checksvc` 20, `schemamigrate` 15, `readsvc` 14, `sectionsvc` 13, `lsp` 12 |

High fan-out in `cli` and `commandimpl` is partly expected because they compose
commands. The more actionable concentration is in middle-layer packages:
`readsvc` owns reference resolution in addition to reads and refresh, while
`checksvc` combines a read-only checker with repair workflows that import other
mutation services.

## Ranked findings

Priority is relative to architecture cleanup, not a statement that current
behavior is incorrect.

| Rank | Finding | Why it hurts | Suggested fix | Effort | Priority |
| ---: | --- | --- | --- | :---: | :---: |
| 1 | Vault-aware reference resolution is embedded in `readsvc` | Mutation peers import a broad read service; `fieldmutation` also self-constructs a runtime and opens the index | Extract `internal/refresolve`; inject target-type lookup into field validation | M | P0 |
| 2 | Content-mutation path policy is owned by `objectsvc` | `sectionsvc` depends on an object mutation service only to validate paths | Extract `internal/mutationguard` with stable neutral service errors | S | P1 |
| 3 | `versioninfo` depends on the vault-facing `maintsvc` | A leaf metadata package transitively pulls in 20 internal packages, including runtime and index | Make `versioninfo` own version types/readers; leave only vault stats in `maintsvc` | S | P1 |
| 4 | LSP and CLI build parallel views over `index.Database` | LSP request snapshots still query a live DB; CLI picker code detaches a DB from a partial runtime | Add a coherent read-side catalog/snapshot API and migrate LSP first, then CLI catalog consumers | L | P1 |
| 5 | `checksvc` is both checker and repair workflow | Fan-out 20 and 32 transitive dependencies; read-only users inherit `objectsvc`/`schemasvc` coupling | Split package-level read and repair boundaries | M | P2 |
| 6 | `check` imports `index` for `DuplicateAlias` only | Validation depends on SQLite ownership for a two-field resolver diagnostic | Move the shared alias-collision contract to `resolver` | S | P2 |
| 7 | `reindexsvc` contains raw index-schema SQL | Dry-run projection duplicates knowledge of three index tables outside `index` | Add `index.Database.StatsForFile` and remove the SQL from the service | S | P2 |

## Implementation briefs

### 1. Extract vault-aware reference resolution from `readsvc`

Current edges:

- `objectsvc/reference.go` imports `readsvc` for `ResolveReference`,
  `ResolveResult`, `AmbiguousRefError`, and `RefNotFoundError`.
- `sectionsvc/rename.go` and `sectionsvc/lifecycle.go` call
  `readsvc.ResolveReference`.
- `checksvc/checksvc.go` calls it while resolving a scoped check.
- `fieldmutation/field_mutation.go` stores `*readsvc.Runtime` in
  `RefValidationContext`, constructs a partial runtime when one is absent,
  calls `OpenDB`, checks `index.ErrIndexRebuildRequired`, and finally calls
  `readsvc.ResolveReference`.

The implementation in `readsvc/resolve.go` is not intrinsically a read-command
operation. It is the vault-wide reference primitive: it combines literal-path
resolution, indexed resolver semantics, assets, daily dates, section metadata,
and stable ambiguity/not-found errors. Its current home creates the service
chain:

```text
fieldmutation / objectsvc / sectionsvc / checksvc
    -> readsvc -> index -> parser
```

Smallest clean boundary:

1. Create `internal/refresolve` and move `ResolveResult`, the typed errors, and
   `Resolve`/`ResolveDynamic` there. Accept the existing
   `*vaultruntime.Runtime`; do not introduce another runtime abstraction.
2. Let `readsvc` use that package for read operations. Temporary aliases in
   `readsvc` can keep migration churn small.
3. Replace `fieldmutation.RefValidationContext.Runtime` with an injected narrow
   target-type function or interface. The caller supplies a resolver backed by
   the already-loaded runtime. `fieldmutation` should not open or close the DB.
4. Move the current `resolveReferenceType` file-read/parse operation behind that
   injected contract, removing `fieldmutation -> readsvc` and
   `fieldmutation -> index`.
5. Migrate `objectsvc`, `sectionsvc`, and `checksvc` to `refresolve` and preserve
   the existing `svcerr` codes and suggestions.

This is independent of the post-mutation `ChangeSet` coordinator: it changes
pre-mutation lookup ownership, not post-write indexing.

### 2. Move content-mutation path policy out of `objectsvc`

`objectsvc/protected_paths.go` defines
`ValidateContentMutationFilePath` and `ValidateContentMutationRelPath`. The
logic depends only on `config`, `ignore`, and `paths`; it rejects protected,
excluded, and template paths. Its ownership creates these sideways edges:

- `sectionsvc/rename.go` and `sectionsvc/lifecycle.go` import `objectsvc` only
  for this guard;
- `checksvc/fix.go` imports the guard before calling the actual move workflow.

Move the two functions to `internal/mutationguard`. It can return
`*svcerr.Error` using the current stable `codes.ErrInvalidInput` and
`codes.ErrValidationFailed` values. Keep forwarding wrappers in `objectsvc`
during migration if useful, but have `sectionsvc` call the policy package
directly.

Removing `sectionsvc -> objectsvc` also removes six otherwise unnecessary
transitive packages from the section service's graph (`fieldmutation`,
`frontmatter`, `pages`, `refs`, `template`, and `objectsvc` itself). This
policy extraction does not overlap mutation coordination.

### 3. Reverse `versioninfo -> maintsvc`

`internal/versioninfo/info.go` is a small leaf-facing API, but its only import
is `maintsvc`. `maintsvc/service.go` combines two unrelated responsibilities:
vault database statistics and executable/build metadata. Consequently,
`versioninfo` has 20 transitive internal dependencies, including
`vaultruntime`, `index`, `parser`, and `schema`.

Move these symbols from `maintsvc` to `versioninfo`:

- `VersionInfo`;
- `BuildInfoReader`;
- `CurrentVersionInfo`, `CurrentVersionInfoWithReader`, and
  `CurrentVersionInfoFromExecutable`;
- `DefaultModulePath` and the private build-setting/ldflags helpers.

Keep `internal/buildinfo` as the ldflags value holder. `maintsvc` should retain
only `Stats` and its vault-facing errors. Update
`commandimpl/system.go`, `commandimpl/docs.go`, `mcp/server.go`,
`cli/version.go`, and the version tests to consume `versioninfo`. This turns
version metadata into a true leaf and removes an unrelated service from LSP
initialization's transitive graph.

### 4. Establish a coherent read-side index catalog

There are two versions of this leak:

- `lsp/workspace.go` holds `*readsvc.Runtime` but exposes
  `db() *index.Database`. `rebuildCaches` manually brackets resolver, object,
  and alias reads with `ResolverGeneration`. After `Server.snapshot` makes a
  shallow copy, handlers still issue live DB reads in `definition.go`,
  `hover.go`, and `references.go`.
- `cli/completion_refs.go` reads `rt.DB.AllObjectIDs` directly.
  `cli/interactive_picker.go` calls `AllObjects`, `AllSections`,
  `QueryAssets`, and `AllIndexedFilePaths` through
  `openDatabaseWithConfig`. That helper constructs a partial runtime, opens
  its DB, sets `rt.DB = nil`, and returns the detached handle.

A useful fix is more than one-line wrappers around index methods. Add a
read-side catalog contract, preferably in `readsvc`, that owns DB open policy
and can return a generation-consistent set of requested data:

```text
CatalogOptions
  objects / sections / assets / aliases / resolver / consistent

CatalogSnapshot
  generation plus model/resolver values, with lookup maps where needed
```

For consistent snapshots, read the resolver generation before and after the
selected data and retry on change, centralizing the loop now in the LSP.
Provide a narrow backlink read alongside the catalog rather than exposing the
DB. Then:

1. store the catalog snapshot by value in `lsp.workspace`;
2. use it for completion, hover, and definition, while the narrow backlink API
   serves references;
3. remove `lsp.workspace.db()` and the direct `lsp -> index` edge;
4. migrate shell completion with a minimal objects-only option so startup
   remains schema/config-free and fast;
5. migrate interactive pickers and delete `openDatabaseWithConfig`.

Keep picker formatting in `cli`; the catalog should return domain values, not
UI-specific items. Preserve LSP's explicit degraded behavior when schema
loading fails.

### 5. Split `checksvc` into read and repair packages

The behavior is already separated by function, but the package boundary is
not. `checksvc` has fan-out 20 and 32 transitive internal dependencies.

Read-side files and symbols:

- `checksvc.go`: `Run` and `BuildJSON`;
- `detect.go`: `DetectMissingRefs`;
- `canonical.go`: non-canonical path/ref detection.

Repair-side files and symbols:

- `fix.go`: `CollectFixableIssues`, `ApplyFixes`, and the
  `objectsvc.MoveFile` dependency;
- `interactive.go` and `interactive_apply.go`: missing-page/type/trait repair
  and the `schemasvc` dependency;
- `CreateMissingRefsNonInteractive` and its result types, currently in
  `checksvc.go`.

Keep read-only checking in `internal/checksvc` and move repair orchestration to
`internal/checkfixsvc`. Both should consume domain types from `internal/check`;
avoid making `checkfixsvc` import `checksvc` merely for DTOs.
`commandimpl/check.go` can compose both packages, while
`commandimpl/runtime_helpers.go` depends only on the read-side detector and CLI
interactive code depends only on repair contracts.

The payoff is an enforced import firewall, not a cosmetic file split. The
read-only checker no longer inherits mutation-service dependencies.

### 6. Move the duplicate-alias contract below `check` and `index`

`check/validator.go` imports `index` only for
`[]index.DuplicateAlias`. The type is declared beside
`index.Database.FindDuplicateAliases` in `index/resolver_build.go`, but its
meaning is resolver ambiguity rather than SQLite behavior.

Rename/move the DTO to `resolver.AliasCollision` (or an equivalently neutral
name), return it from `index.Database.FindDuplicateAliases`, and accept it in
`check.Validator`. This removes `check -> index` and its `indexschema`/file-lock
transitive dependencies without moving SQL or participating in the completed
`indexschema` contract work.

### 7. Encapsulate per-file index statistics

`reindexsvc/service.go:fileIndexStats` calls `db.DB().QueryRow` directly against
`objects`, `traits`, and `refs`. It is used by `projectedDryRunStats`; global
counts are already correctly encapsulated by `index.Database.Stats`.

Add `StatsForFile(filePath string) (IndexStats, error)` to
`internal/index/stats.go` and have the reindex dry-run projection call it.
Keep projection policy in `reindexsvc`; only table-shape knowledge moves back
to `index`. This is separate from the `indexschema` query/executor contract.

## Explicit exclusions and non-findings

- `model -> schema` through `schema.FieldValue` is real today but is already in
  flight. It is intentionally not ranked. After that work lands, verify that
  `model` no longer imports schema definitions and that formatting helpers did
  not recreate the edge.
- Mutation `ChangeSet` and post-mutation coordination are in flight. This audit
  does not propose moving `commandimpl.autoReindexWarnings` or redesigning
  write completion.
- `internal/indexschema`, the command registry split, `vaultruntime` migration,
  `svcerr` unification, CLI presentation helpers, and test-file splits are
  treated as completed/recent work rather than new findings.
- `index -> parser` is the intended parse-then-index direction.
  `parser.ParsedDocument` is a compact domain-shaped parse result; extracting
  it solely to remove that import would add indirection without correcting a
  wrong direction.
- `query` is large, but `docs/querying/internals.md` deliberately colocates
  syntax, validation, and execution, and `indexschema` now isolates the
  storage contract. A syntax/executor package split would be high churn and is
  not ranked until independent change pressure justifies it.
- `reindexsvc` opening an exclusive rebuild session outside ordinary
  `Runtime.OpenDB` is intentional: rebuild has lock, dry-run, and incomplete
  rebuild semantics that normal reads must not expose.
- No production edge was found from foundational packages such as `model`,
  `paths`, `parser`, `index`, or `query` into `cli`, `mcp`, or `commandimpl`.
  The cleanup target is therefore peer-service coupling and adapter leakage,
  not a hidden low-to-high import cycle.
