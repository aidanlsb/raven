# Core Concepts

Use this guide to explain Raven's model.

## Source of truth

- Markdown files and vault-local asset files are durable state.
- The SQLite index is derived and rebuildable.
- Schema drives typed validation and indexing.

## Main concepts

- **Type**: named structure in `schema.yaml`
- **Object**: one markdown file of a type
- **Asset**: vault-local non-Markdown resource such as an image or PDF
- **Trait**: inline annotation in body content
- **Reference**: wiki link to another object, section, or asset; Markdown links/images can also point to assets
- **Saved query**: named query in `raven.yaml`

## How agents should inspect the model

Use schema introspection through the compact surface:

```text
raven_invoke(command="schema", args={"subcommand":"types"})
raven_invoke(command="schema", args={"subcommand":"type", "name":"project"})
```

Type and field descriptions in `schema.yaml` are part of the user's terminology. Read them before making assumptions.

Author wikilinks and `ref` fields with canonical object IDs. After `new`,
`upsert`, or `daily`, take the ID from `data.id`; the human CLI presents the
same value as `link as <id>`. Short forms may resolve when unambiguous, but they
are resolution sugar and agents should not generate them.

Assets are scanned from `directories.assets` in `raven.yaml`, not modeled in
`schema.yaml`. When adding images, PDFs, or other non-Markdown files, place them
under the configured asset root and link with full Markdown paths such as
`[PDF](assets/pdfs/file.pdf)`. Use `[[assets/pdfs/file.pdf]]` for a semantic-only
graph reference.

## Command reference arguments

Commands that target existing vault content use `reference` for one input and
`references` for bulk input. The shared grammar accepts canonical object IDs,
vault-relative Markdown paths, section IDs (`object#fragment`), full asset
paths, aliases, name-field values, and unambiguous short forms. Date-aware
commands also accept ISO and documented dynamic dates. Agents should pass
canonical IDs, section IDs, or full asset paths.

Restrictions still apply by command:
- `resolve` and `open`: objects, sections, or indexed assets.
- `read`: managed Markdown files or sections.
- `set`, `unset`, and `reclassify`: file-level typed objects only.
- `delete`: file-backed objects or assets, never sections.
- `edit`: managed Markdown files or section subtrees, not config/schema/templates/assets.
- `check` and `check fix`: file, directory, or object scope; omit `reference` for the whole vault.
- `backlinks`: objects, sections, or indexed assets.
- `outlinks`: objects or sections.
