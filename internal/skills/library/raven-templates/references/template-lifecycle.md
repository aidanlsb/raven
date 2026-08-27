# Template Lifecycle

Use this page to create, bind, inspect, and safely remove template files.

## Type template quick path

```bash
rvn template write meeting/standard.md --content '# Meeting Notes' --json
rvn schema template set meeting_standard --file templates/meeting/standard.md --json
rvn schema template bind meeting_standard --type meeting --default --json
rvn new meeting "Weekly Standup" --json
```

Re-run the bind command with `--default` to change the default when the template
ID is already bound.

## Core type template quick path (`date`)

```bash
rvn template write daily.md --content '# Daily Note' --json
rvn schema template set daily_default --file templates/daily.md --json
rvn schema template bind daily_default --core date --default --json
rvn daily tomorrow --json
```

## Path semantics

- `rvn template write meeting/standard.md ...` writes under `directories.template`, producing `templates/meeting/standard.md` by default.
- `rvn schema template set ... --file ...` resolves and stores the vault-relative template file path, for example `templates/meeting/standard.md`.
- Template writes replace the full file body.
- Template content is copied when an object is created; changing a template does not update existing notes.

## Inspect current template state

```bash
rvn template list --json
rvn schema template list --json
rvn schema template list --type meeting --json
rvn schema template list --core date --json
```

## Safe teardown order

1. Unbind IDs, adding `--clear-default` for the current default:
```bash
rvn schema template unbind meeting_standard --type meeting --clear-default --json
rvn schema template unbind daily_default --core date --clear-default --json
```
2. Remove schema template IDs:
```bash
rvn schema template remove meeting_standard --json
rvn schema template remove daily_default --json
```
3. Delete template files:
```bash
rvn template delete meeting/standard.md --json
rvn template delete daily.md --json
```

Use `--force` on delete only when stale schema references are expected and intentionally ignored.

Safety blockers:

- `rvn schema template remove` blocks while a template ID is bound to any type or core type.
- `rvn template delete` blocks while any schema template definition references the file path.

## Related guidance

- [Back to Raven Templates](../body.md)
- Canonical template guide:
  `rvn docs types-and-traits templates --json`
- Template directory configuration:
  `rvn docs using-your-vault configuration --json`
