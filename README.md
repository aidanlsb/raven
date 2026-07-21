<h1 align="center">Raven</h1>

<p align="center"><strong>A CLI for plain-text knowledge management, with first-class support for AI agents.</strong></p>

Raven combines three properties so you retain full ownership of your knowledge while agents can work with it reliably:

- **Enforced schema.** Raven validates your files against the types, fields, and traits you define, keeping their structure consistent over time.
- **Deterministic queries.** Agents retrieve information using explicit, repeatable criteria rather than relying on fuzzy search or interpretation.
- **Plain-text source of truth.** Everything is Markdown with YAML frontmatter. The SQLite index under `.raven/` is only a rebuildable cache.

Here's an ordinary Raven note:

```markdown
---
type: meeting
title: Security review kickoff
project: project/midgard-security-review
---

Met with [[person/freya]] on [[2026-07-20]] to plan the security review. We agreed to focus the first pass on authentication and infrastructure, but work cannot begin until the draft scope is approved.

@todo @due(2026-07-22) Send the draft scope for review
```

The frontmatter is schema-validated; the traits and references are deterministically queryable.

Now ask your agent:

> Summarize what is blocking the Midgard security review, list its open follow-ups, and point me to the source notes.

> The review is blocked until the draft scope is approved. One follow-up is open: send the draft scope for review by July 22. Source: `meeting/security-review-kickoff.md`.

The rest of this README shows how to get there.

## Mental model

Five terms cover almost everything in Raven:

| Term | What it is |
|------|------------|
| **Vault** | The folder that holds your notes (plus `raven.yaml`, `schema.yaml`, and the `.raven/` cache). |
| **Type** | What a file represents — `project`, `meeting`, `person`. Custom types are defined in `schema.yaml`; `page`, `date`, and `section` are built in. |
| **Field** | Structured data in a note's YAML frontmatter (e.g. a person's `email`, a meeting's `project`). |
| **Trait** | An inline, structured tag on a line of content, like `@todo` or `@due(2026-07-22)`. Queryable, unlike a plain hashtag. |
| **Reference** | A link to another note, written as a canonical ID in frontmatter or a `[[wiki-style/link]]` in content. References form a graph you can traverse with backlinks and queries. |

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

## Get started with an agent

The fastest way to learn Raven is to let an agent set up your vault and teach you — this is where Raven shines. It ships reusable [Agent Skills](https://agentskills.io/) that give coding agents everything they need to drive your vault.

### 1. Install the skills

```bash
rvn skill install          # interactive: prints the plan and prompts [y/N]
rvn skill install --yes    # agents / CI: apply without prompting
```

This installs all shipped skills to `~/.agents/skills` (use `--scope project` for `.agents/skills` in the current project, or `--dest` for another location). Pass skill names to narrow it, e.g. `rvn skill install raven-core raven-query`. In non-interactive or `--json` runs it returns a preview unless you pass `--yes`.

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

Open your coding agent in the vault directory and give it this prompt:

> Use the `raven-onboarding` skill to set up my vault and walk me through how Raven works.

The onboarding skill introduces Raven's concepts, helps you shape a schema for what you actually track, and gets you creating and querying notes — all in the context of your own vault. From there you can ask for real work, like:

> Add a meeting note for today's kickoff with Freya on the Midgard security review, capture any decisions and follow-ups, and link it to the project and to Freya.

### Optional: MCP

Skills teach CLI workflows and don't require MCP. To additionally give MCP-native agents direct tool access to Raven's commands, install Raven into a supported client:

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

To update already-installed Raven-managed skills later, use `rvn skill sync` (see `rvn skill --help`).

See the full [MCP reference](docs/agents/mcp.md), [Installation](docs/getting-started/installation.md), and [First Vault](docs/getting-started/first-vault.md) guides for more setup details.

## Doing it by hand

Prefer to drive Raven directly, or curious what the agent does under the hood? Here's the same flow — tracking projects, meetings, and the people involved — done manually.

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

…or make the same changes from the CLI (or just ask your agent):

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

The `daily` command creates a built-in date note, giving the `[[2026-07-20]]` reference in our meeting a target. Now create `meeting/security-review-kickoff.md` as an ordinary Markdown file:

```markdown
---
type: meeting
title: Security review kickoff
project: project/midgard-security-review
---

Met with [[person/freya]] on [[2026-07-20]] to plan the security review. We agreed to focus the first pass on authentication and infrastructure, but work cannot begin until the draft scope is approved.

@todo @due(2026-07-22) Send the draft scope for review
```

> **A note on identifiers.** A note's canonical ID is usually `type/slug`, e.g. `project/midgard-security-review`; built-in date notes use the bare ISO date. Frontmatter references use canonical IDs. Inside `[[links]]`, you can also use a shorter form like `[[freya]]` when it's unambiguous.

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
meeting/security-review-kickoff.md
  @todo @due(2026-07-22) Send the draft scope for review
```

References form a graph regardless of whether they point to a person, project, or date. Follow it with backlinks:

```bash
rvn backlinks person/freya
rvn backlinks 2026-07-20
```

```text
meeting/security-review-kickoff.md
  Met with [[person/freya]] on [[2026-07-20]] to plan the security review.
```

### 4. Ask your agent

With the vault populated, ask the question from the top of this README. The agent can run the same deterministic queries and backlink lookups you just used, then answer with the source note. See [Core Concepts](docs/getting-started/core-concepts.md) and the [Query Language](docs/querying/query-language.md) guide to go deeper.

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
