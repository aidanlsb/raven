# Agent setup

Raven is a CLI. Agents drive the same commands through skills, and optionally through MCP.

This guide starts with Raven's packaged Agent Skills, then covers optional MCP
setup for clients that support it.

## What to set up

There are two distinct layers:

- Agent Skills that teach agents Raven-specific workflows
- MCP server setup that lets compatible agents call Raven tools directly

The skills teach CLI-first workflows and work with agents that can run `rvn`
from a shell. MCP is not required. It is a structured tool interface for clients
that speak it.

## Install Raven skills

Raven packages skills using the open [Agent Skills](https://agentskills.io/)
standard. `rvn skill install` is the command for installing and aligning
the packaged catalog:

```bash
rvn skill install                   # interactive: prints the plan and prompts [y/N]
rvn skill install --confirm --json  # agents / CI: apply without prompting
```

In an interactive terminal `rvn skill install` prints the plan (what will be
installed, updated, or removed) and prompts before writing.

In non-interactive or `--json` runs it never prompts. Agents cannot answer a
`y/N` prompt. Pass `--confirm` to apply. Without it the command returns a
preview and sets `needs_confirm: true`. The preview lists each skill with its
planned action (`install`, `update`, `remove`, or `up to date`) so an agent can
tell whether confirmation is still required. After confirmation, the response
reports `mode: "applied"` and the number of file changes made:

```bash
rvn skill install --json          # Preview only. needs_confirm: true
rvn skill install --confirm --json
```

With no names, install reconciles the full Raven-managed set to the catalog
shipped by the current `rvn` version:

- installs shipped skills that are missing
- replaces existing Raven-managed skills with the shipped version
- removes old Raven-managed skills that are no longer shipped

Removal is receipt-based. Raven removes only files recorded in its receipt and
leaves other files and non-Raven skill directories untouched.

Pass one or more names to install or align only those shipped skills. A named
install does not remove unrelated old Raven-managed skills:

```bash
rvn skill install raven-core raven-query --confirm --json
```

You can also list the catalog first:

```bash
rvn skill list --json
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

User-scoped skills install to `~/.agents/skills`. To install into the current
project instead:

```bash
rvn skill install --scope project --confirm --json
```

That writes to `.agents/skills`. Use `--dest /path/to/skills` with any skill
command when an agent needs a different location.

### Keeping packaged skills current

Run the same install command after upgrading `rvn`:

```bash
rvn skill install --confirm --json
```

The no-name form reconciles the entire catalog, including adding newly shipped
skills and removing receipt-managed skills no longer shipped. To update only
specific packaged skills, name them explicitly.

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

## From zero: the happy path

You do not need a vault before involving an agent. The onboarding skill can set
up Raven from scratch:

1. Install the CLI and install the skills with `rvn skill install` (above).
2. Open your agent.
3. Paste the [recommended first prompt](#recommended-first-prompt). A vault is
   optional. If you don't have one yet, the agent will help you create it.

The agent detects your vault state, creates a first vault with `rvn init` when
none exists (asking where to put it), sets your editor with `rvn config set
editor=<cmd> editor_mode=auto` (asking first, since it is machine-wide), then
asks what you're trying to keep track of and proposes a small, personalized
schema in plain English before applying it. Once you agree, it seeds that schema
with your real projects, people, and tasks and teaches create, query, and check
against your own data. It can also point you at Raven's built-in LSP (`rvn lsp`;
see [Editor integration](../using-your-vault/editor-integration.md)) for
in-editor diagnostics and
completion. MCP is optional throughout.

## Creating a new vault with an agent

If an agent needs to create a vault, have it run `rvn init <path> --json` (or the `init`
MCP tool). Init applies Raven's first-run vault policy:

- The new vault is auto-registered in global config.
- If it is the first vault on the machine, it is also set as the default and active vault,
  so later CLI commands resolve to it automatically.
- If another vault already exists, init registers and activates the new vault while leaving
  the existing default unchanged.

Agents should read the `post_init` object in the response:

- On the first vault, `is_first_vault` is `true`, `post_init.actions` is empty, and CLI
  routing is already applied.
- When another vault already exists, `activated=true` and `active_vault` identify the new
  target. `previous_active_vault` / `previous_vault` and `switch_back` disclose the routing
  change and exact restore command. The agent should surface that switch before continuing;
  changing the default remains a separate explicit choice.

MCP ignores both `active_vault` and `default_vault`. After `init`
over MCP, invoke `vault_focus` with the new name/path, pass `vault`/`vault_path`
on each vault-scoped call, or restart the server with a launch pin. CLI-based
agents can use the active/default routing described above.

## Recommended first prompt

After installing the onboarding skill, a good first prompt is:

> Use the raven-onboarding skill to help me set up Raven. Ask me what I'm trying to keep track of, then propose a small schema (a few types) in plain English before changing anything. Once I agree, set it up and seed it with my real projects, people, and tasks, then show me one query, one backlinks lookup, and one check against my own data, explaining each step.

That prompt keeps the agent intent-first. It designs a small model around your goals, asks before it applies any schema change, then runs a query, a backlinks lookup, and a check against your own notes instead of a generic feature tour.

## What a healthy setup looks like

Your agent should be able to:

- inspect the schema and reuse what's already there
- propose a small, personalized schema from what you're trying to track, and ask before applying it
- create a typed item through Raven instead of direct file writes
- query the vault and explain what it found

If the agent can do those things, the integration is in good shape.

## Common mistakes

- Installing MCP without passing the intended vault path
- Forgetting to check `rvn mcp status`
- Assuming skills grant tool access when the agent has neither shell nor MCP access
- Letting the agent write files directly instead of using Raven commands where Raven already has a safe primitive

## Where to go deeper

- Read the [MCP reference](../agents/mcp.md) for the compact MCP contract.
- Read [Common commands](../using-your-vault/common-commands.md) for the
  command set agents can invoke.
- Read [Core concepts](core-concepts.md) if terms like object, trait, or
  reference are unfamiliar.
- Read [Schema introduction](../types-and-traits/schema-intro.md) before asking
  an agent to make major schema changes.
- Use the [documentation map](documentation-map.md) to choose another topic.
