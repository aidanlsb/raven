# Editor integration (LSP)

Raven ships a Language Server Protocol server. Any editor with an LSP client
gets diagnostics, completion, quick-fix Code Actions, go-to-definition,
find-references, and hover on vault files.

```bash
rvn lsp
```

The server speaks LSP 3.17 over stdio. It is part of the `rvn` binary. There
is nothing extra to install.

## Vault selection

The server picks its vault in this order:

1. `--vault-path` or `--vault` flags (explicit pin).
2. The editor workspace root, when it is a Raven vault (contains `raven.yaml`,
   `schema.yaml`, or `.raven/`).
3. The active or default vault from Raven config (`rvn vault use`).

For most setups no flags are needed: open your vault directory in the editor
and the workspace root is used.

## Capabilities

| Capability | What it does |
|---|---|
| Diagnostics | Validates open buffers as you type: broken or ambiguous `[[refs]]`, undefined `@traits`, invalid trait values, unknown frontmatter keys, missing required fields, and more. Diagnostic codes match `rvn check` issue types (e.g. `missing_reference`). |
| Code Actions | Offers in-buffer quick fixes when a diagnostic has one unambiguous textual replacement. In v1 this expands short refs such as `[[freya]]` to their full ID and removes non-canonical configured-root prefixes. Display text in links such as `[[freya\|The Queen]]` is preserved. |
| Completion | `[[` completes object IDs and aliases from the index. `@` completes trait names from the schema. Frontmatter key positions complete the declared type's fields. |
| Go-to-definition | Jump from a `[[wikilink]]` or bare frontmatter `ref` / `ref[]` value to its target file or section heading. Ambiguous refs list all candidates. |
| Find-references | Backlinks to the current file's object, or to the reference target under the cursor. |
| Hover | Preview a reference target: object ID, type, frontmatter fields, and the first lines of the body. |

The index is refreshed incrementally each time a file is saved. The server also
detects commits from an external `rvn reindex` and reloads its index-backed
caches on the next request or diagnostics pass. Diagnostics for unsaved buffer
content are computed fully in memory.

## Neovim

Neovim 0.11+ with the built-in LSP client:

```lua
vim.lsp.config('raven', {
  cmd = { 'rvn', 'lsp' },
  filetypes = { 'markdown' },
  root_markers = { 'raven.yaml', '.raven' },
})
vim.lsp.enable('raven')
```

Use Neovim's normal Code Action command (for example,
`vim.lsp.buf.code_action()`) on a Raven diagnostic to apply an available
quick fix. Lightbulb plugins discover the same `quickfix` actions.

With `nvim-lspconfig` on older Neovim versions:

```lua
local configs = require('lspconfig.configs')
if not configs.raven then
  configs.raven = {
    default_config = {
      cmd = { 'rvn', 'lsp' },
      filetypes = { 'markdown' },
      root_dir = require('lspconfig.util').root_pattern('raven.yaml', '.raven'),
    },
  }
end
require('lspconfig').raven.setup({})
```

To pin a vault regardless of the workspace root, add flags to `cmd`:

```lua
cmd = { 'rvn', 'lsp', '--vault', 'personal' },
```

## Other editors

Any LSP client works with the same command. For example, Helix
(`languages.toml`):

```toml
[language-server.raven]
command = "rvn"
args = ["lsp"]

[[language]]
name = "markdown"
language-servers = ["raven"]
```

## Notes

- The server holds a shared handle on the SQLite index (`.raven/index.db`) in
  WAL mode. Incremental `rvn reindex` and other normal commands can run
  alongside it.
- `rvn reindex --full` and automatic incompatible-schema replacement require
  exclusive index access. If one reports that the index is locked, stop the LSP
  or wait for the other index user to finish, then retry.
- Index-backed features (completion, references) reflect the last saved state
  of files. Diagnostics always reflect the current buffer.
- The buffer's own file must be saved once before other files can resolve
  references to it.
