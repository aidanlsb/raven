# Documentation map

Use this page to choose the shortest path through Raven's long-form docs. For
exact flags and arguments, use `rvn help <command>`; these pages explain the
model, rules, and end-to-end workflows.

## Start here

| Goal | Read next |
|---|---|
| Install Raven and create a vault | [Installation](installation.md) → [First vault](first-vault.md) |
| Learn the mental model | [Core concepts](core-concepts.md) |
| Connect an agent | [Agent setup](agent-setup.md) → [MCP reference](../agents/mcp.md) |
| Use everyday commands | [Common commands](../using-your-vault/common-commands.md) |
| Design or change a schema | [Schema introduction](../types-and-traits/schema-intro.md) → [Schema reference](../types-and-traits/schema.md) |
| Write structured Markdown | [File format](../types-and-traits/file-format.md) → [References](../types-and-traits/references.md) |
| Query the vault | [Query language](../querying/query-language.md) |
| Change many items | [Bulk operations](../vault-management/bulk-operations.md) |
| Import external data | [Import](../vault-management/import.md) |

## Browse by area

- **Getting started:** [installation](installation.md),
  [first vault](first-vault.md), [core concepts](core-concepts.md), and
  [agent setup](agent-setup.md).
- **Using a vault:** [common commands](../using-your-vault/common-commands.md),
  [daily notes](../using-your-vault/daily-notes.md),
  [file links](../using-your-vault/file-links.md),
  [configuration](../using-your-vault/configuration.md), and
  [editor integration](../using-your-vault/editor-integration.md).
- **Types and traits:** [schema introduction](../types-and-traits/schema-intro.md),
  [schema reference](../types-and-traits/schema.md),
  [file format](../types-and-traits/file-format.md),
  [references](../types-and-traits/references.md), and
  [templates](../types-and-traits/templates.md).
- **Querying:** [query language](../querying/query-language.md) and
  [maintainer internals](../querying/internals.md).
- **Vault management:** [bulk operations](../vault-management/bulk-operations.md)
  and [import](../vault-management/import.md).
- **Agents:** [MCP reference](../agents/mcp.md). MCP clients should begin with
  `raven://guide/index`; packaged skills are listed in
  [Agent setup](agent-setup.md).

## Agent lookup pattern

```bash
rvn docs --json
rvn docs querying query-language --json
rvn docs search "missing reference" --json
```

Over MCP, use the same docs command through the compact surface:

```text
raven_invoke(command="docs", args={"section":"querying","topic":"query-language"})
```

Agent-only operating rules live in `raven://guide/index`; the pages in this
tree remain the canonical long-form user documentation.
