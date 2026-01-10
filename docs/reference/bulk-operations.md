# Bulk operations (reference)

Bulk operations let you act on many objects found by a query.

## `rvn query --ids`

`--ids` prints one **result ID** per line, suitable for piping.

- For **object queries**, this is the object ID.
- For **trait queries**, this is the trait ID.

Examples:

```bash
rvn query "object:project .status:active" --ids
rvn query "trait:due value:past" --ids
```

## `rvn query --object-ids`

For piping trait query results into object-based bulk commands, use `--object-ids`:

- For **object queries**, it is identical to `--ids`.
- For **trait queries**, it outputs the **containing object IDs** (deduped).

```bash
rvn query "trait:due value:past" --object-ids
```

## Piping to `--stdin` commands

Commands that accept `--stdin` can read IDs from standard input:

```bash
rvn query "object:project .status:active" --ids | rvn add --stdin "Quarterly review scheduled"
rvn query "trait:due value:past" --object-ids | rvn set --stdin status=blocked
rvn query "object:project .status:archived" --ids | rvn delete --stdin
```

Notes:
- `rvn set --stdin` supports **file-level** and **embedded** object IDs.
- `rvn add/delete/move --stdin` operate on **file-level** IDs (embedded IDs are skipped).

## `rvn query --apply ...` (shorthand)

`--apply` is sugar for “query → act”.

Syntax:

```bash
rvn query "<query>" --apply <command> [args...] [--confirm]
```

Supported commands:
- `set field=value...`
- `add <text...>`
- `delete`
- `move <destination-dir/>` (must end with `/`)

Examples:

```bash
# Preview (default)
rvn query "trait:due value:past" --apply set status=overdue

# Apply
rvn query "trait:due value:past" --apply set status=overdue --confirm

# Move results into a directory (preview/apply)
rvn query "object:project .status:archived" --apply move archive/projects/
rvn query "object:project .status:archived" --apply move archive/projects/ --confirm
```

## Preview vs apply

Bulk operations are **preview-only by default**.

- Without `--confirm`: shows a preview (and warnings).
- With `--confirm`: applies changes.

