<h1 align="center">Raven</h1>

<p align="center"><strong>A CLI for plain-text knowledge management, with first-class support for AI agents.</strong></p>

Raven turns a folder of Markdown files into a structured, queryable knowledge base — and gives AI agents the tools to read and maintain it for you. Your notes stay as plain `.md` files you fully own; Raven adds a lightweight schema, structured tags, and a query language on top.

## Why Raven?

If you already keep notes in Markdown, you've probably hit the ceiling of plain files: you can grep for text, but you can't ask "which follow-ups from my meetings are still open?" or "what's blocking this project?" without reading everything yourself.

Tools like Notion and Obsidian add structure, but your data lives in their format and their app. Raven takes a different approach:

- **Plain text is the source of truth.** Everything is Markdown with YAML frontmatter. No proprietary database, no lock-in. The index under `.raven/` is a derived cache you can rebuild any time with `rvn reindex`.
- **Structure you define.** A lightweight schema describes the things you track (projects, meetings, people…) so notes become queryable data, not just prose.
- **Agent-native.** Raven ships an MCP server so agents (Claude, Cursor, Codex, …) can query and update your vault through structured tools instead of blindly reading files — which makes their answers accurate and grounded in your notes.

Here's the payoff. After capturing a few notes the normal way, you can ask your agent:

> Summarize what is blocking the Midgard security review, tell me who owns each follow-up, and point me to the source notes.

Because the agent queries Raven directly, it answers from the actual project, meeting notes, todo tags, and links — not a fuzzy text search:

> The review is waiting on scope confirmation before work begins. Two follow-ups are open from `meeting/kickoff.md`: send the draft scope to Freya, and have Freya confirm which systems are in scope for `project/midgard-security-review`. The current decision on record is to keep the first pass focused on authentication and infrastructure.

The rest of this README shows how to get there.

## Mental model

Five terms cover almost everything in Raven:

| Term | What it is |
|------|------------|
| **Vault** | The folder that holds your notes (plus `raven.yaml`, `schema.yaml`, and the `.raven/` cache). |
| **Type** | What a file represents — `project`, `meeting`, `person`. Every note has one, defined in `schema.yaml`. |
| **Field** | Structured data in a note's YAML frontmatter (e.g. a project's `status`, a meeting's `with`). |
| **Trait** | An inline, structured tag on a line of content, like `@todo` or `@decision`. Queryable, unlike a plain hashtag. |
| **Reference** | A `[[wiki-style/link]]` between notes, forming a graph you can traverse with backlinks and queries. |

Keep these in mind and the rest of the docs will read easily.

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

