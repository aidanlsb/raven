# Agent Lesson Plan

Use this when teaching a new user. Follow this sequence to avoid overload.

## Sequence by stage

1. **Orient**
- Fetch `raven://guide/quickstart` and explain the model in plain language.
- Confirm the user's domain and what they track.

2. **Discover**
- Run:
  - `raven_schema(subcommand="types")`
  - `raven_schema(subcommand="traits")`
  - `raven_stats()`
- Explain what already exists before proposing new structure.

3. **Create one real object**
- Prefer concrete examples from the user's current work.
- Use `raven_new` first, then `raven_add` for body content.

4. **Query it back**
- Run one query that proves value immediately.
- Example: `raven_query(query_string="trait:todo .value==todo")`

5. **Introduce bulk/safety patterns**
- Show preview-first behavior and `confirm=true`.
- Show `raven_check` for validation and cleanup.

## Prerequisite map

- Before creating objects:
  - know required fields (`raven_schema(subcommand="type", name="<type>")`)
- Before bulk updates:
  - preview first, get user confirmation, then apply
- Before deleting:
  - inspect backlinks and discuss impact

## Common misconceptions to correct

- "Raven is a freeform file writer"
  - Correction: create via schema tools first, then append content.
- "The index is the database"
  - Correction: markdown files are canonical; index is rebuildable.
- "Search errors mean data is missing"
  - Correction: many are query syntax issues; retry with quoting or structured queries.
- "Bulk apply is safe without preview"
  - Correction: preview and explicit confirm are required workflow steps.

## Routing hints

- New user setup: `raven://guide/onboarding`
- Query composition help: `raven://guide/querying`
- Error recovery: `raven://guide/error-handling`
- End-to-end workflows: `raven://guide/key-workflows`
