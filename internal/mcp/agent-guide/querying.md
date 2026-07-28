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

- `query` — real Raven items, real trait instances, or indexed asset rows; filter by type/section/trait/asset, fields, scope, and references.
- `search` — you only know a text fragment and do not yet know the type, trait, or structure. Returns file/snippet matches; it does NOT distinguish a real `@todo` trait from prose that mentions `@todo`. For real traits use `query "trait:todo"`.
- `backlinks <reference>` — incoming references to one object/asset (structured equivalent: `query "... refd(...)"`; `read` also appends backlinks).
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
raven_invoke(command="query", args={"query_string":"type:page refs([[assets/pdfs/paper.pdf]])"})
raven_invoke(command="query", args={"query_string":"asset .extension==pdf"})
raven_invoke(command="query", args={"query_string":"asset startswith(.media_type, \"image/\")"})
raven_invoke(command="query", args={"query_string":"asset refd(type:project .status==active)"})
```

For text search inside typed queries, use `content("term")`.

Sections use the bare `section` query root and return heading-derived rows with
IDs like `file#slug`. Section rows include `line_start`, direct `line_end`, and
`subtree_line_end`. Body appends with `add` use the direct range so they land
before child headings; structural placement with `section_create` and
`section_move` uses complete subtree boundaries. Section query IDs can be piped
into bulk `add` (`--ids` + `stdin=true`) to append inside each matching section,
and `read` with `sections=true` returns a file's outline directly.

Assets can be reference targets in `refs(...)` and `refd(...)` flows, including links discovered from Markdown links/images. Use the bare `asset` query root to return asset rows directly.

Asset queries support derived metadata fields only: `.id`, `.file_path`, `.filename`, `.extension`, `.media_type`, and `.size_bytes`. Assets do not have outbound refs, traits, or scope, so `asset refs(...)`, `asset has(...)`, and scope predicates are invalid.

Use `links(...)` on type, section, or trait roots to filter by outgoing non-Raven
links, for example `type:project links(.ext==pdf)` or
`trait:todo links(.is_image==true)`. Link fields are `.ext`, `.is_image`,
`.scheme`, `.raw_target`, `.display`, and `.normalized_key`. There is no
`linkd()` inverse because external files and URLs are leaf targets.

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
