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

## Query vs search

- Use `query` when you want real Raven items or real trait instances.
- Use `search` when you only know a text fragment and do not yet know the type, trait, or structure.
- `search` returns file/snippet matches. It does not distinguish a real `@todo` trait from plain prose that happens to mention `@todo`.
- If the user asks for actual open tasks, due items, briefs, or typed items, start with `query`.

## Examples

```text
raven_invoke(command="query", args={"query_string":"type:project .status==active"})
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo"})
raven_invoke(command="query", args={"query_string":"type:meeting refs([[project/website]])"})
```

For text search inside typed queries, use `content("term")`.

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
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo within(type:section content(\"pricing\"))"})
```

Open todos in a path plus structured filter:

```text
raven_invoke(command="query", args={"query_string":"type:page matches(.path, \"^pages/work/\") has(trait:todo .value==todo)"})
```

Text mentions instead of real traits:

```text
raven_invoke(command="search", args={"query":"@todo pricing"})
```

If this returns relevant files, convert the follow-up to `query` so the result set is trait-aware and safe to mutate.
