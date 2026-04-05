# Bulk Operations Reference

Bulk operations let you act on many objects found by a query. All bulk operations preview by default—add `--confirm` to apply changes.

Use `querying/query-language.md` for query syntax and `using-your-vault/configuration.md` for saved query definitions.

## Overview

There are two approaches to bulk operations:

1. **`--apply` flag** — Run an operation directly on query results
2. **Piping with `--ids`** — Get IDs and pipe to another command with `--stdin`

---

## `--apply` Operations

The `--apply` flag runs an operation on all query results.

### Syntax

```bash
rvn query "<query>" --apply "<command> [args...]" [--confirm]
```

### Supported Commands

| Query type | `--apply` commands |
|------------|--------------------|
| `object:...` | `set field=value...`, `add <text...>`, `delete`, `move <destination/>` |
| `trait:...` | `update <new_value>` |

### Preview vs Apply

```bash
# Preview (default) - shows what would change
rvn query "object:project .status==active" --apply "set reviewed=true"

# Apply - actually makes the changes
rvn query "object:project .status==active" --apply "set reviewed=true" --confirm
```

---

## Set Fields

Update frontmatter fields on matching objects.

### Examples

```bash
# Set single field
rvn query "object:project .status==active" --apply "set reviewed=true" --confirm

# Set multiple fields
rvn query "object:person !exists(.status)" --apply "set status=active role=member" --confirm

# Clear a field (set to empty)
rvn query "object:project .status==archived" --apply "set priority=" --confirm
```

### Behavior

- Works on both file-level and embedded objects
- Fields are validated against the schema
- New fields can be added (if allowed by schema)

---

## Update Trait Values

Use trait queries when you want to update trait values directly.

### Examples

```bash
# Mark all open todos as done
rvn query "trait:todo .value==todo" --apply "update done" --confirm

# Promote urgent priority traits to critical
rvn query "trait:priority .value==urgent" --apply "update critical" --confirm
```

### Behavior

- Works only on trait query results (`trait:...`)
- Preserves the trait name and updates only the trait value
- Validates the new value against schema trait constraints

---

## Add Text

Append text to the end of matching files.

### Examples

```bash
# Add a note
rvn query "object:project .status==active" --apply "add ## Reviewed on $(date +%Y-%m-%d)" --confirm

# Add a trait
rvn query "object:project .status==active" --apply "add @reviewed(2026-01-10)" --confirm

# Add with reference
rvn query "object:meeting" --apply "add See also: [[project/website]]" --confirm
```

### Behavior

- Only works on file-level objects (embedded objects are skipped)
- Text is appended to the end of the file
- Respects the file's existing formatting

---

## Delete

Delete matching objects (moves to trash by default).

### Examples

```bash
# Delete archived projects
rvn query "object:project .status==archived" --apply "delete" --confirm

# Delete old daily notes (be careful!)
rvn query "object:date" --ids | head -100 | rvn delete --stdin --confirm
```

### Behavior

- Files are moved to `.trash/` by default (configurable)
- Only works on file-level objects (embedded objects are skipped)
- Does NOT automatically update backlinks

**Warning:** Always check backlinks before deleting:

```bash
# Check what references these objects first
for id in $(rvn query "object:project .status==archived" --ids); do
  echo "=== $id ==="
  rvn backlinks "$id"
done
```

---

## Move

Move matching objects to a directory.

### Examples

```bash
# Archive old projects
rvn query "object:project .status==archived" --apply "move archive/project/" --confirm

# Reorganize people
rvn query "object:person .role==contractor" --apply "move contractor/" --confirm
```

### Behavior

