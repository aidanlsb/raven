# RQL Internals

This note is for maintainers changing Raven Query Language (RQL) behavior. For user-facing syntax, see `querying/query-language.md`.

RQL runs through four main layers:

1. Parse the query string into a small AST in `internal/query`.
2. Validate the AST against `schema.yaml` when schema information is available.
3. Resolve references that need index context, such as shorthand object IDs and date-note aliases.
4. Compile predicates into parameterized SQL over the SQLite index and map rows back to model objects.

The index is a derived cache. Query code should never treat SQLite rows as durable source data; Markdown files and schema definitions remain the source of truth.

## Query Roots

Every parsed query has exactly one root kind:

| Root | AST shape | Primary index rows |
|------|-----------|--------------------|
| `type:<name>` | object query with `TypeName` set to the schema type | `objects` |
| `trait:<name>` | trait query with `TypeName` set to the trait name | `traits` |
| `section` | section query with no type name | `sections` |
| `asset` | asset query with no type name | `assets` |

The root kind determines which predicates are legal, which SQL builder runs, and what the command response contains. Avoid adding predicates that silently change the result kind; compose them through nested queries instead.

## Layer Boundaries

The parser only understands syntax. It should build AST nodes, preserve literal values, and produce clear parse errors. It should not decide whether a type, trait, field, or reference exists.

The validator owns schema-aware checks. It rejects unknown object types, traits, fields, and invalid predicate/root combinations before execution. When adding a predicate that depends on field type, put those checks in the validator rather than the parser.

The executor owns index-aware behavior. It resolves query-time references with the resolver, snapshots `today`/`now` for one execution, prepares SQL, and scans rows. Execution errors should be reserved for problems that require index state, such as ambiguous reference resolution.

The command and service layers own ergonomics around saved queries, pagination, `--ids`, `--count-only`, `--apply`, refresh, and JSON output. Keep RQL semantics in `internal/query` unless the behavior is specifically about CLI/MCP command flow.

## Predicate Implementation

Most predicates need changes in several places:

| Concern | Typical location |
|---------|------------------|
| AST node | `internal/query/ast.go` |
| Lexer/parser support | `internal/query/lexer.go`, `internal/query/parser*.go` |
| Schema validation | `internal/query/validator.go` |
| SQL generation | `internal/query/sql_predicates_*.go` |
| Execution coverage | `internal/query/*_test.go` |
| User docs | `docs/querying/query-language.md` |

Keep SQL builders parameterized. Do not interpolate user query values into SQL strings; return SQL fragments plus argument slices.

## Reference Semantics

Reference-like syntax can mean either a literal target or a nested query result set:

```text
refs([[project/raven]])
refs(type:project .status==active)
trait:todo within([[project/raven]])
```

Direct targets are resolved during execution, because ambiguity depends on indexed objects, assets, schema, and the configured daily directory. Missing targets intentionally match nothing instead of creating implicit objects.

For schema `ref` and `ref[]` fields, field comparisons use reference-aware semantics. Preserve explicit ambiguity errors for shorthand values that could match multiple objects.

## Testing Guidance

Add tests at the lowest layer that owns the behavior:

- Parser tests for syntax shape and parse errors.
- Validator tests for schema and root/predicate legality.
- Executor tests for SQL-backed semantics, reference resolution, dates, array fields, and nested predicates.
- CLI integration tests only when command flags, JSON shape, saved-query behavior, or apply/preview behavior changes.

Prefer focused table-driven cases over broad fixtures. Query behavior has many interacting dimensions, so tests should name the root kind, predicate shape, and expected edge case explicitly.

## Change Checklist

Before changing RQL behavior, check:

- Does the parser preserve enough information for validation and execution?
- Does validation reject unsupported root/predicate combinations before SQL generation?
- Are reference, date, and array semantics covered by executor tests?
- Does the public query-language doc need a syntax or example update?
- If command behavior changed, did `internal/commands/registry.go` and MCP-facing docs stay in sync?
