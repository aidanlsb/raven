# Quickstart Mental Model

Use this when the user is new to Raven and needs one coherent explanation before details.

## Raven in 60 seconds

Raven is plain markdown + schema + query:
- Files are the source of truth.
- `schema.yaml` defines types, fields, and traits.
- `raven_query` lets agents answer questions from structured data instead of guessing.

## Core model

- **Type**: category with structure (`project`, `person`, `meeting`)
- **Object**: one file of a type (`projects/website.md`)
- **Trait**: inline annotation in body content (`@todo`, `@due`)
- **Reference**: wiki link (`[[people/freya]]`) connecting files
- **Index**: rebuildable cache (`raven_reindex`), never the source of truth

## First commands to run

```text
raven_schema(subcommand="types")
raven_schema(subcommand="traits")
raven_stats()
raven_query(list=true)
raven_workflow_list()
```

Then fetch:
- `raven://schema/current` for full schema
- `raven://queries/saved` for saved query names
- `raven://workflows/list` for available workflow entry points
- `raven://vault/agent-instructions` for vault-specific agent guidance (if present)

## Recommended first create flow

```text
create = raven_new(type="project", title="Website Redesign")
raven_add(text="## Notes\n- Kickoff next week", to=create.data.file)
```

If no existing type fits, use:

```text
create = raven_new(type="page", title="Quick Note")
raven_add(text="## Notes\n- ...", to=create.data.file)
```

## Related topics

- `raven://guide/getting-started` for discovery sequence details
- `raven://guide/core-concepts` for deeper model details
- `raven://guide/query-cheatsheet` for fast query patterns
- `raven://guide/lesson-plan` for a full teaching path
