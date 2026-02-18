<p align="center">
  <img src="raven.svg" alt="Raven" width="180" />
</p>

<h1 align="center">Raven</h1>

<p align="center">
  <strong>A plain-text knowledge base built for agent collaboration.</strong><br>
  <em>Early and experimental</em>
</p>

---

You keep notes in Markdown. The problem: they accumulate without structure. When you ask an AI assistant "what decisions did we make on the API project?", it can't actually look anything up — it guesses from whatever context you paste in.

Raven adds schema, a query language, and an MCP server to your Markdown vault. Your files stay readable and portable. But now you — and your agent — can query them.

## A working example

Say you're running a software project. You want to track decisions and action items inline as you take meeting notes, and be able to retrieve them later.

**Define what you care about**

```bash
rvn schema add trait decision --type bool
rvn schema add trait todo --type bool
```

**Create typed objects**

```bash
rvn new project "API Redesign" --field status=active
rvn new person "Sarah"
```

**Capture notes as you work**

In today's daily note, you capture what happened in your meeting:

```bash
rvn add "Move to GraphQL for faster client iteration @decision [[projects/api-redesign]] [[people/sarah]]"
rvn add "Prototype the search endpoint by Friday @todo @due(2026-02-21) [[projects/api-redesign]]"
```

These are just lines in a Markdown file. But Raven indexes the structure.

**Query across your vault**

```bash
# All decisions made on the API redesign
rvn query 'trait:decision refs([[projects/api-redesign]])'

# Open todos that are past their due date
rvn query 'trait:todo at(trait:due .value==past)'

# Every daily note that mentions Sarah
rvn query 'object:date refs([[people/sarah]])'
```

**Ask your agent**

Connect Raven via MCP and your agent runs the same queries against your actual vault:

```
"What decisions have been made on the API Redesign project, and are there any overdue tasks?"
```

The agent reads your schema, queries your notes, and gives you a grounded answer — not a guess.

---

## Why Markdown-first matters

Your notes live as `.md` files in a directory you control. They are readable without Raven. They open in any editor. The structure Raven adds — type frontmatter, inline traits like `@decision`, wikilink references like `[[projects/api-redesign]]` — stays embedded in your files.

You can query with Raven. You can also just open the file.

---

## Get started

### Prerequisites

- Go 1.22+ ([Install Go](https://go.dev/doc/install))

### Install

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
rvn version
```

### Create a vault

```bash
rvn init ~/notes
cd ~/notes
```

You now have `schema.yaml`, `raven.yaml`, and `.raven/`.

### First loop

```bash
rvn new project "Onboarding"
rvn add "Kickoff complete @highlight [[projects/onboarding]]"
rvn query 'trait:highlight refs([[projects/onboarding]])'
```

Success: at least one result appears. If not, run `rvn reindex` and retry.

### Connect your agent (MCP)

See the [MCP setup guide](docs/reference/mcp.md).

---

## Documentation

**Guides** (read in order):
1. [Getting Started](docs/guide/getting-started.md) — first-session flow and verification
2. [Configuration](docs/guide/configuration.md) — `config.toml` and `raven.yaml`
3. [Schema Introduction](docs/guide/schema-intro.md) — practical `schema.yaml` basics
4. [CLI Basics](docs/guide/cli-basics.md) — everyday commands
5. [CLI Advanced](docs/guide/cli-advanced.md) — bulk operations and power workflows

**References** (when you know what you need):
- [CLI Reference](docs/reference/cli.md)
- [Query Language](docs/reference/query-language.md)
- [raven.yaml Reference](docs/reference/vault-config.md)
- [schema.yaml Reference](docs/reference/schema.md)

---

## Contributing

See [AGENTS.md](AGENTS.md) for project architecture and contribution workflow.
