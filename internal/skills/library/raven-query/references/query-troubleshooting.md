# Query Troubleshooting

## No matches returned

- Check type and field names with `rvn schema`.
- Validate query mode: `type:<type>` vs `trait:<name>` vs `asset`.
- Remove predicates one-by-one to isolate the failing constraint.

## Ambiguous references

- Resolve the target first: `rvn resolve <ref> --json`.
- Use full object IDs in `[[...]]` when ambiguity exists.

## Unexpectedly broad results

- Add explicit predicates first (`.status==active`, `on(type:...)`, `within(type:...)`).
- Use `--limit` and inspect IDs before any apply command.

## Shell parsing issues

- Wrap query strings in single quotes.
- Keep regex and parentheses inside the quoted string.

## Apply rejected or unsafe

- Confirm query type:
  - type query: `set`, `add`, `delete`, `move`
  - trait query: `update <value>`
  - asset query: no `--apply` support
- Re-run without `--confirm` first to inspect preview.

## Asset query errors

- Use bare `asset`, not `asset:<kind>`.
- Filter only derived asset fields: `.id`, `.file_path`, `.filename`, `.extension`, `.media_type`, `.size_bytes`.
- Use `asset refd(...)` to find referenced assets; assets do not support `refs(...)`, `has(...)`, `content(...)`, or hierarchy predicates.

## Saved query input errors

- Ensure `{{args.<name>}}` placeholders match `args:` declared in `raven.yaml`.
- Pass missing inputs by position or `key=value` pairs.

## Stale index suspicion

- Use `rvn query ... --refresh --json`.
- If needed after broader file changes: `rvn reindex --json`.
