# Quickstart Mental Model

Use this when the user is new to Raven and needs one coherent explanation before details.

## Raven in 60 seconds

Raven is plain markdown + schema + query:
- Files are the source of truth.
- `schema.yaml` defines types, fields, and traits.
- Agents should use the compact MCP surface: discover, describe, then invoke registry commands.

## Core model

- **Type**: category with structure (`project`, `person`, `meeting`)
- **Object**: one file of a type (`projects/website.md`)
- **Trait**: inline annotation in body content (`@todo`, `@due`)
- **Reference**: wiki link (`[[people/freya]]`) connecting files
- **Index**: rebuildable cache, never the source of truth

## First commands to run

```text
raven_invoke(command="schema", args={"subcommand":"types"})
raven_invoke(command="schema", args={"subcommand":"traits"})
raven_invoke(command="vault_stats")
raven_invoke(command="query_saved_list")
```

Then fetch:
- `raven://schema/current`
- `raven://queries/saved`
- `raven://vault/agent-instructions`

## Recommended first create flow

```text
create = raven_invoke(command="new", args={"type":"project", "title":"Website Redesign"})
raven_invoke(command="add", args={"text":"## Notes\n- Kickoff next week", "to":create.data.file})
```

If no existing type fits:

```text
create = raven_invoke(command="new", args={"type":"page", "title":"Quick Note"})
raven_invoke(command="add", args={"text":"## Notes\n- ...", "to":create.data.file})
```

## Related topics

- `raven://guide/getting-started`
- `raven://guide/core-concepts`
- `raven://guide/write-patterns`
- `raven://guide/response-contract`
- `raven://guide/query-cheatsheet`
- `raven://guide/onboarding`
