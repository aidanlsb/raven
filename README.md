<h1 align="center">Raven</h1>

<p align="center"><strong>A CLI for plain-text knowledge management, with first-class support for AI agents.</strong></p>

A Raven "vault" is a folder of markdown files, with a few additional features:

- **Types & schema.** You define types for your notes, and their fields (YAML frontmatter) are validated against the schema.
- **Traits.** Inline markers like `@todo` or `@due(2026-07-22)` attach queryable metadata to particular pieces of content.
- **References.** `[[wiki-style]]` links connect notes to each other, and Raven maintains them as files are renamed or moved.
- **Queries.** A full query language allows precise retrieval, by you or your agent.

Here's an example:

```markdown
---
type: meeting
title: Security review kickoff
project: project/midgard-security-review
---

- Met with [[person/freya]] on [[2026-07-20]] to plan the security review 
- Agreed to focus the first pass on authentication and infrastructure
- Work cannot begin until the draft scope is approved

@todo @due(2026-07-22) Send the draft scope for review
```

If you need todos inside meetings that reference the Midgard security review project, ask your agent or run this query:

```bash
rvn query 'trait:todo within(type:meeting refs(project/midgard-security-review))'
```

The sections below cover setup.

## Installation

Install with Homebrew:

```bash
brew tap aidanlsb/tap
brew install aidanlsb/tap/rvn
rvn version
```

Or install with Go:

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
rvn version
```

Prebuilt binaries for Linux, macOS, and Windows are available on the [releases page](https://github.com/aidanlsb/raven/releases/latest). See [Installation](docs/getting-started/installation.md) for details.

That is the full install: a single `rvn` binary. You can create a vault by hand or let an agent set one up and teach you (below). To drive everything yourself, skip to [Doing it by hand](#doing-it-by-hand).

## Get started with an agent

Open your agent in the directory where you want your notes and paste this prompt:

```
Install Raven and let it onboard me:

1. Check if `rvn` is on PATH with `rvn version`. If missing:
   - Prefer Homebrew: `brew tap aidanlsb/tap && brew install aidanlsb/tap/rvn`
   - If brew is unavailable: `go install github.com/aidanlsb/raven/cmd/rvn@latest`
   - If neither brew nor go is available, point me at https://github.com/aidanlsb/raven/releases/latest
   - Confirm with `rvn version` after install

2. Install shipped skills with `rvn skill install --confirm --json` (agents cannot answer the interactive y/N prompt; without --confirm the command only previews)

3. Use the `raven-onboarding` skill to set up my vault and walk me through how Raven works
```

The onboarding skill detects that you have no vault yet, creates and registers one, then introduces Raven's concepts. It helps you shape a schema around what you track and gets you creating and querying notes in the context of your own vault. From there you can ask for work, like:

> Add a meeting note for today's kickoff with Freya on the Midgard security review, capture any decisions and follow-ups, and link it to the project and to Freya.

See [Agent Setup](docs/getting-started/agent-setup.md) for the skill catalog and other setup paths.

### Optional: MCP

Skills teach CLI workflows and do not require MCP. To additionally give MCP-native agents direct tool access to Raven's commands, install Raven into a supported client:

```bash
rvn mcp install --client claude-code
rvn mcp install --client claude-desktop
rvn mcp install --client cursor
rvn mcp install --client codex
rvn mcp status
```

Or print a manual config snippet:

```bash
rvn mcp show
rvn mcp show --client cursor
```

See the full [MCP reference](docs/agents/mcp.md), [Installation](docs/getting-started/installation.md), and [First Vault](docs/getting-started/first-vault.md) guides for more setup details.

## Doing it by hand

You can drive Raven directly. This section shows the same flow (tracking projects, meetings, and the people involved) done manually.

First, create a vault:

```bash
rvn init ~/notes
```

Raven creates:

```text
notes/
├── .gitignore    # ignores .raven/ and .trash/
├── .raven/       # derived cache and local metadata (rebuildable with `rvn reindex`)
├── daily/        # date notes (`YYYY-MM-DD.md`)
├── page/         # untyped pages
├── templates/    # type templates
├── type/         # typed notes (`type/<type>/...`)
├── raven.yaml    # vault configuration (including `directories`)
└── schema.yaml   # types, fields, and traits
```

Those content folders match the `directories` block in `raven.yaml`: typed notes
live under `type/<type>/` (for example `type/project/...`), pages under `page/`,
and daily notes under `daily/`.

On a fresh machine, this first `rvn init` registers `~/notes` in global config and sets it as your default and active vault, so the CLI can find it right away. When another vault already exists, `rvn init` registers and activates the new vault immediately while leaving `default_vault` unchanged. JSON `post_init` reports `switch_back` (the exact `rvn vault use ...` command) to return to the previous vault; use `rvn vault pin` to change the default. See [Vault Creation & Management](docs/getting-started/first-vault.md).

### 1. Extend the schema

Each file Raven indexes has a **type**, and custom types can require or allow specific **fields**. The starter schema gives you `project` and `person`; we'll add `meeting`.

We want each meeting to record which project it belongs to, so its schema includes a `project` reference. The `todo` and `due` traits used in the example are already part of the starter schema.

You can edit `schema.yaml` directly:

```yaml
types:
  meeting:
    default_path: meeting/
    name_field: title
    fields:
      title:
        required: true
        type: string
      project:
        type: ref
        target: project
