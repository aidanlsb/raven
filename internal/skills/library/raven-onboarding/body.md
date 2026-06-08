# Raven Onboarding

Use this skill when a user asks to learn Raven, set up a first vault, or have an agent walk them through Raven concepts in an existing vault.

This skill is CLI-first. Use MCP as a fallback when CLI access is unavailable, preserving the same JSON expectations.

## Operating rules

- Start by identifying whether the user needs a new vault, an existing vault connected, or a tour of the current vault.
- Do not assume the current working directory is the intended vault; verify with `rvn vault current --json` and `rvn vault path --json` when possible.
- Teach from the user's actual vault: inspect `raven.yaml`, `schema.yaml`, types, traits, and vault stats before giving workflow-specific advice.
- Use `rvn ... --json` for all commands so results are deterministic.
- Prefer Raven commands over direct file edits for onboarding demos.
- Ask before changing schema or global/default vault routing.

## First-session flow

1. Establish the vault context:
   - New vault: explain `rvn init <path> --json`, then ask for the target path before running it.
   - Existing vault: inspect `rvn vault list --json`, `rvn vault current --json`, and `rvn vault stats --json`.
2. Explain the vault model:
   - Markdown files are the durable source of truth.
   - `.raven/` is derived cache and local metadata.
   - `raven.yaml` is vault-local config.
   - `schema.yaml` defines types, fields, traits, and templates.
3. Inspect schema shape with `rvn schema --json`; use `rvn schema type <name> --json` and `rvn schema trait <name> --json` for focused explanations.
4. Demonstrate one safe create flow with `rvn new <type> "<title>" --json`, choosing an existing simple type.
5. Demonstrate traits by adding a line with a defined trait through `rvn add "..." --to today --json` or explaining how to create a missing trait with `rvn schema add trait ... --json`.
6. Demonstrate references with `[[object/id]]`, then use `rvn backlinks <id> --json` to show the graph.
7. Demonstrate daily notes with `rvn daily --json` and `rvn add "..." --json`.
8. Verify health with `rvn check --json`; use `rvn reindex --json` if the index is stale.

## Teaching points

- Types describe whole objects/files, such as projects, people, meetings, notes, books, or issues.
- Fields are frontmatter properties on typed objects.
- Traits are inline annotations in body text, useful for tasks, decisions, priorities, reading lists, and other line-level facts.
- References are `[[id]]` links to objects or sections; use exact resolved IDs when automation depends on them.
- Daily notes are a built-in capture workflow, not a replacement for typed project or meeting objects.

## Cross-references

- Use `raven-vault-admin` for vault setup, active/default vault selection, and config changes.
- Use `raven-schema` when onboarding requires new types, fields, or traits.
- Use `raven-core` for creating objects, adding notes, editing content, daily notes, and references.
- Use `raven-query` for search, structured query examples, and saved queries.
- Use `raven-maintenance` for `rvn check`, `rvn check fix`, and reindexing.

## Load references as needed

- End-to-end scripts and prompt templates: `references/onboarding-playbook.md`
