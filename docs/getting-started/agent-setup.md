# Agent Setup

Raven is designed to work well both as a CLI and as a tool-backed system for
local AI agents.

This guide starts with Raven's packaged Agent Skills, then covers optional MCP
setup for clients that support it.

## What to set up

There are two distinct layers:

- Agent Skills that teach agents Raven-specific workflows
- MCP server setup that lets compatible agents call Raven tools directly

The skills teach CLI-first workflows and work with agents that can run `rvn`
from a shell. MCP is not required, but it provides a structured tool interface.

## Install Raven skills

Raven packages skills using the open [Agent Skills](https://agentskills.io/)
standard. List the available skills and install the recommended starting set:

```bash
rvn skill list --json
rvn skill sync raven-onboarding --confirm --json
rvn skill sync raven-core --confirm --json
```

| Skill | Use it for |
|---|---|
| `raven-onboarding` | Guided first-session setup and Raven concepts |
| `raven-core` | Creating, reading, editing, and organizing content safely |
| `raven-query` | RQL, saved queries, search, and link traversal |
| `raven-schema` | Schema design and migration workflows |
| `raven-maintenance` | Vault checks, reindexing, and data import |
| `raven-templates` | Template files and schema-template bindings |
| `raven-vault-admin` | Vault setup, selection, and configuration |

User-scoped skills install to `~/.agents/skills`. To install a skill in the
current project instead:

```bash
rvn skill sync raven-core --scope project --confirm --json
```

That writes to `.agents/skills`. Use `--dest /path/to/skills` with any skill
command when an agent needs a different location.

To refresh already installed Raven-managed skills, run:

```bash
rvn skill sync --confirm --json
```

The no-name sync updates or removes existing Raven-managed skills and reports
shipped skills that are not installed. It does not install missing skills
unless you name one explicitly.

## Inspect and remove skills

`rvn skill doctor` shows the resolved install root and which Raven skills are
installed, which is useful when a skill does not seem to be picked up:

```bash
rvn skill doctor --json
rvn skill doctor --scope project --json
```

To uninstall a skill, use `rvn skill remove`. It previews by default and
applies with `--confirm`:

```bash
rvn skill remove raven-core --json          # Preview
rvn skill remove raven-core --confirm --json
```

## Install MCP into a supported client

Examples:

```bash
rvn mcp install --client codex --vault-path /path/to/vault
rvn mcp install --client claude-desktop --vault-path /path/to/vault
rvn mcp install --client claude-code --vault-path /path/to/vault
rvn mcp install --client cursor --vault-path /path/to/vault
```

Check the resulting config:

```bash
rvn mcp status
```

If you need the raw config snippet instead of direct installation:

```bash
rvn mcp show --vault-path /path/to/vault
rvn mcp show --client codex --vault-path /path/to/vault
```

If you run the server manually, use:

```bash
rvn serve --vault-path /path/to/vault
```

To remove Raven from a client config again:

```bash
rvn mcp remove --client codex
```

## Recommended first prompt

After installing the onboarding skill, a good first prompt is:

> Use the raven-onboarding skill to help me onboard to Raven in this vault. Start by inspecting the schema, traits, and vault stats. Then walk me through one concrete create flow, one query, and one check, explaining each step as you go.

That prompt forces the agent to inspect the actual vault before making changes and gives you a quick end-to-end validation of the setup.

## What a healthy setup looks like

Your agent should be able to:

- inspect the schema
- list or read saved queries
- create a typed item through Raven instead of direct file writes
- query the vault and explain what it found

If the agent can do those four things, the integration is in good shape.

## Common mistakes

- Installing MCP without passing the intended vault path
- Forgetting to check `rvn mcp status`
- Assuming skills grant tool access when the agent has neither shell nor MCP access
- Letting the agent write files directly instead of using Raven commands where Raven already has a safe primitive

## Where to go deeper

- Read `agents/mcp.md` for the compact MCP contract
- Read `using-your-vault/common-commands.md` for the full command surface agents can invoke
- Read `getting-started/core-concepts.md` if the agent is using terms like object, trait, or reference before you are comfortable with them
- Read `types-and-traits/schema-intro.md` before asking an agent to make major schema changes
