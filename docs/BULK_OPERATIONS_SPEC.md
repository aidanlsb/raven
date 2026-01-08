# Bulk Operations on Query Results

**RFC Status:** Implemented  
**Author:** —  
**Date:** January 2026

## Summary

Enable bulk operations on query results through Unix-style piping and a `--apply` flag, allowing users and agents to transform multiple objects in a single command.

## Motivation

Currently, acting on query results requires manual iteration:

```bash
# Find overdue items, then update each one manually
rvn query "trait:due value:past"
# → returns 15 results
rvn set projects/bifrost status=blocked
rvn set projects/midgard status=blocked
# ... repeat 13 more times
```

This is tedious for humans and inefficient for agents. A bulk operation capability would:

1. Reduce multi-step workflows to single commands
2. Enable powerful composition with existing Unix tools
3. Make agents significantly more effective at vault maintenance
4. Align with Raven's philosophy of composable, queryable plain text

## Design Principles

1. **Composition over features** — Use pipes and existing commands rather than inventing new syntax
2. **Explicit over implicit** — Bulk operations should be obvious, not hidden
3. **Dry-run by default** — Destructive operations require confirmation
4. **Streaming** — Process results incrementally, don't load everything into memory

## Specification

### 1. Structured Output Mode

Queries gain a `--ids` flag that outputs just object IDs, one per line:

```bash
rvn query "trait:due value:past" --ids
```

Output:
```
projects/bifrost
projects/midgard
daily/2026-01-02#standup
people/freya
```

For trait queries, output the containing object's ID (the object the trait appears in).

### 2. Piping to Commands

Commands that accept an object ID (`set`, `delete`, `move`, `add`) gain a `--stdin` flag to read IDs from standard input:

```bash
# Set status=blocked on all objects with overdue items
rvn query "trait:due value:past" --ids | rvn set --stdin status=blocked

# Delete all objects tagged for cleanup
rvn query "trait:cleanup" --ids | rvn delete --stdin

# Add a note to all active projects
rvn query "object:project .status:active" --ids | rvn add --stdin "Review scheduled for Q2"
```

### 3. The `--apply` Shorthand

For common cases, `--apply` provides inline syntax without pipes:

```bash
rvn query "trait:due value:past" --apply set status=overdue
rvn query "object:project .client:[[clients/acme]]" --apply delete
rvn query "trait:status value:done" --apply add "@archived(2026-01-07)"
```

This is syntactic sugar for the pipe form. The grammar is:

```
rvn query "<query>" --apply <command> [args...]
```

Where `<command>` is one of: `set`, `delete`, `add`, `move`.

### 4. Dry-Run by Default

All bulk operations preview changes before applying:

```bash
$ rvn query "trait:due value:past" --apply set status=overdue

Preview: 4 objects will be modified

  projects/bifrost
    + status: overdue (was: active)
  
  projects/midgard  
    + status: overdue (was: active)
  
  daily/2026-01-02#standup
    + status: overdue (was: <unset>)
  
  people/freya
    + status: overdue (was: <unset>)

Run with --confirm to apply changes.
```

Apply with `--confirm`:

```bash
rvn query "trait:due value:past" --apply set status=overdue --confirm
```

### 5. Filtering with Standard Tools

Because `--ids` outputs plain text, users can filter with `grep`, `head`, `fzf`, etc.:

```bash
# Only projects (not daily notes)
rvn query "trait:due value:past" --ids | grep "^projects/" | rvn set --stdin status=blocked

# First 5 results only
rvn query "trait:due value:past" --ids | head -5 | rvn set --stdin status=blocked

# Interactive selection with fzf
rvn query "trait:due value:past" --ids | fzf --multi | rvn set --stdin status=blocked
```

### 6. Supported Commands

#### `set --stdin`

Read object IDs from stdin, apply field updates to each:

```bash
echo "projects/bifrost" | rvn set --stdin status=paused priority=high
```

Behavior:
- Updates frontmatter fields on each object
- For embedded objects, updates the inline fields after the heading
- Skips objects that don't exist (with warning)
- Validates field values against schema

#### `delete --stdin`

Read object IDs from stdin, delete each:

```bash
echo "projects/old-thing" | rvn delete --stdin
```

Behavior:
- Moves to trash by default (respects `deletion.behavior` config)
- Warns about backlinks before deletion
- `--force` skips confirmation even in interactive mode

#### `add --stdin`

Read object IDs from stdin, append text to each:

```bash
echo "projects/bifrost" | rvn add --stdin "New note appended here"
```

Behavior:
- Appends to end of file (for file objects)
- Appends to end of section (for embedded objects)
- Supports traits in the added text: `"@reviewed(2026-01-07) Looks good"`

#### `move --stdin`

Read object IDs from stdin, move each to a destination pattern:

```bash
rvn query "object:project .status:completed" --ids | rvn move --stdin archive/projects/
```

Behavior:
- Destination can be a directory (preserves filename) or include `{name}` placeholder
- Updates references by default (`--update-refs=false` to disable)
- Fails if destination already exists (no silent overwrite)