Prebuilt binaries for Linux, macOS, and Windows are also available on the [releases page](https://github.com/aidanlsb/raven/releases/latest) — see [Installation](docs/getting-started/installation.md) for details.

Then create a vault:

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

On a fresh machine, this first `rvn init` also registers `~/notes` in global config
and sets it as your default and active vault, so the CLI can find it right away — no
extra setup needed. Additional vaults are registered too, but you switch between them
explicitly with `rvn vault use` / `rvn vault pin`. See
[Vault Creation & Management](docs/getting-started/first-vault.md).

The starter `schema.yaml` already includes `project` and `person` types, which you can modify or replace.

## Connect an agent

Raven's agent support is optional — the CLI works on its own — but it's where Raven shines. It ships reusable [Agent Skills](https://agentskills.io/) that teach coding agents how to work with your vault, plus an MCP server for agents that speak MCP.

### Skills

The fastest way to get started is to install the shipped skills:

```bash
rvn skill install          # interactive: prints the plan and prompts [y/N]
rvn skill install --yes    # agents / CI: apply without prompting
```

In an interactive terminal, `rvn skill install` prints what will be installed
and prompts before writing. In non-interactive or `--json` runs it does not
prompt: pass `--yes` to apply, otherwise it returns a preview and reports that
confirmation is required.

By default it installs all shipped skills; pass skill names to narrow it, e.g.
`rvn skill install raven-core raven-query`.

Skills install to `~/.agents/skills` by default. Use `--scope project` for
`.agents/skills` in the current project, or `--dest` to choose another
location.

| Skill | Use it for |
|---|---|
| `raven-onboarding` | Guided first-session setup and Raven concepts |
| `raven-core` | Creating, reading, editing, and organizing content |
| `raven-query` | RQL, saved queries, search, and link traversal |
| `raven-schema` | Schema design and migration |
| `raven-maintenance` | Vault checks, reindexing, and import |
| `raven-templates` | Template files and schema-template bindings |
| `raven-vault-admin` | Vault setup, selection, and configuration |

The packaged skills teach CLI workflows and do not require MCP. Use
`rvn skill install` for first-time installation. `rvn skill sync` is for
updating already-installed Raven-managed skills: without a skill name it only
updates/realigns existing ones and reports (but does not install) missing
skills.

### MCP server

MCP gives compatible agents direct access to Raven commands. Install Raven
into a supported MCP client:

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

### Agent onboarding

After installing the onboarding skill, ask your agent to use it to introduce
Raven in the context of your vault.

See the full [MCP reference](docs/agents/mcp.md), [Installation](docs/getting-started/installation.md), and [First Vault](docs/getting-started/first-vault.md) guides for more setup details.

## A guided walkthrough

Let's track projects, meetings, and the people involved, then let an agent answer questions about them.

### 1. Extend the schema

Every note has a **type** defined in `schema.yaml`, and each type can require or allow certain **fields**. The starter schema gives you `project` and `person`, but not `meeting` or `decision` — so we'll add them.

We want each meeting to record which project it belongs to and who attended, and we want a lightweight way to mark decisions. A project/person link is a **field** (a `ref`); a decision is a good fit for a **trait**, since it tags a specific line of content rather than the whole file. Traits hold a single typed value (`enum`, `boolean`, `date`, …); a boolean trait defaults to `true` when written bare, which is perfect for a plain `@decision` tag.

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
      with:
        type: ref[]
        target: person

traits:
  decision:
    type: boolean
```

…or make the same changes from the CLI (or just ask your agent):

```bash
rvn schema add type meeting --name-field title --default-path meeting/
rvn schema add field meeting project --type ref --target project
rvn schema add field meeting with --type ref[] --target person
rvn schema add trait decision --type bool
```

### 2. Create some notes

Create objects with the CLI. Each becomes an ordinary Markdown file under the directory for its type (`project/`, `person/`, …):

```bash
rvn new project "Midgard Security Review" --field status=active
rvn new person "Freya" --field role=lead
```

Raven also has built-in daily notes for quick capture. `add` appends to today's daily note by default; use `--to` to target another file:

```bash
rvn daily
rvn add "Met with [[person/freya]] about [[project/midgard-security-review]]" --to today
rvn add "@todo Send the draft scope to [[person/freya]]" --to today
```

Of course, you can also just write files by hand. Here's a meeting note:

```markdown
---
type: meeting
title: Kickoff
project: project/midgard-security-review
with:
  - person/freya
---

[[person/freya]] wants the initial scope and timeline confirmed before the review begins.

@todo Send the draft scope to [[person/freya]]
@todo [[person/freya]] to confirm which systems are in scope for [[project/midgard-security-review]]
@decision Keep the first pass focused on authentication and infrastructure.
```

> **A note on identifiers.** A note's canonical ID is `type/slug`, e.g. `project/midgard-security-review`. In frontmatter and `[[links]]` you can use that full ID, and inside `[[links]]` you can also use a shorter form like `[[freya]]` when it's unambiguous.

### 3. Query your vault

The Raven Query Language (RQL) retrieves notes and traits by structure, not just text. For example:

```bash
# Open todos inside meetings that reference the Midgard project
rvn query 'trait:todo within(type:meeting refs([[project/midgard-security-review]]))'

# Decisions recorded in those same meetings
rvn query 'trait:decision within(type:meeting refs([[project/midgard-security-review]]))'
```

Read the first query as: *find `todo` traits that live within a `meeting` that references `project/midgard-security-review`.* Results:

```text
meeting/kickoff.md
  @todo Send the draft scope to [[person/freya]]
  @todo [[person/freya]] to confirm which systems are in scope for [[project/midgard-security-review]]

meeting/kickoff.md
  @decision Keep the first pass focused on authentication and infrastructure.
```

To trace everything connected to one person, follow the reference graph with backlinks:

```bash
rvn backlinks person/freya
```

```text
meeting/kickoff.md
  [[person/freya]] wants the initial scope and timeline confirmed before the review begins

project/midgard-security-review.md
  Project lead: [[person/freya]]
```

### 4. Ask your agent

With the vault populated, the briefing from the top of this README just works — the agent runs the same queries and backlinks you just saw and answers from your structured notes rather than a raw text search. See [Core Concepts](docs/getting-started/core-concepts.md) and the [Query Language](docs/querying/query-language.md) guide to go deeper.

Raven also includes a built-in language server for diagnostics, completion,
navigation, and hover while you edit. See the
[editor integration guide](docs/using-your-vault/editor-integration.md) to
configure `rvn lsp` in your editor.

## Documentation

- [Installation](docs/getting-started/installation.md)
- [First Vault](docs/getting-started/first-vault.md)
- [Core Concepts](docs/getting-started/core-concepts.md)
- [Agent Setup](docs/getting-started/agent-setup.md)
- [Daily Notes](docs/using-your-vault/daily-notes.md)
- [Common Commands](docs/using-your-vault/common-commands.md)
- [Configuration](docs/using-your-vault/configuration.md)
- [Schema Introduction](docs/types-and-traits/schema-intro.md)
- [Schema Reference](docs/types-and-traits/schema.md)
- [File Format](docs/types-and-traits/file-format.md)
- [References](docs/types-and-traits/references.md)
- [Templates](docs/types-and-traits/templates.md)
- [Query Language](docs/querying/query-language.md)
- [Bulk Operations](docs/vault-management/bulk-operations.md)
- [Import](docs/vault-management/import.md)
- [MCP Reference](docs/agents/mcp.md)

You can also browse the docs from the CLI:

```bash
rvn docs
```

## License

Raven is released under the [MIT License](LICENSE).
