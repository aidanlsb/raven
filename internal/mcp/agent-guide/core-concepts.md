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
- **Asset kind**: organization/validation rule for assets; not a schema type
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

Asset kinds live in `raven.yaml` under `assets.kinds`, not in `schema.yaml`. When adding images, PDFs, or other non-Markdown files, place them under the configured asset root and link with normal Markdown paths such as `[PDF](assets/pdfs/file.pdf)`. Use `[[assets/pdfs/file.pdf]]` for a semantic-only graph reference, and only use short asset refs such as `[[file]]` when unambiguous.
