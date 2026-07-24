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
standard. The primary first-run path is `rvn skill install`, which installs the
shipped skills in one command:

```bash
rvn skill install          # interactive: prints the plan and prompts [y/N]
rvn skill install --yes    # agents / CI: apply without prompting
```

In an interactive terminal `rvn skill install` prints the plan (what will be
installed or updated) and prompts `Install these skills? [y/N]` before writing.

In non-interactive or `--json` runs it never prompts — agents cannot answer a
`y/N` prompt. Pass `--yes` to apply; without it the command returns a preview
and sets `needs_confirm: true`. The preview lists each skill with its planned
action (`install`, `update`, or `up to date`) so an agent can tell whether a
confirm is still required. After `--yes`, the response reports
`mode: "applied"` and the number of file changes made:

```bash
rvn skill install --json          # Preview only — needs_confirm: true
rvn skill install --yes --json    # Installs all shipped skills
```

By default it installs every shipped skill. Pass one or more names to narrow
the install:

```bash
rvn skill install raven-core raven-query --yes --json
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
rvn skill install --scope project --yes --json
```

That writes to `.agents/skills`. Use `--dest /path/to/skills` with any skill
command when an agent needs a different location.

### Updating already-installed skills

`rvn skill install` is for first-time installation. To refresh skills that are
already installed, use `rvn skill sync`:

```bash
rvn skill sync --confirm --json
```

The no-name sync updates or removes existing Raven-managed skills and reports
shipped skills that are not installed. It does **not** install missing skills
unless you name one explicitly — use `rvn skill install` to install missing
skills, or `rvn skill sync <name> --confirm` to install/realign a specific
skill.

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
3. Paste the recommended first prompt below. A vault is optional — if you don't
   have one yet, the agent will help you create it.

The agent detects your vault state, creates a first vault with `rvn init` when
none exists (asking where to put it), sets your editor with `rvn config set
editor=<cmd> editor_mode=auto` (asking first, since it is machine-wide), then
asks what you're trying to keep track of and proposes a small, personalized
schema in plain English before applying it. Once you agree, it seeds that schema
with your real projects, people, and tasks and teaches create, query, and check
against your own data. It can also point you at Raven's built-in LSP (`rvn lsp`,
see `using-your-vault/editor-integration.md`) for in-editor diagnostics and
completion. MCP is optional throughout.

## Creating a new vault with an agent

If an agent needs to create a vault, have it run `rvn init <path> --json` (or the `init`
MCP tool). Init applies Raven's first-run vault policy:

- The new vault is auto-registered in global config.
- If it is the first vault on the machine, it is also set as the default and active vault,
  so later commands resolve to it automatically.
- If another vault already exists, init registers and activates the new vault while leaving
  the existing default unchanged.

Agents should read the `post_init` object in the response:

- On the first vault, `is_first_vault` is `true`, `post_init.actions` is empty, and routing is
  already applied — the agent can proceed immediately, no further setup needed.
- When another vault already exists, `activated=true` and `active_vault` identify the new
  target. `previous_active_vault` / `previous_vault` and `switch_back` disclose the routing
  change and exact restore command. The agent should surface that switch before continuing;
  changing the default remains a separate explicit choice.

## Recommended first prompt

After installing the onboarding skill, a good first prompt is:

> Use the raven-onboarding skill to help me set up Raven. Ask me what I'm trying to keep track of, then propose a small schema (a few types) in plain English before changing anything. Once I agree, set it up and seed it with my real projects, people, and tasks, then show me one query, one backlinks lookup, and one check against my own data, explaining each step.

That prompt keeps the agent intent-first: it designs a small model around your goals — asking before it applies any schema change — and validates the setup end-to-end against your real content instead of running a generic feature tour.

## What a healthy setup looks like

Your agent should be able to:

- inspect the schema and reuse what's already there
- propose a small, personalized schema from what you're trying to track — and ask before applying it
- create a typed item through Raven instead of direct file writes
- query the vault and explain what it found

If the agent can do those things, the integration is in good shape.

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
