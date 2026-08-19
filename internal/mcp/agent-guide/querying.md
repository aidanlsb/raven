# Querying

Use this guide when composing Raven Query Language (RQL) expressions.

## Start with structure

If you are unsure what types or traits exist:

```text
raven_invoke(command="schema", args={"subcommand":"types"})
raven_invoke(command="schema", args={"subcommand":"traits"})
```

## Query strategy

1. Start with the most specific query that matches the user's intent.
2. If no results, remove one predicate at a time.
3. Prefer structured predicates over broad text search.
4. Use `search` only when the structure is unknown.

## Choosing the retrieval tool

- `query` — real Raven items, real trait instances, or outgoing link-edge rows; filter by type/section/trait/link, fields, scope, references, and links.
- `search` — you only know a text fragment and do not yet know the type, trait, or structure. Returns file/snippet matches; it does NOT distinguish a real `@todo` trait from prose that mentions `@todo`. For real traits use `query "trait:todo"`.
- `backlinks <reference>` — incoming references to one object or section (structured equivalent: `query "... refd(...)"`; `read` also appends backlinks).
- `outlinks <reference>` — outgoing references from one object (structured equivalent: `query "... refs(...)"`).
- `resolve <reference>` — map an accepted reference input to its canonical object ID without reading content; use that ID in authored references.
- `read <reference>` — full file content once you have identified the object.
- `date <date>` — everything for a date; in RQL use `type:date .date==<date>` for the daily-note object or `trait:due .value==<date>` for items due that day.
- If the user asks for actual open tasks, due items, briefs, or typed items, start with `query`.

## Scope predicates are root-dependent

Traits attach to the nearest section, so lead with the forgiving forms:

- From the object side use `contains(trait:...)` (recursive), not `has(trait:...)` (direct-only): `type:project has(trait:todo)` usually returns nothing; use `type:project contains(trait:todo .value==todo)`.
- From the trait side use `within(type:...)` (recursive), not `in(type:...)` (direct-only).
- `has`/`contains` look downward (on `type:`/`section`); `in`/`within` look upward (on `trait:`/`section`).
- `in(...)` is scope containment, not set membership — for "value is one of a set" use `oneof(.field, [a,b])`.

## Examples

```text
raven_invoke(command="query", args={"query_string":"type:project .status==active"})
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo"})
raven_invoke(command="query", args={"query_string":"type:meeting refs([[project/website]])"})
raven_invoke(command="query", args={"query_string":"link .ext==pdf within(type:project)"})
```

For text search inside typed queries, use `content("term")`.

## Saved queries

Saved queries are named RQL definitions with optional declared inputs and a
description. They do not store runtime execution policy. When invoking one,
pass `refresh`, `ids`, `limit`, `offset`, `count-only`, `apply`, `confirm`,
`pipe`, or `browse` in the current `query` call as needed.

```text
raven_invoke(command="query", args={"query_string":"open-projects","limit":100})
```

Sections use the bare `section` query root and return heading-derived rows with
IDs like `file#slug`. Section rows include `line_start`, direct `line_end`, and
`subtree_line_end`. Body appends with `add` use the direct range so they land
before child headings; structural placement with `section_create` and
`section_move` uses complete subtree boundaries. Section query IDs can be piped
into bulk `add` (`--ids` + `stdin=true`) to append inside each matching section,
and `read` with `sections=true` returns a file's outline directly.

Use `links(...)` on type, section, or trait roots to filter by outgoing non-Raven
links, for example `type:project links(.ext==pdf)` or
`trait:todo links(.is_image==true)`.
For both `refs(...)` and `links(...)`, a type root covers the whole file, a
section root covers the complete section subtree, and a trait root covers only
the trait's source line.

The bare `link` root returns outgoing Markdown link/image edges to non-Raven
file and URL targets. Both surfaces use the same complete field grammar:
`.source_id`, `.source_type`, `.file_path`, `.line`, `.position_start`,
`.position_end`, `.raw_target`, `.display`, `.is_image`, `.scheme`, `.ext`,
and `.normalized_key`. Line and position fields compare numerically. Use
`within(type:...)` or `within(section ...)` for root source scope.
For `.normalized_key`, URLs lowercase the host and strip only default ports
(`:80` for HTTP and `:443` for HTTPS), preserving path case, query, trailing
slash, and fragment. Files inside the vault use vault-relative POSIX keys;
absolute targets outside the vault remain absolute.
Equality on `.normalized_key` and `.raw_target` is case-sensitive; equality on
other string link fields remains case-insensitive. Every link field is present,
so `exists()`/`!exists()` are rejected; use `.ext==""` for an empty extension.
`link --ids` projects `source_id` once per edge. Link rows are outgoing-only
and do not support `refs()`, `refd()`, `in()`, `content()`, arrays, or
`--apply`; there is no `linkd()` inverse because external files and URLs are
leaf targets.
The `link` root has no `in()`. Use `trait:<name> links(...)` to find trait lines
with matching links, not `link within(trait:<name>)`.

If you see SQLite/FTS errors during full-text search, treat them as query-syntax issues and simplify or quote punctuation-heavy terms.

## Common agent patterns

Real open todos:

```text
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo"})
```

Open todos in briefs:

```text
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo within(type:brief)"})
```

Open todos under a topic heading or section:

```text
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo within(section includes(.title, \"pricing\"))"})
```

Open todos in a path plus structured filter:

```text
raven_invoke(command="query", args={"query_string":"type:page matches(.path, \"^pages/work/\") contains(trait:todo .value==todo)"})
```

Text mentions instead of real traits:

```text
raven_invoke(command="search", args={"query":"@todo pricing"})
```

If this returns relevant files, convert the follow-up to `query` so the result set is trait-aware and safe to mutate.