### 7. JSON Output Mode

For programmatic use, `--json` outputs structured results:

```bash
rvn query "trait:due value:past" --apply set status=overdue --json
```

```json
{
  "ok": true,
  "data": {
    "action": "set",
    "fields": {"status": "overdue"},
    "results": [
      {"id": "projects/bifrost", "status": "modified"},
      {"id": "projects/midgard", "status": "modified"},
      {"id": "daily/2026-01-02#standup", "status": "modified"},
      {"id": "people/freya", "status": "skipped", "reason": "field not in schema"}
    ]
  },
  "meta": {
    "total": 4,
    "modified": 3,
    "skipped": 1
  }
}
```

### 8. Transaction Semantics

Bulk operations are **not atomic**. Each object is processed independently:

- If object 3 of 10 fails, objects 1-2 are already modified
- Errors are collected and reported at the end
- Use `--stop-on-error` to halt on first failure

Rationale: Atomic transactions would require either:
- Copying all files before modification (slow, doubles disk usage)
- A write-ahead log (complexity not justified for a notes system)

Users needing atomicity should use git:

```bash
git stash
rvn query "..." --apply set status=done --confirm
git diff  # review
git checkout .  # rollback if needed
```

### 9. Agent Considerations

For MCP tool exposure:

```yaml
tools:
  raven_query:
    # existing params...
    apply:
      type: object
      properties:
        command:
          type: string
          enum: [set, delete, add, move]
        args:
          type: object
        confirm:
          type: boolean
          default: false
```

Agents should:
1. First run without `--confirm` to preview
2. Present the preview to the user
3. Only run with `--confirm` after user approval

### 10. Error Handling

| Scenario | Behavior |
|----------|----------|
| Object doesn't exist | Skip with warning |
| Field not in schema | Skip with warning (for `set`) |
| Invalid field value | Skip with error |
| Permission denied | Fail with error |
| Embedded object in read-only file | Fail with error |

Warnings don't prevent other objects from being processed. Errors are collected and reported in the summary.

## Examples

### Mark all overdue items as blocked

```bash
rvn query "trait:due value:past" --apply set status=blocked --confirm
```

### Archive completed projects

```bash
rvn query "object:project .status:completed" --ids \
  | rvn move --stdin archive/projects/
```

### Add review date to all highlights from last month

```bash
rvn query "trait:highlight within:{object:date .date:2025-12-*}" \
  --apply add "@reviewed(2026-01-07)" --confirm
```

### Delete orphaned meeting notes (no backlinks)

```bash
rvn query "object:meeting" --ids | while read id; do
  if [ -z "$(rvn backlinks "$id" --ids)" ]; then
    echo "$id"
  fi
done | rvn delete --stdin --confirm
```

### Interactive bulk edit with fzf

```bash
rvn query "object:person" --ids \
  | fzf --multi --preview "rvn read {}" \
  | rvn set --stdin team=platform
```

## Implementation Notes

### Phase 1: Core Pipeline

1. Add `--ids` flag to `rvn query`
2. Add `--stdin` flag to `set`, `delete`, `add`
3. Implement dry-run preview logic

### Phase 2: Apply Shorthand

1. Parse `--apply <cmd> [args...]` syntax
2. Wire to existing command handlers
3. Add `--confirm` flag

### Phase 3: Move Support

1. Add `--stdin` to `move` command
2. Handle destination patterns
3. Batch reference updates efficiently

### Performance Considerations

- Stream IDs rather than collecting all in memory
- Batch index updates (single reindex at end, not per-object)
- For large operations, show progress: `Processing 142/500...`

## Alternatives Considered

### Alternative A: Query-native mutations

```bash
rvn query "trait:due value:past" --set status=blocked
```

Rejected because:
- Conflates querying and mutation
- Harder to compose with external tools
- Less explicit about what's happening

### Alternative B: Dedicated `bulk` command

```bash
rvn bulk set status=blocked --query "trait:due value:past"
```

Rejected because:
- Inverts the natural query-then-act flow
- New command rather than composing existing ones

### Alternative C: Interactive TUI

Rejected because:
- Doesn't work for agents
- Adds significant complexity
- Unix pipes are more flexible

## Open Questions

1. **Should `--apply` support multiple commands?**
   
   e.g., `--apply set status=done --apply add "@completed"`
   
   Leaning no — use a shell loop for multi-step operations.

2. **Should there be a `--limit` flag?**
   
   e.g., `rvn query "..." --apply delete --limit 10`
   
   Probably yes for safety, but `head -10` works with pipes.

3. **How to handle embedded objects in `move`?**
   
   Moving `daily/2026-01-02#standup` doesn't make sense — it's part of a file. Should this error, or extract to a new file?
   
   Leaning toward error with a helpful message.

## References

- [Raven Query Language](./QUERY_LOGIC.md)
- [MCP Tool Specification](./SPECIFICATION.md#mcp-server)
