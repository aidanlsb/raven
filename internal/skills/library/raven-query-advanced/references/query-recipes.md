# Query Recipes

Use these as templates and adapt predicates to the local schema.

## Discovery and narrowing

```bash
rvn query 'object:project' --count-only --json
rvn query 'object:project .status==active' --limit 20 --json
rvn query 'object:project .status==active' --ids --json
```

## References and hierarchy

```bash
rvn query 'object:meeting refs([[projects/website]])' --json
rvn query 'object:meeting parent(object:date)' --json
rvn query 'object:project encloses(trait:todo .value==todo)' --json
```

## Trait-centric work

```bash
rvn query 'trait:due .value<today' --json
rvn query 'trait:todo within(object:project .status==active)' --json
rvn query 'trait:todo refs([[people/freya]])' --json
```

## Bulk operations

```bash
rvn query 'object:project has(trait:due .value<today)' --apply 'set status=overdue' --confirm --json
rvn query 'trait:todo .value==todo' --apply 'update done' --confirm --json
```

## Saved query lifecycle

```bash
rvn query saved set overdue 'trait:due .value<today' --json
rvn query overdue --json
rvn query saved remove overdue --json
```

## Adjacent helpers

```bash
rvn search 'meeting notes' --type meeting --json
rvn backlinks projects/website --json
rvn outlinks meetings/team-sync --json
```
