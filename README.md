<h1 align="center">Raven</h1>

**A CLI for plain-text knowledge management, with first-class support for AI agents.**

A Raven "vault" is a collection of markdown files, with a few additional capabilities:
- Typed notes: define a `project` type with required yaml frontmatter fields, specified in a schema 
- Traits for annotations: create a `@priority` trait in your schema that you can use to tag important notes
- References: link your notes together with `[[backlinks]]` to create a graph
- Queries: retrieve your notes precisely with a fully featured query language

# Getting Started
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

Then initialize a vault:

```bash
rvn init ~/notes
cd ~/notes
```

Raven creates:

```text
notes/
├── .raven/       # derived cache and local metadata
├── raven.yaml    # vault configuration
└── schema.yaml   # types, fields, and traits
```

## Agent Setup

Once you have a vault, connect Raven to your agent of choice.

### MCP Setup

Install Raven into a supported MCP client:

```bash
rvn mcp install --client claude-desktop
rvn mcp install --client claude-code
rvn mcp install --client cursor
rvn mcp status
```

Or print a manual config snippet with:

```bash
rvn mcp show
```

### Skill Installation

Raven also ships with a few skills for supported agent runtimes:

```bash
rvn skill list --target codex
rvn skill install raven-core --target codex --confirm
```

Available skill targets are `codex`, `claude`, and `cursor`.

### Agent Onboarding

After MCP and skills are in place, a good first prompt is:

> Help me onboard to Raven in this vault. Start by inspecting the schema, traits, and vault stats. Then walk me through one concrete create flow, one query, and one check, explaining each step as you go.

See the full [MCP reference](docs/agents/mcp.md), [Installation](docs/getting-started/installation.md), and [First Vault](docs/getting-started/first-vault.md) guides for more setup details.

## Example Usage

Let's say that you want to track people, projects, and meetings in your vault. These are "types." You might also want a quick way to tag when decisions get made, which is a good use case for "traits."

Raven's starter schema already gives you the `project` and `person` types (which you can modify), but `meeting` and `decision` do not yet exist. All types and traits in your vault are defined in `schema.yaml`. To add to your schema, you can edit `schema.yaml` directly, use the CLI, or ask an agent. We'll cover the first two here:

**Editing `schema.yaml`**
Add your new types under `types` and add the fields you want to track for meetings. Let's say for meetings you'll want to track which project they're associated with, who you met with, and any explicit decisions recorded in the notes. Traits are single valued, so you just need to define what sort of value the trait holds (e.g., `enum`, `boolean`, `date`, etc.) and optionally set a default. Boolean traits default to `true` when left bare so they're a good fit for things like `decision` where you just want to add a structured tag to some content.

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

**Use the CLI**

```bash
rvn schema add type meeting --name-field title --default-path meeting/
rvn schema add field meeting project --type ref --target project
rvn schema add field meeting with --type ref[] --target person
rvn schema add trait decision --type bool
```

Create new instances of these types ("objects") using the CLI:

```bash
rvn new project "Midgard Security Review" --field status=active
rvn new person "Freya" --field role=lead
```

Those commands create ordinary markdown files, saved to directories corresponding to the type (`project/` and `person/`).

Raven also has a built-in daily notes feature, which will create a new note for every day for jotting things down.

```bash
rvn daily
```
You can use the `add` command to append content to existing notes. By default `add` appends to the daily note, but you can use the `--to` argument to write to different files as well.

```
rvn add "Met with [[person/freya]] about [[project/midgard-security-review]]" --to today
rvn add "@todo Send the draft scope to [[person/freya]]" --to today
```


You can also create files manually. For example, to take notes for a meeting:

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
@priority(high)
@decision Keep the first pass focused on authentication and infrastructure.
```

Use the Raven query language to retrieve information from your vault:

```bash
rvn query 'trait:todo within(object:meeting refs(midgard-security-review))'
rvn query 'trait:decision within(object:meeting refs(midgard-security-review))'
```
Results:

```text
meeting/kickoff.md
  @todo Send the draft scope to [[person/freya]]
  @todo [[person/freya]] to confirm which systems are in scope for [[project/midgard-security-review]]

meeting/kickoff.md
  @decision Keep the first pass focused on authentication and infrastructure.
```

Trace everything connected to one person:

```bash
rvn backlinks person/freya
```

```text
meeting/kickoff.md
  [[person/freya]] wants the initial scope and timeline confirmed before the review begins

project/midgard-security-review.md
  Project lead: [[person/freya]]
```

Before the next leadership check-in, you can ask your agent for a briefing:

> Summarize what is blocking the Midgard security review, tell me who owns each follow-up, and point me to the source notes.

Because the agent can query Raven directly, it can answer from the project, the meeting note, the todo traits, and the backlinks instead of guessing from raw files:

> The review is waiting on scope confirmation before work begins. Two follow-ups are open from `meeting/kickoff.md`: send the draft scope to Freya, and have Freya confirm which systems are in scope for `project/midgard-security-review`. The current decision on record is to keep the first pass focused on authentication and infrastructure.

That is the core value: Raven keeps the source of truth as markdown, but gives both you and your agent a precise retrieval layer on top of it.

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
