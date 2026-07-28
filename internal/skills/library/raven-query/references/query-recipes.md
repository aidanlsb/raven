# Query Recipes

Use these as templates and adapt predicates to the local schema.

## Discovery and narrowing

```bash
rvn query 'type:project' --count-only --json
rvn query 'type:project .status==active' --limit 20 --json
rvn query 'type:project .status==active' --ids --json
```

## References and scope

Lead with the forgiving scope forms `contains(...)`/`within(...)`; traits attach to the nearest section, so `has(trait:todo)`/`in(type:...)` miss todos nested under headings.

```bash
rvn query 'type:meeting refs([[project/website]])' --json
rvn query 'section within(type:project .status==active)' --json
rvn query 'type:project contains(trait:todo .value==todo)' --json
rvn query 'trait:todo .value==todo within(type:project .status==active)' --json
```

## Assets

```bash
rvn query 'asset .extension==pdf' --json
rvn query 'asset startswith(.media_type, "image/")' --json
rvn query 'asset refd(type:project .status==active)' --json
```

## Outgoing links

```bash
rvn query 'type:project links(.ext==pdf)' --json
rvn query 'trait:todo links(.is_image==true)' --json
rvn query 'link .ext==pdf within(type:project)' --json
```

Use `links(...)` when the results should remain source entities; use the bare
`link` root when the results should be individual edge rows. Both use the same
complete link-field grammar.

## Trait-centric work

```bash
rvn query 'trait:due .value<today' --json
rvn query 'trait:todo within(type:project .status==active)' --json
rvn query 'trait:todo refs([[person/freya]])' --json
rvn query 'trait:tags any(.value, _ == "raven")' --json
rvn query 'trait:reviewers any(.value, _ == [[person/freya]])' --json
```

## Bulk operations

```bash
rvn query 'type:project has(trait:due .value<today)' --apply 'set status=overdue' --json
rvn query 'type:project has(trait:due .value<today)' --apply 'set status=overdue' --confirm --json
rvn query 'trait:todo .value==todo' --apply 'update done' --json
rvn query 'trait:todo .value==todo' --apply 'update done' --confirm --json
```

In each pair, the first command previews the bulk change and the second applies it after approval. Section, asset, and link queries do not support `--apply`; for sections, pipe IDs into bulk add instead: `rvn query 'section .title==Tasks' --ids | rvn add "text" --stdin --confirm`.

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
