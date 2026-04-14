# Query Recipes

Use these as templates and adapt predicates to the local schema.

## Discovery and narrowing

```bash
rvn query 'type:project' --count-only --json
rvn query 'type:project .status==active' --limit 20 --json
rvn query 'type:project .status==active' --ids --json
```

## References and hierarchy

```bash
rvn query 'type:meeting refs([[project/website]])' --json
rvn query 'type:meeting parent(type:date)' --json
rvn query 'type:project encloses(trait:todo .value==todo)' --json
```

## Trait-centric work

```bash
rvn query 'trait:due .value<today' --json
rvn query 'trait:todo within(type:project .status==active)' --json
rvn query 'trait:todo refs([[person/freya]])' --json
```

## Bulk operations

```bash
rvn query 'type:project has(trait:due .value<today)' --apply 'set status=overdue' --confirm --json
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
rvn backlinks project/website --json
rvn outlinks meeting/team-sync --json
```
