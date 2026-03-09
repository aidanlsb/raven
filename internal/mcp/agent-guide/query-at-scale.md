# Query At Scale

Use this guide when keep size is large, queries are expensive, or result sets are broad.

## Strategy

1. Start with a selective query.
2. Measure result volume with `count-only=true`.
3. Page results with `limit`/`offset`.
4. Fetch full file content only for narrowed candidates.

## Volume controls

### Count first

```text
raven_query(query_string="trait:todo .value==todo", count-only=true)
```

### Page results

```text
raven_query(query_string="object:project .status==active", limit=50, offset=0)
raven_query(query_string="object:project .status==active", limit=50, offset=50)
```

### Return only IDs for follow-up operations

```text
raven_query(query_string="object:project .status==archived", ids=true)
```

Use IDs for targeted mutation pipelines (`stdin=true`) after preview and user approval.

## Narrowing patterns

- Prefer structured predicates (`.field==...`, `has(...)`, `within(...)`, `refs(...)`) over broad text search.
- Add location constraints early (`within(object:meeting)` vs global trait search).
- Use `content("...")` only when exact structured fields are unavailable.

## Retrieval pattern (two-phase)

1. Query for candidate IDs/paths.
2. Read only the candidates needed for user-facing detail.

```text
# Phase 1: narrow
raven_query(query_string="object:meeting refs([[projects/website]])", limit=20)

# Phase 2: inspect specific hits
raven_read(path="meetings/team-sync", raw=true)
```

## When to use search instead

Use `raven_search` for fuzzy discovery when type/trait is unknown. Return to `raven_query` once structure is identified.

## Related topics

- `raven://guide/querying` - predicate reference and composition
- `raven://guide/query-cheatsheet` - quick query patterns
- `raven://guide/key-workflows` - bulk update flow after query narrowing
