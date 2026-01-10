# Bulk Operations Reference

Bulk operations let you act on many objects found by a query. All bulk operations preview by default—add `--confirm` to apply changes.

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

| Command | Description |
|---------|-------------|
| `set field=value...` | Update frontmatter fields |
| `add <text...>` | Append text to files |
| `delete` | Delete matching objects |
| `move <destination/>` | Move to directory (must end with `/`) |

### Preview vs Apply

```bash
# Preview (default) - shows what would change
rvn query "trait:due value:past" --apply "set status=overdue"

# Apply - actually makes the changes
rvn query "trait:due value:past" --apply "set status=overdue" --confirm
```

---

## Set Fields

Update frontmatter fields on matching objects.

### Examples

```bash
# Set single field
rvn query "object:project .status:active" --apply "set reviewed=true" --confirm

# Set multiple fields
rvn query "object:person !.status:*" --apply "set status=active role=member" --confirm

# Clear a field (set to empty)
rvn query "object:project .status:archived" --apply "set priority=" --confirm

# Set on trait query results
rvn query "trait:due value:past" --apply "set status=overdue" --confirm
```

### Behavior

- Works on both file-level and embedded objects
- Fields are validated against the schema
- New fields can be added (if allowed by schema)

---

## Add Text

Append text to the end of matching files.

### Examples

```bash
# Add a note
rvn query "object:project .status:active" --apply "add ## Reviewed on $(date +%Y-%m-%d)" --confirm

# Add a trait
rvn query "object:project .status:active" --apply "add @reviewed(2026-01-10)" --confirm

# Add with reference
rvn query "object:meeting" --apply "add See also: [[projects/website]]" --confirm
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
rvn query "object:project .status:archived" --apply "delete" --confirm

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
for id in $(rvn query "object:project .status:archived" --ids); do
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
rvn query "object:project .status:archived" --apply "move archive/projects/" --confirm

# Reorganize people
rvn query "object:person .role:contractor" --apply "move contractors/" --confirm
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
rvn query "object:project .status:active" --ids

# Trait query - outputs trait IDs
rvn query "trait:due value:past" --ids
```

### `--object-ids` Flag

For trait queries, outputs the containing object IDs (deduplicated):

```bash
# Get objects that contain overdue traits
rvn query "trait:due value:past" --object-ids
```

### Piping Examples

```bash
# Set fields via pipe
rvn query "object:project .status:active" --ids | rvn set --stdin priority=high --confirm

# Add text via pipe
rvn query "trait:due value:past" --object-ids | rvn add --stdin "@reviewed(2026-01-10)" --confirm

# Delete via pipe
rvn query "object:project .status:archived" --ids | rvn delete --stdin --confirm

# Move via pipe
rvn query "object:project .status:archived" --ids | rvn move --stdin archive/projects/ --confirm
```

### Combining with Shell Tools

```bash
# Process first 10 results
rvn query "object:project" --ids | head -10 | rvn set --stdin reviewed=true --confirm

# Filter with grep
rvn query "object:person" --ids | grep "team-" | rvn set --stdin department=engineering --confirm

# Save IDs for later
rvn query "trait:due value:past" --object-ids > overdue.txt
cat overdue.txt | rvn set --stdin status=overdue --confirm
```

---

## Commands Supporting `--stdin`

| Command | Behavior |
|---------|----------|
| `rvn set` | Set fields on each object (file-level and embedded) |
| `rvn add` | Append text to each file (file-level only) |
| `rvn delete` | Delete each object (file-level only) |
| `rvn move` | Move each object (file-level only) |

All stdin commands require `--confirm` to apply (preview by default).

---

## Object Type Limitations

### File-Level Objects

Full path like `people/freya`:

- All operations work
- Can be deleted, moved, have text appended

### Embedded Objects

Path with fragment like `projects/website#tasks`:

- `set` works (updates the type declaration)
- `add`, `delete`, `move` skip these objects

When running bulk operations, embedded objects are skipped with a note in the preview:

```
Skipping embedded object: projects/website#tasks (use set for embedded objects)
```

---

## Error Handling

Bulk operations collect errors and continue processing:

```bash
$ rvn query "object:project" --apply "set status=invalid-value" --confirm
Updated: projects/alpha (status=invalid-value) [ERROR: invalid enum value]
Updated: projects/beta (status=invalid-value) [ERROR: invalid enum value]
Updated: projects/gamma (status=invalid-value) [ERROR: invalid enum value]

3 errors occurred. See above for details.
```

### Rollback

Raven doesn't have built-in rollback. Use git:

```bash
# Undo all changes since last commit
git checkout .

# Or restore specific files
git checkout -- people/freya.md projects/website.md
```

---

## Common Patterns

### Mark Items as Reviewed

```bash
# Add a reviewed trait to all active projects
rvn query "object:project .status:active" --apply "add @reviewed($(date +%Y-%m-%d))" --confirm
```

### Archive Old Content

```bash
# Move archived projects to archive folder
rvn query "object:project .status:archived" --apply "move archive/projects/" --confirm
```

### Fix Missing Fields

```bash
# Find objects missing a field and set a default
rvn query "object:person !.status:*" --apply "set status=active" --confirm
```

### Update Enum Values After Schema Change

```bash
# After adding "critical" to priority enum, update old "urgent" values
rvn query "trait:priority value:urgent" --object-ids | rvn set --stdin priority=critical --confirm
```

### Clean Up Overdue Items

```bash
# Mark overdue items with a status
rvn query "trait:due value:past" --apply "set status=overdue" --confirm

# Or add a note
rvn query "trait:due value:past" --object-ids | rvn add --stdin "@flagged Overdue - needs attention" --confirm
```

### Batch Create Tags

```bash
# Add a tag to all projects in a category
rvn query "object:project .category:frontend" --ids | while read id; do
  current=$(rvn read "$id.md" --json | jq -r '.frontmatter.tags // []')
  # ... update tags ...
done
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
# Safe workflow
git add -A && git commit -m "Before bulk update"
rvn query "object:project .status:archived" --apply "move archive/" 
# Review preview...
rvn query "object:project .status:archived" --apply "move archive/" --confirm
```
