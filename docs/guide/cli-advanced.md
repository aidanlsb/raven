# CLI Advanced

Use this guide after you are comfortable with `cli-basics.md`.

Goal: run large or high-impact operations safely and predictably.

## Preview-first bulk operations

Most bulk operations support preview by default and apply only with `--confirm`.

### Apply operations through query

```bash
# Preview
rvn query 'object:project .status==active' --apply 'set reviewed=true'

# Apply
rvn query 'object:project .status==active' --apply 'set reviewed=true' --confirm
```

### Update trait values in bulk

```bash
# Preview marking todo -> done
rvn query 'trait:status .value==todo' --apply 'update value=done'

# Apply
rvn query 'trait:status .value==todo' --apply 'update value=done' --confirm
```

### Pipe IDs between commands

```bash
rvn query 'object:project .status==active' --ids | rvn set --stdin priority=high --confirm
```

## Schema evolution workflow

Use a controlled sequence for schema changes:

```bash
rvn schema validate
rvn check
rvn reindex --full
```

Common schema operations:

```bash
rvn schema add type meeting --name-field title --default-path meetings/
rvn schema add field meeting attendees --type ref[] --target person
rvn schema add trait decision --type bool
```

Use rename helpers when changing existing models:

```bash
rvn schema rename type event meeting --confirm
rvn schema rename field meeting host facilitator --confirm
```

## Safe content surgery

For deterministic content edits:

```bash
# Preview
rvn edit daily/2026-02-15.md "- old text" "- new text"

# Apply
rvn edit daily/2026-02-15.md "- old text" "- new text" --confirm
```

For path changes with reference updates:

```bash
rvn move projects/old-plan projects/new-plan
```

## Validation and recovery

### Find and fix structural issues

```bash
rvn check --errors-only
rvn check --issues missing_reference,unknown_type
```

### Rebuild index state

```bash
rvn reindex
rvn reindex --full
```

Use full reindex after schema changes or external file operations.

## Workflow automation

Use workflows for repeatable, multi-step operations:

```bash
rvn workflow list
rvn workflow show meeting-prep
rvn workflow run meeting-prep --input meeting_id=meetings/team-sync
```

Persist deterministic outputs with upsert:

```bash
rvn upsert brief "Daily Brief 2026-02-15" --content "# Daily Brief"
```

## Multi-vault and scripting patterns

### Target specific vaults

```bash
rvn --vault personal query 'trait:status .value==todo'
rvn --vault-path /tmp/scratch-vault stats
```

### Prefer JSON for automation

```bash
rvn query 'object:project .status==active' --json
rvn check --json
```

## Keep this nearby

- `reference/cli.md` for exact flags and arguments
- `reference/query-language.md` for complex query composition
- `reference/bulk-operations.md` for operation semantics and safety details

