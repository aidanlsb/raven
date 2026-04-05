# Query At Scale

Use this guide when a query can return many results.

## 1. Count before reading everything

```text
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo", "count-only":true})
```

## 2. Page through results

```text
raven_invoke(command="query", args={"query_string":"object:project .status==active", "limit":50, "offset":0})
raven_invoke(command="query", args={"query_string":"object:project .status==active", "limit":50, "offset":50})
```

## 3. Use IDs for follow-up flows

```text
raven_invoke(command="query", args={"query_string":"object:project .status==archived", "ids":true})
```

## 4. Narrow before reading files

```text
raven_invoke(command="query", args={"query_string":"object:meeting refs([[project/website]])", "limit":20})
raven_invoke(command="read", args={"path":"meeting/team-sync", "raw":true})
```

## Practical rules

- Prefer structured predicates over wide text search.
- Use `limit` and `offset` instead of pulling entire result sets.
- Read only the files you need after narrowing with queries.
