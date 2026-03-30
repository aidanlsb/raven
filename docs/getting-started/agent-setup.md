# Agent Setup

Raven is designed to work well both as a CLI and as a tool-backed system for local AI agents.

This guide covers the fastest way to connect Raven to an MCP-capable client and the basic skill setup that helps agents behave well in a vault.

## What to set up

There are two distinct layers:

- MCP server setup so the agent can call Raven tools
- optional skill installation so the agent starts with better Raven-specific instructions

You can use MCP without installing skills, but the experience is better with both.

## Install MCP into a supported client

Examples:

```bash
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
```

If you run the server manually, use:

```bash
rvn serve --vault-path /path/to/vault
```

## Install Raven skills

List available skills for your target runtime:

```bash
rvn skill list --target codex --json
rvn skill list --target claude --json
rvn skill list --target cursor --json
```

Install the core Raven skill:

```bash
rvn skill install raven-core --target codex --confirm --json
```

Available targets are `codex`, `claude`, and `cursor`.

## Recommended first prompt

After MCP and skills are installed, a good first prompt is:

> Help me onboard to Raven in this vault. Start by inspecting the schema, traits, and vault stats. Then walk me through one concrete create flow, one query, and one check, explaining each step as you go.

That prompt forces the agent to inspect the actual vault before making changes and gives you a quick end-to-end validation of the setup.

## What a healthy setup looks like

Your agent should be able to:

- inspect the schema
- list or read saved queries and workflows
- create a typed object through Raven instead of direct file writes
- query the vault and explain what it found

If the agent can do those four things, the integration is in good shape.

## Common mistakes

- Installing MCP without passing the intended vault path
- Forgetting to check `rvn mcp status`
- Expecting skills alone to replace MCP tool access
- Letting the agent write files directly instead of using Raven commands where Raven already has a safe primitive

## Where to go deeper

- Read `agents/mcp.md` for the compact MCP contract
- Read `using-your-vault/common-commands.md` for the full command surface agents can invoke
- Read `getting-started/core-concepts.md` if the agent is using terms like object, trait, or reference before you are comfortable with them
- Read `types-and-traits/schema-intro.md` before asking an agent to make major schema changes
