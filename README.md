<h1 align="center">Raven</h1>

<p align="center"><strong>A CLI for a folder of markdown notes. You use it. An agent can too.</strong></p>

A Raven vault is a folder of markdown files. The CLI adds structure on top of that:

- **Types and schema.** You define types for your notes. YAML frontmatter is validated against `schema.yaml`.
- **Traits.** Inline markers like `@todo` or `@due(2026-07-22)` attach queryable metadata to a specific line.
- **References.** `[[wiki-style]]` links connect notes. Raven rewrites them when you rename or move files.
- **Queries.** RQL finds objects and traits by type, fields, and links. Text search is a separate command.

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

Later you want "todos inside meetings that reference the Midgard security review project." Ask your agent, or run:

```bash
rvn query 'trait:todo within(type:meeting refs(project/midgard-security-review))'
```

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

Prebuilt binaries for Linux, macOS, and Windows are on the [releases page](https://github.com/aidanlsb/raven/releases/latest). See [Installation](docs/getting-started/installation.md) for checksums, `PATH` setup, and Gatekeeper notes.

That's the whole install: one `rvn` binary. If you have an agent, start there. It will create the vault and teach you. If you'd rather do it yourself, skip to [Doing it by hand](#doing-it-by-hand).

## Get started with an agent

Starting from a machine with no vault, an agent can create one, register it, and shape a schema around what you actually track. Raven ships [Agent Skills](https://agentskills.io/) that teach coding agents the CLI.

### 1. Install the skills

```bash
rvn skill install
```

This installs the shipped skills to `~/.agents/skills`. Use `--scope project` for `.agents/skills` in the current project, or `--dest` for another location.

| Skill | Use it for |
|---|---|
| `raven-onboarding` | Guided first-session setup and Raven concepts |
| `raven-core` | Creating, reading, editing, and organizing content |
| `raven-query` | RQL, saved queries, search, and link traversal |
| `raven-schema` | Schema design and migration |
| `raven-maintenance` | Vault checks, reindexing, and import |
| `raven-templates` | Template files and schema-template bindings |
| `raven-vault-admin` | Vault setup, selection, and configuration |

### 2. Let the agent onboard you

Open your agent wherever you want your notes to live and paste this:

> Use the `raven-onboarding` skill to set up my vault and walk me through how Raven works.

The onboarding skill sees that you have no vault, creates and registers one, then works in that vault: concepts, a schema for what you track, then creating and querying notes. After that, ask for real work:

> Add a meeting note for today's kickoff with Freya on the Midgard security review, capture any decisions and follow-ups, and link it to the project and to Freya.

### Optional: MCP

Skills teach CLI workflows. They do not need MCP. If your agent speaks MCP, install Raven into a supported client so it can call commands as tools:

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

Run `rvn skill install` again after upgrading `rvn` to install newly shipped skills, align existing Raven-managed skills, and remove receipt-managed skills this version no longer ships. For non-interactive use, apply the preview with `rvn skill install --confirm --json`.

See the [MCP reference](docs/agents/mcp.md), [Installation](docs/getting-started/installation.md), and [First vault](docs/getting-started/first-vault.md) guides for more setup.

## Doing it by hand

Same flow as above, tracking projects, meetings, and people, without an agent.

First, create a vault:

```bash
rvn init ~/notes
```

Raven creates:

```text
notes/
├── .raven/       # derived cache and local metadata (rebuildable with `rvn reindex`)
├── raven.yaml    # vault configuration
└── schema.yaml   # types, fields, and traits
```

On a fresh machine, this first `rvn init` also registers `~/notes` in global config and sets it as your default and active vault, so later commands can find it. Extra vaults get registered too, but you switch between them with `rvn vault use` / `rvn vault pin`. See [Vault creation and management](docs/getting-started/first-vault.md).

### 1. Extend the schema

Each file Raven indexes has a **type**. Custom types can require or allow specific **fields**. The starter schema gives you `project` and `person`. We'll add `meeting`.

Each meeting should record which project it belongs to, so the schema includes a `project` reference. The `todo` and `due` traits from the example are already in the starter schema.

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

…or make the same changes from the CLI:

```bash
rvn schema add type meeting --name-field title --default-path meeting/
rvn schema add field meeting project --type ref --target project
```

### 2. Create some notes

Create objects with the CLI. Each becomes an ordinary Markdown file under the directory for its type (`project/`, `person/`, …):

```bash
rvn new project "Midgard Security Review"
rvn new person "Freya"
rvn daily 2026-07-20
```

The `daily` command creates a built-in date note, so the `[[2026-07-20]]` reference in the meeting has a target. Now create `meeting/security-review-kickoff.md` as an ordinary Markdown file:

```markdown
---
type: meeting
title: Security review kickoff
project: project/midgard-security-review
---

Met with [[person/freya]] on [[2026-07-20]] to plan the security review. We agreed to focus the first pass on authentication and infrastructure, but work cannot begin until the draft scope is approved.

@todo @due(2026-07-22) Send the draft scope for review
```

> **A note on identifiers.** A note's canonical ID is usually `type/slug`, e.g. `project/midgard-security-review`. Built-in date notes use the bare ISO date. Frontmatter references use canonical IDs. Inside `[[links]]`, you can also use a shorter form like `[[freya]]` when it's unambiguous.

### 3. Query your vault

RQL finds notes and traits by structure. Text search is `rvn search`. For example:

```bash
# Open todos inside meetings that reference the Midgard project
rvn query 'trait:todo within(type:meeting refs([[project/midgard-security-review]]))'

# Items due by July 22 inside those same meetings
rvn query 'trait:due .value<=2026-07-22 within(type:meeting refs([[project/midgard-security-review]]))'
```

Read the first query as: *find `todo` traits that live within a `meeting` that references `project/midgard-security-review`.* Both queries return the task from our note:

```text
meeting/security-review-kickoff.md
  @todo @due(2026-07-22) Send the draft scope for review
```

References form a graph whether they point at a person, a project, or a date. Follow it with backlinks:

```bash
rvn backlinks person/freya
rvn backlinks 2026-07-20
```

```text
meeting/security-review-kickoff.md
  Met with [[person/freya]] on [[2026-07-20]] to plan the security review.
```

### 4. Ask your agent

With the vault populated, ask the question from the top of this README. The agent can run the same queries and backlink lookups you just used, then answer with the source note. See [Core concepts](docs/getting-started/core-concepts.md) and the [Query language](docs/querying/query-language.md) guide.

Raven also ships a language server for diagnostics, completion, navigation, and hover while you edit. See the [editor integration guide](docs/using-your-vault/editor-integration.md) to configure `rvn lsp`.

## Daily notes and quick capture

Daily notes are one file per day. Journaling, quick capture, meeting notes. Each is an ordinary `date`-typed note, so types, traits, references, and queries all apply.

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

Because daily notes are `date`-typed, the traits and references you capture stay queryable. Pull today's open todos, or review everything tied to a day with the read-only `rvn date` hub:

```bash
rvn query 'trait:todo within([[2026-07-20]])'   # todos captured that day
rvn date today                                  # everything connected to a date
```

You can give new daily notes a template, and change where they're stored via `directories.daily` in `raven.yaml`. See the [Daily notes](docs/using-your-vault/daily-notes.md) guide.

## Documentation

- [Installation](docs/getting-started/installation.md)
- [First vault](docs/getting-started/first-vault.md)
- [Core concepts](docs/getting-started/core-concepts.md)
- [Agent setup](docs/getting-started/agent-setup.md)
- [Daily notes](docs/using-your-vault/daily-notes.md)
- [Common commands](docs/using-your-vault/common-commands.md)
- [Configuration](docs/using-your-vault/configuration.md)
- [Schema introduction](docs/types-and-traits/schema-intro.md)
- [Schema reference](docs/types-and-traits/schema.md)
- [File format](docs/types-and-traits/file-format.md)
- [References](docs/types-and-traits/references.md)
- [Templates](docs/types-and-traits/templates.md)
- [Query language](docs/querying/query-language.md)
- [Bulk operations](docs/vault-management/bulk-operations.md)
- [Import](docs/vault-management/import.md)
- [MCP reference](docs/agents/mcp.md)

You can also browse the docs from the CLI:

```bash
rvn docs
```

## License

Raven is released under the [MIT License](LICENSE).