```

`default_path` is relative to `directories.type` (`type/` after `rvn init`), so
new meetings go in `type/meeting/`.

You can make the same changes from the CLI or ask your agent:

```bash
rvn schema add type meeting --name-field title --default-path meeting/
rvn schema add field meeting project --type ref --target project
```

### 2. Create some notes

Create objects with the CLI. Each typed note is a Markdown file under
`type/<type>/` (`type/project/`, `type/person/`, and so on):

```bash
rvn new project "Midgard Security Review"
rvn new person "Freya"
rvn daily 2026-07-20
```

That writes `type/project/midgard-security-review.md` and
`type/person/freya.md`. The `daily` command creates a built-in date note at
`daily/2026-07-20.md`, giving the `[[2026-07-20]]` reference in our meeting a
target. Create `type/meeting/security-review-kickoff.md` as a Markdown file:

```markdown
---
type: meeting
title: Security review kickoff
project: project/midgard-security-review
---

Met with [[person/freya]] on [[2026-07-20]] to plan the security review. We agreed to focus the first pass on authentication and infrastructure, but work cannot begin until the draft scope is approved.

@todo @due(2026-07-22) Send the draft scope for review
```

> **A note on identifiers.** Typed notes live under `type/<type>/`. The file
> `type/project/midgard-security-review.md` has canonical ID
> `project/midgard-security-review`. Built-in date notes live under `daily/` and
> use the bare ISO date as their ID. Frontmatter references and most command
> arguments use canonical IDs, not file paths. Inside `[[links]]`, you can also
> use a shorter form like `[[freya]]` when it's unambiguous.

### 3. Query your vault

The Raven Query Language (RQL) retrieves notes and traits by structure, not just text. For example:

```bash
# Open todos inside meetings that reference the Midgard project
rvn query 'trait:todo within(type:meeting refs([[project/midgard-security-review]]))'

# Items due by July 22 inside those same meetings
rvn query 'trait:due .value<=2026-07-22 within(type:meeting refs([[project/midgard-security-review]]))'
```

Read the first query as: *find `todo` traits that live within a `meeting` that references `project/midgard-security-review`.* Both queries return the task from our note:

```text
type/meeting/security-review-kickoff.md
  @todo @due(2026-07-22) Send the draft scope for review
```

References form a graph regardless of whether they point to a person, project, or date. Follow it with backlinks:

```bash
rvn backlinks person/freya
rvn backlinks 2026-07-20
```

```text
type/meeting/security-review-kickoff.md
  Met with [[person/freya]] on [[2026-07-20]] to plan the security review.
```

### 4. Ask your agent

With the vault populated, ask the question from the top of this README. The agent can run the same deterministic queries and backlink lookups you just used, then answer with the source note. See [Core Concepts](docs/getting-started/core-concepts.md) and the [Query Language](docs/querying/query-language.md) guide.

Raven includes a built-in language server for diagnostics, completion, navigation, and hover while you edit. See the [editor integration guide](docs/using-your-vault/editor-integration.md) to configure `rvn lsp` in your editor.

## Daily notes & quick capture

Daily notes give you a date-stamped file for each day. Use them for journaling, quick capture, or meeting notes. Each is a `date`-typed note, so everything you've seen (types, traits, references, queries) applies.

```bash
rvn daily                 # open (creating if needed) today's note
rvn daily yesterday       # or a relative day
rvn daily 2026-07-20      # or a specific date
```

`rvn add` appends a line to today's note without opening it:

```bash
rvn add "Met with [[person/freya]] about the rollout"
rvn add "@todo Send scope doc to [[person/freya]]"
rvn add "Prep for standup" --to tomorrow   # target another day or file
```

Because daily notes are just `date`-typed notes, the traits and references you
capture stay queryable. Pull today's open todos, or review everything tied to a
day with the read-only `rvn date` hub:

```bash
rvn query 'trait:todo within([[2026-07-20]])'   # todos captured that day
rvn date today                                  # everything connected to a date
```

You can give new daily notes consistent structure with a template, and change
where they're stored via `directories.daily` in `raven.yaml`. See the
[Daily Notes](docs/using-your-vault/daily-notes.md) guide for the full workflow.

## Documentation

- [Documentation Map](docs/getting-started/documentation-map.md)
- [Installation](docs/getting-started/installation.md)
- [First Vault](docs/getting-started/first-vault.md)
- [Core Concepts](docs/getting-started/core-concepts.md)
- [Agent Setup](docs/getting-started/agent-setup.md)
- [Daily Notes](docs/using-your-vault/daily-notes.md)
- [Common Commands](docs/using-your-vault/common-commands.md)
- [File Links](docs/using-your-vault/file-links.md)
- [Configuration](docs/using-your-vault/configuration.md)
- [Schema Introduction](docs/types-and-traits/schema-intro.md)
- [Schema Reference](docs/types-and-traits/schema.md)
- [File Format](docs/types-and-traits/file-format.md)
- [References](docs/types-and-traits/references.md)
- [Templates](docs/types-and-traits/templates.md)
- [Query Language](docs/querying/query-language.md)
- [RQL Internals](docs/querying/internals.md)
- [Bulk Operations](docs/vault-management/bulk-operations.md)
- [Import](docs/vault-management/import.md)
- [MCP Reference](docs/agents/mcp.md)
- [Local HTTP API](docs/agents/local-api.md) — companion UIs such as Perch attach to `rvn api`, then poll `GET /v1/revision` for live vault changes (no event stream)

You can also browse the docs from the CLI:

```bash
rvn docs
```

## License

Raven is released under the [MIT License](LICENSE).
