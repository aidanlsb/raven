# Query Troubleshooting

Use this page to diagnose empty, broad, ambiguous, or rejected queries.

## No matches returned

- Check type and field names with `rvn schema`.
- Validate query mode: `type:<type>` vs `section` vs `trait:<name>` vs `link`.
- Remove predicates one-by-one to isolate the failing constraint.
- Scope trap: traits attach to the nearest section, so `type:project has(trait:todo)` (direct-only) usually returns nothing. Use `contains(trait:todo ...)` from the object side or `within(type:project)` from the trait side.

## Ambiguous references

- Resolve the target first: `rvn resolve <reference> --json`.
- Use the returned canonical object ID in `[[...]]`; canonical targets avoid
  ambiguity and are the preferred authoring form.

## Unexpectedly broad results

- Add explicit predicates first (`.status==active`, `in(type:...)`, `within(type:...)`).
- Use `--limit` and inspect IDs before any apply command.

## Shell parsing issues

- Wrap query strings in single quotes.
- Keep regex and parentheses inside the quoted string.

## Apply rejected or unsafe

- Confirm query type:
  - type query: `set`, `add`, `delete`, `move`
  - trait query: `update <value>`
  - section query: a `move` plan parses but file-level bulk `move` rejects
    section IDs; use `rvn section move <file#slug>`
  - link query: no `--apply` support
- Re-run without `--confirm` first to inspect preview.

## Saved query input errors

- Ensure `{{args.<name>}}` placeholders match `args:` declared in `raven.yaml`.
- Pass missing inputs by position or `key=value` pairs.

## Stale index suspicion

- Use `rvn query ... --refresh --json`.
- If needed after broader file changes: `rvn reindex --json`.

## Related guidance

- [Back to Raven Query](../body.md)
- [Query language](query-language.md)
- [Query recipes](query-recipes.md)
- Canonical long-form RQL guide:
  `rvn docs querying query-language --json`