- Destination must end with `/` (it's a directory)
- Only works on file-level objects (embedded objects are skipped)
- References are updated automatically
- Creates destination directory if needed

---

## Piping with `--ids`

For complex operations, get IDs and pipe to other commands.

### `--ids` Flag

Outputs one ID per line for piping:

```bash
# Object query - outputs object IDs
rvn query "object:project .status==active" --ids

# Trait query - outputs trait IDs
rvn query "trait:due .value<today" --ids
```

### Piping Examples

```bash
# Set fields via pipe
rvn query "object:project .status==active" --ids | rvn set --stdin priority=high --confirm

# Update trait values via pipe
rvn query "trait:todo .value==todo" --ids | rvn update --stdin done --confirm

# Delete via pipe
rvn query "object:project .status==archived" --ids | rvn delete --stdin --confirm

# Move via pipe
rvn query "object:project .status==archived" --ids | rvn move --stdin archive/project/ --confirm
```

### Combining with Shell Tools

```bash
# Process first 10 results
rvn query "object:project" --ids | head -10 | rvn set --stdin reviewed=true --confirm

# Filter with grep
rvn query "object:person" --ids | grep "team-" | rvn set --stdin department=engineering --confirm
```

---

## Commands Supporting `--stdin`

| Command | Behavior |
|---------|----------|
| `rvn set` | Set fields on each object (file-level and embedded) |
| `rvn update` | Update each trait value (trait IDs only) |
| `rvn add` | Append text to each file (file-level only) |
| `rvn delete` | Delete each object (file-level only) |
| `rvn move` | Move each object (file-level only) |

All stdin commands require `--confirm` to apply (preview by default).

---

## Object Type Limitations

### File-Level Objects

Full path like `person/freya`:

- All operations work
- Can be deleted, moved, have text appended

### Embedded Objects

Path with fragment like `project/website#tasks`:

- `set` works (updates the type declaration)
- `add`, `delete`, `move` skip these objects

When running bulk operations, embedded objects are skipped with a note in the preview:

```
Skipping embedded object: project/website#tasks (use set for embedded objects)
```

---

## Error Handling

Bulk operations collect errors and continue processing:

```bash
$ rvn query "object:project" --apply "set status=invalid-value" --confirm
Updated: project/alpha (status=invalid-value) [ERROR: invalid enum value]
Updated: project/beta (status=invalid-value) [ERROR: invalid enum value]
Updated: project/gamma (status=invalid-value) [ERROR: invalid enum value]

3 errors occurred. See above for details.
```

### Rollback

Raven doesn't have built-in rollback. Use git:

```bash
# Inspect what changed
git status
git diff

# Restore specific files
git restore person/freya.md project/website.md

# Restore all tracked files in the working tree (use with care)
git restore .
```

---

## Common Patterns

### Mark Items as Reviewed

```bash
# Add a reviewed trait to all active projects
rvn query "object:project .status==active" --apply "add @reviewed($(date +%Y-%m-%d))" --confirm
```

### Archive Old Content

```bash
# Move archived projects to archive folder
rvn query "object:project .status==archived" --apply "move archive/project/" --confirm
```

### Fix Missing Fields

```bash
# Find objects missing a field and set a default
rvn query "object:person !exists(.status)" --apply "set status=active" --confirm
```

### Update Enum Values After Schema Change

```bash
# After adding "critical" to priority enum, update old "urgent" values
rvn query "object:project .priority==urgent" --ids | rvn set --stdin priority=critical --confirm
```

### Clean Up Overdue Items

```bash
# Mark projects with overdue items
rvn query "object:project has(trait:due .value<today)" --apply "set status=overdue" --confirm
```

### Batch Create Tags

```bash
# Add a reviewed marker to all frontend projects
rvn query "object:project .category==frontend" --apply "add @reviewed(2026-01-10)" --confirm
```

---

## Safety Checklist

Before running bulk operations:

1. **Preview first** — Run without `--confirm` to see what will change
2. **Check the count** — Make sure the number of affected objects is expected
3. **Verify the query** — Run `rvn query "..." --json` to inspect full results
4. **Commit first** — `git commit -am "Before bulk operation"` so you can rollback
5. **Start small** — Use `| head -5` to test on a few objects first

```bash
# Safe flow
git add -A && git commit -m "Before bulk update"
rvn query "object:project .status==archived" --apply "move archive/"
# Review preview...
rvn query "object:project .status==archived" --apply "move archive/" --confirm
```

---

## Related docs

- `vault-management/import.md` — bulk importing from external JSON data
- `querying/query-language.md` — full RQL syntax for queries
- `using-your-vault/common-commands.md` — individual commands (`rvn set`, `rvn move`, `rvn delete`, etc.)
