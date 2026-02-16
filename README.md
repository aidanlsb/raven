<p align="center">
  <img src="raven.svg" alt="Raven" width="180" />
</p>

<h1 align="center">Raven</h1>

<p align="center">
  <strong>A plain-text knowledge base built for agent collaboration.</strong><br>
  <em>Early and experimental</em>
</p>

---

Raven keeps Markdown files as the source of truth, then adds:
- schema for structure and validation
- query language for retrieval
- CLI and MCP for agent workflows

## Start Here: First Loop (5 minutes)

This is the fastest path to first success:
1. create a vault
2. add structured information
3. query it successfully

### Prerequisites

- Go 1.22+ installed ([Install Go](https://go.dev/doc/install))
- A text editor on your machine (`code`, `cursor`, `vim`, etc.)

### 1) Install and verify

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
rvn version
```

Success check: `rvn version` prints version/build info.

### 2) Create a vault

```bash
rvn init ~/notes
cd ~/notes
```

Success check: you now have `schema.yaml`, `raven.yaml`, and `.raven/`.

### 3) Add structure

```bash
rvn new project "Onboarding"
rvn add "Planning [[projects/onboarding]] @highlight"
```

What this does:
- creates a typed project object (`projects/onboarding`)
- appends a structured note (reference + trait) to today's daily note

### 4) Query across your vault

```bash
rvn query 'trait:highlight refs([[projects/onboarding]])'
```

Success check: at least one result appears from your daily note.

If you get no results, run `rvn reindex` once and re-run the query.

### 5) Activated

You have completed the first loop:
- note captured
- structure applied (`[[reference]]` + `@trait`)
- query returned expected data

## Next Step: Connect Your Agent

Now that the first loop works, set up Raven for agent-assisted workflows:
- [MCP setup guide](docs/reference/mcp.md)

Suggested first prompt after MCP setup:

```text
"Summarize my current onboarding project and list open highlights or tasks that reference it."
```

---

## Documentation Map

Use guides in this order:
1. [Getting Started](docs/guide/getting-started.md) - first-session flow and verification
2. [Configuration Guide](docs/guide/configuration.md) - `config.toml` and `raven.yaml`
3. [Schema Introduction](docs/guide/schema-intro.md) - practical `schema.yaml` basics
4. [CLI Basics](docs/guide/cli-basics.md) - everyday commands
5. [CLI Advanced](docs/guide/cli-advanced.md) - bulk operations and power workflows

Use references when you already know what you need:
- [CLI Reference](docs/reference/cli.md)
- [Query Language](docs/reference/query-language.md)
- [raven.yaml Reference](docs/reference/vault-config.md)
- [schema.yaml Reference](docs/reference/schema.md)

---

## What Raven Is Optimized For

- Markdown-first knowledge with explicit structure
- Long-lived notes that remain useful with or without AI tools
- Agent workflows constrained by your schema and references

## Contributing

See:
- [AGENTS.md](AGENTS.md) for project architecture and contribution workflow
- [docs/](docs/) for user-facing and design documentation

