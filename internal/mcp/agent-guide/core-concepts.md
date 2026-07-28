# Core Concepts

Use this guide to explain Raven's model.

## Source of truth

- Vault files are durable state.
- The SQLite index is derived and rebuildable.
- Schema drives typed validation and indexing.

## Main concepts

- **Type**: named structure in `schema.yaml`
- **Object**: one markdown file of a type
- **Trait**: inline annotation in body content
- **Reference**: wiki link to another object or section
- **Link edge**: outgoing Markdown link or image to a file or URL
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

Non-Markdown files are not Raven identities. Copy them into the vault directly,
run `reindex`, and link them with standard Markdown such as
`[PDF](../files/file.pdf)`. Standard links and images render portably when
Markdown becomes HTML; `[[...]]` is reserved for Raven object and section
references and does not render as a file link or image.

## Command reference arguments

Commands that target one existing vault item use `reference`; retired
`object` / `object_id` spellings are not aliases. Most bulk target commands use
`references`; bulk `add`/`move` use `object_ids`, and bulk `update` uses
`trait_ids`. Confirm the exact array key with `raven_describe`. The shared
reference grammar accepts canonical object IDs, vault-relative Markdown paths,
section IDs (`object#fragment`), aliases, name-field values,
and unambiguous short forms. Date-aware commands also accept ISO and documented
dynamic dates. Agents should pass canonical object or section IDs.

Restrictions still apply by command:
- `resolve` and `open`: objects or sections.
- `read`: managed Markdown files or sections.
- `set`, `unset`, and `reclassify`: file-level Markdown objects only.
- `delete`: file-backed objects or explicit non-Markdown file paths, never sections.
- `edit`: managed Markdown files or section subtrees, not config/schema/templates/non-Markdown files.
- `check` and `check fix`: file, directory, or object scope; omit `reference` for the whole vault.
- `backlinks`: objects or sections.
- `outlinks`: objects or sections.
