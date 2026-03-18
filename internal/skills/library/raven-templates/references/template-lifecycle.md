# Template Lifecycle

## Type template quick path

```bash
rvn template write meeting/standard.md --content '# Meeting Notes' --json
rvn schema template set meeting_standard --file templates/meeting/standard.md --json
rvn schema template bind meeting_standard --type meeting --json
rvn schema template default meeting_standard --type meeting --json
```

## Core type template quick path (`date`)

```bash
rvn template write daily.md --content '# Daily Note' --json
rvn schema template set daily_default --file templates/daily.md --json
rvn schema template bind daily_default --core date --json
rvn schema template default daily_default --core date --json
```

## Inspect current template state

```bash
rvn template list --json
rvn schema template list --json
rvn schema template list --type meeting --json
rvn schema template list --core date --json
```

## Safe teardown order

1. Clear or change defaults:
```bash
rvn schema template default --type meeting --clear --json
rvn schema template default --core date --clear --json
```
2. Unbind IDs:
```bash
rvn schema template unbind meeting_standard --type meeting --json
rvn schema template unbind daily_default --core date --json
```
3. Remove schema template IDs:
```bash
rvn schema template remove meeting_standard --json
rvn schema template remove daily_default --json
```
4. Delete template files:
```bash
rvn template delete meeting/standard.md --json
rvn template delete daily.md --json
```

Use `--force` on delete only when stale schema references are expected and intentionally ignored.
