# Query Troubleshooting

## No matches returned

- Check type and field names with `rvn schema`.
- Validate query mode: `object:<type>` vs `trait:<name>`.
- Remove predicates one-by-one to isolate the failing constraint.

## Ambiguous references

- Resolve the target first: `rvn resolve <ref> --json`.
- Use full object IDs in `[[...]]` when ambiguity exists.

## Unexpectedly broad results

- Add explicit predicates first (`.status==active`, `on(object:...)`, `within(object:...)`).
- Use `--limit` and inspect IDs before any apply command.

## Shell parsing issues

- Wrap query strings in single quotes.
- Keep regex and parentheses inside the quoted string.

## Apply rejected or unsafe

- Confirm query type:
  - object query: `set`, `add`, `delete`, `move`
  - trait query: `update <value>`
- Re-run without `--confirm` first to inspect preview.

## Saved query input errors

- Ensure `{{args.<name>}}` placeholders match `args:` declared in `raven.yaml`.
- Pass missing inputs by position or `key=value` pairs.

## Stale index suspicion

- Use `rvn query ... --refresh --json`.
- If needed after broader file changes: `rvn reindex --json`.
