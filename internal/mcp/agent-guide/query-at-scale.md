# Query At Scale

Use this guide when a query can return many results.

Queries are unlimited by default (`limit` 0), so an unscoped query returns the
entire result set in one call. Probe first, then page.

## 1. Count before reading everything

```text
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo", "count-only":true})
```

Count-only returns just `total` — the cheapest way to size a result set.

## 2. Page through results

```text
raven_invoke(command="query", args={"query_string":"type:project .status==active", "limit":50, "offset":0})
raven_invoke(command="query", args={"query_string":"type:project .status==active", "limit":50, "offset":50})
```

Each paged response includes `total`, `returned`, `offset`, `limit`, and
`has_more`. When `has_more` is `true` the response also includes `next_offset`.
Loop while `has_more` is `true`, sending the returned `next_offset` as the next
request's `offset`; stop when `has_more` is `false`. Do not guess offsets.

## 3. Use IDs for follow-up flows

```text
raven_invoke(command="query", args={"query_string":"type:project .status==archived", "ids":true})
```

## 4. Narrow before reading files

```text
raven_invoke(command="query", args={"query_string":"type:meeting refs([[project/website]])", "limit":20})
raven_invoke(command="read", args={"reference":"meeting/team-sync", "raw":true})
```

## Practical rules

- Prefer structured predicates over wide text search.
- Use `count-only` to size a result set, then `limit`/`offset` to page instead of pulling everything.
- Loop on `has_more`, advancing `offset` to `next_offset` each iteration.
- Read only the files you need after narrowing with queries.

## Related topics

- `raven://guide/querying`
- `raven://guide/query-cheatsheet`
- `raven://guide/response-contract`
