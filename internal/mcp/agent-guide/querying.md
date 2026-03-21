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

## Examples

```text
raven_invoke(command="query", args={"query_string":"object:project .status==active"})
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo"})
raven_invoke(command="query", args={"query_string":"object:meeting refs([[projects/website]])"})
```

For text search inside typed queries, use `content("term")`.

If you see SQLite/FTS errors during full-text search, treat them as query-syntax issues and simplify or quote punctuation-heavy terms.
