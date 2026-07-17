# Raven Onboarding

Use this skill when a user wants to learn Raven, set up their first vault, or have an agent walk them through Raven concepts. It works end-to-end from a clean machine with **no vault yet**, as well as for users who already have a vault.

This skill is CLI-first. Use MCP as a fallback when CLI access is unavailable, preserving the same JSON expectations.

## Operating rules

- Begin by detecting state, not by assuming a vault exists. A brand-new user may have zero vaults, no default, and no active vault — that is a normal starting point, not an error.
- Do not assume the current working directory is the intended vault. Once a vault exists, confirm context with `rvn vault current --json` and `rvn vault path --json` when possible.
- Once a vault exists, teach from the user's actual vault: inspect `raven.yaml`, `schema.yaml`, types, traits, and vault stats before giving workflow-specific advice.
- Use `rvn ... --json` for all commands so results are deterministic.
- Prefer Raven commands over direct file edits for onboarding demos.
- Ask before changing schema or global/default vault routing.
- Let `rvn init` own vault registration and default/active routing. Read its `post_init` output and do not blindly re-run `vault add` / `vault use` when init already handled them.

## First-session flow

1. **Detect vault state.** Run `rvn vault list --json` and read the result:
   - Empty `vaults` (or `meta.count` of `0`) means there is no vault yet — follow the new-vault path below.
   - One or more entries means at least one vault exists — follow the existing-vault tour. Use `active_vault` / `default_vault` to see whether one is already selected.
   - There is no need to run `rvn vault current --json` first when no vault exists; detection above already covers the empty state.
2. **If there is no vault yet, create one (primary path):**
   - Ask where to create it. Suggest a sensible default such as `~/notes` or `~/raven`, but let the user choose.
   - Run `rvn init <path> --json`.
   - Read `post_init` from the result and honor what init already did:
     - If `is_first_vault` is `true`, this was the first vault on the machine: it is registered, set as default, and active, and `post_init.actions` is empty — proceed directly, no further routing needed.
     - If `already_registered` is `true`, the vault is registered — do not run `rvn vault add` again.
     - If `is_default` is `true` and/or `is_active` is `true`, routing is already set — do not run `rvn vault pin` / `rvn vault use` for those.
     - If `is_first_vault` is `false` with `needs_user_choice_for_activate` / `needs_user_choice_for_default` set to `true`, another vault already exists; `post_init.actions` lists the `activate` / `set_default` actions — ask the user before running them.
     - If init did **not** register or route the vault (fields are `false`, or `post_init` is absent on older builds), offer to finish setup using the commands in `post_init.commands` / `post_init.next_steps`. Ask before setting a default or active vault, since routing is machine-wide config.
   - After init, work against this vault (pass `--vault <name>` or `--vault-path <path>` if it is not yet the active vault).
3. **Set up the editor (once a vault exists).** Keep this short — it is one onboarding step, not an editor deep-dive.
   - Inspect current settings with `rvn config show --json` (and note `$EDITOR` if it is relevant to the user's choice).
   - If `editor` is unset, or the user wants to change it, ask which editor they use (common: `cursor`, `code`, `nvim`).
   - Apply with `rvn config set --editor <cmd> --editor-mode auto --json`. Ask before changing machine-wide config, consistent with the rule about global/default vault routing.
   - Confirm with `rvn config show --json`.
   - **LSP pointer (awareness only).** Mention that Raven ships a built-in LSP (`rvn lsp`) that gives diagnostics, completion, go-to-definition, and find-references in any editor with an LSP client, and point at `docs/using-your-vault/editor-integration.md` (or the `rvn docs` equivalent). Do not auto-configure nvim/vscode plugins here — make the user aware and hand off.
4. **Explain the vault model:**
   - Markdown files are the durable source of truth.
   - `.raven/` is derived cache and local metadata.
   - `raven.yaml` is vault-local config.
   - `schema.yaml` defines types, fields, traits, and templates.
5. Inspect schema shape with `rvn schema --json`; use `rvn schema type <name> --json` and `rvn schema trait <name> --json` for focused explanations.
6. Demonstrate one safe create flow with `rvn new <type> "<title>" --json`, choosing an existing simple type.
7. Demonstrate traits by adding a line with a defined trait through `rvn add "..." --to today --json` or explaining how to create a missing trait with `rvn schema add trait ... --json`.
8. Demonstrate references with `[[object/id]]`, then use `rvn backlinks <id> --json` to show the graph.
9. Demonstrate daily notes with `rvn daily --json` and `rvn add "..." --json`.
10. Verify health with `rvn check --json`; use `rvn reindex --json` if the index is stale.

## Teaching points

- Types describe whole objects/files, such as projects, people, meetings, notes, books, or issues.
- Fields are frontmatter properties on typed objects.
- Traits are inline annotations in body text, useful for tasks, decisions, priorities, reading lists, and other line-level facts.
- References are `[[id]]` links to objects or sections; use exact resolved IDs when automation depends on them.
- Daily notes are a built-in capture workflow, not a replacement for typed project or meeting objects.

## Cross-references

- Use `raven-vault-admin` for vault setup, active/default vault selection, and deeper config changes (editor, UI, and other machine-level settings).
- Use `raven-schema` when onboarding requires new types, fields, or traits.
- Use `raven-core` for creating objects, adding notes, editing content, daily notes, and references.
- Use `raven-query` for search, structured query examples, and saved queries.
- Use `raven-maintenance` for `rvn check`, `rvn check fix`, and reindexing.

## Load references as needed

- End-to-end scripts and prompt templates: `references/onboarding-playbook.md`
