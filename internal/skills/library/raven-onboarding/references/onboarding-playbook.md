# Raven Onboarding Playbook

Use these flows as scripts for an **intent-first** first session: detect state, create a vault if needed, then design a small personalized schema *from a conversation about the user's work*, seed it with their real content, and teach the mechanics against that data. Narrate what each command proves, and pause before mutating config or schema.

The order that matters: **discover intent → propose a tiny schema in plain English → confirm → apply → seed real data → teach by doing → hand off.** Do not lead with "run schema / new / add." Those commands come after the design conversation.

## Detect vault state first

Always start here. Do not assume a vault already exists.

```bash
rvn vault list --json
```

Read the result:
- Empty `vaults` (or `meta.count` of `0`): there is no vault yet. Follow **New vault setup**.
- One or more entries: at least one vault exists. Target it, then run the **Discover intent** conversation against what they already have (see **Existing vault tour**). Use `active_vault`, `default_vault`, and `current_vault` from the same result to inspect routing.

## New vault setup

This is the primary path for a brand-new user. Ask for the intended path first (suggest a sensible default such as `~/notes` or `~/raven`), then initialize:

```bash
rvn init /path/to/vault --json
```

`rvn init` creates `raven.yaml`, `schema.yaml`, `.raven/`, and starter vault content, and applies the first-run vault policy. Let init own registration and default/active routing — read its `post_init` block before running any `vault` command.

Inspect `post_init` and act accordingly:
- `is_first_vault: true` — this was the first vault on the machine: it is registered, set as the default, and active, and `post_init.actions` is empty. It is ready to use; proceed directly.
- `already_registered: true` — the vault is registered; do **not** run `rvn vault add`.
- `is_default: true` — it is already the default; do **not** run `rvn vault pin`.
- `is_active: true` — it is already active; do **not** run `rvn vault use`.
- `is_first_vault: false` — another vault already existed, so init activated the new vault. Surface `active_vault`, `previous_active_vault` / `previous_vault`, and `switch_back`. Ask before changing the default.
- Any routing field `false` (or `post_init` absent on older builds) — offer to finish setup using the commands surfaced in `post_init.commands` / `post_init.next_steps`. Ask the user before setting a default or active vault, since that is machine-wide routing.

Different builds behave differently, so branch on the fields rather than assuming:
- First vault is registered and set as default/active automatically (`is_first_vault: true`, empty `post_init.actions`).
- A later vault is registered and activated automatically; report the switch and exact restore command. The default stays unchanged unless the user explicitly changes it.
- Older builds may register nothing in `--json` mode, in which case `post_init.commands.register_and_pin` and `post_init.commands.activate` show the exact commands to run once the user agrees.

The starter vault ships a small default schema — `person` and `project` types plus `todo`, `due`, and `priority` traits. Treat these as a starting point to extend during the design conversation, not as the final model. Do **not** install extra preset types on your own; the schema grows from what the user tells you.

If the vault is not yet the active vault, target it explicitly for everything below:

```bash
rvn schema --vault <name> --json          # or: --vault-path /path/to/vault
```

The active/default vault is machine config, not vault content.

## Set up the editor

Once a vault exists, help the user point Raven at their editor. Keep it short.

```bash
rvn config show --json                                    # inspect current settings; note $EDITOR if relevant
rvn config set editor=cursor editor_mode=auto --json  # ask first — this is machine-wide config
rvn config show --json                                    # confirm
```

- If `editor` is already set and the user is happy with it, skip the change.
- Ask which editor they use before writing config (common: `cursor`, `code`,
  `nvim`). Setting `editor_mode=auto` with `rvn config set` lets Raven decide
  terminal vs GUI launch.
- Changing the editor edits machine-wide `config.toml`, so ask first — same rule as default/active vault routing.

### LSP pointer (awareness only)

Raven ships a built-in LSP (`rvn lsp`) that any LSP-capable editor can use for
diagnostics, quick-fix code actions, completion, hover, go-to-definition
(including frontmatter ref values), and find-references over vault files:

```bash
rvn lsp   # LSP server over stdio; part of the rvn binary, nothing extra to install
```

Point the user at `docs/using-your-vault/editor-integration.md` (or the `rvn docs` equivalent) for editor-specific wiring. Do **not** auto-configure Neovim/VS Code plugins from onboarding — make the user aware and hand off. For deeper config and UI settings, use `raven-vault-admin`.

## Discover intent

This is the core of onboarding. Do it **before** proposing types or running any schema command. Ask about the user's real life and problems, not Raven's features.

Prompt bank (pick a couple, follow the thread):
- "What are you trying to keep track of that keeps slipping through the cracks?"
- "What did you do this week that you wish you'd written down?"
- "What do you look up or search for over and over?"
- "Who and what do you deal with regularly — people, clients, projects, papers, recipes, workouts?"
- "When something falls apart, what information do you wish you'd had?"

Listen for two things:

| You hear… | It's probably a… |
|-----------|------------------|
| a recurring **noun** (project, client, paper, meeting, book, workout) | candidate **type** |
| a recurring **verb / status** (follow up, review, decide, remember, due) | candidate **trait** or **field** |

Read posture from *how* they answer — do not ask them to pick a mode:

| Signal | Posture | How to respond |
|--------|---------|----------------|
| jargon, asks for the exact command/flag, "just set it up", moves fast | power | be concise; point at files/commands; minimal narration |
| asks "why", wants the difference, likes tradeoffs | curious | explain reasoning and tradeoffs; catch over-engineering |
| short answers, "you decide", "whatever's easiest" | low-effort | propose a default; ask one yes/no; do the work |

If intent is genuinely vague, offer one or two example shapes **to react to**, not to install:
- Work / delivery: `project`, `person`, `meeting` (+ `todo`, `due` traits).
- Research / reading: `source`, `note` (+ `highlight`, `todo` traits).
- Personal CRM: `person`, `interaction`.

Present these as "some people start with X; does anything there match how you work?" — never as canned presets to apply wholesale.

## Propose a tiny schema in plain English

Before touching `schema.yaml`, describe the model in words and get a "yes." Use a compact proposal like:

> Here's what I'd start with:
> - **project** — the things you're driving. Fields: `status` (active/paused/done), `owner`.
> - **person** — people you work with. (Reuse the built-in `person` type.)
> - **meeting** — notes from a conversation. Fields: `date`, and a link to the `project` and the `people` there.
>
> Tasks and due dates won't be types — they'll be `@todo` / `@due(...)` traits on the lines where they come up. Sound right, or should we cut or rename anything?

Rules for the proposal:
- **2–4 types to start.** Warn out loud against ten types on day one. A few well-chosen types beat a sprawling model the user abandons.
- Only add fields worth **filtering or grouping on**. Everything else stays prose.
- Name the reference topology in a sentence before adding `ref` fields (see **References: sketch the topology first**).
- Call out anything that should stay a plain page for now (see **When a page is enough**).
- Reuse defaults (`person`, `project`, `todo`, `due`, `priority`) instead of re-creating them.

## Field vs trait cheat sheet

Teach this once, clearly, while proposing the model. The decision rule: **Is this a fact about the whole file, or about one line among many in it?** Whole file → field. One line → trait.

| | Field | Trait |
|---|-------|-------|
| Scope | the whole object/file | one line of prose |
| How many per object | usually one | many |
| Where it lives | frontmatter (YAML) | inline in the body |
| Written as | `status: active` | `@todo`, `@due(2026-02-01)` |
| Set / updated with | `rvn set <reference> status=active --json` | `rvn add "@todo ..." --json` |
| Good for | status, owner, stage, date, category | tasks, decisions, highlights, priorities |
| Example | a project's `status`, a meeting's `date` | `- @todo(...) email the vendor` on a bullet |

Anti-pattern to avoid in a starter schema: giving one concept **both** homes — for example a `due` *field* and a `@due` *trait*. Pick one home per concept so queries stay unambiguous. Due dates that live on individual tasks are a trait; a single date that describes the whole object (a meeting's date) is a field.

## References: sketch the topology first

Before adding any `ref` field, say in one sentence what points at what. For example: "a meeting points at the people who were there and the project it's about; a project points at its owner."

- Use `ref` / `ref[]` fields for **structural** relationships you will query (meeting → project, project → owner).
- Use plain `[[wikilinks]]` in prose for **incidental** mentions.
- Author both structural fields and prose wikilinks with canonical object IDs. Use `data.id` returned by
  `new`/`upsert`/`daily`; in human output, copy the `link as <id>` value. Do not
  generate bare short forms, even though they may resolve when unambiguous.
- Prefer a few meaningful links over wiring everything to everything — ref spaghetti is hard to reason about and query later.
- `ref` and `ref[]` fields must specify a `--target` type.

## When a page is enough

Not everything needs a type. If a category has no fields worth querying and no repeated structure, keep it as a plain page or a daily-note entry.

- A capture bucket ("ideas", "random notes") can live in daily notes until a pattern emerges.
- Promote a bucket to a type only once you keep wanting the same fields on each instance, or you want to query across them.
- A type earns its place by being queried across many instances — otherwise it is probably a page.

## Apply the agreed schema

Only after the user has said yes to the plain-English design. Ask before mutating schema. Inspect first so you extend rather than duplicate:

```bash
rvn schema --json
rvn schema type person --json
```

Then apply exactly the agreed set. Worked example for the "project / person / meeting" design above:

```bash
rvn schema add type meeting --name-field title --default-path meeting/ --json
rvn schema add field project status --type enum --values active,paused,done --json
rvn schema add field project owner --type ref --target person --json
rvn schema add field meeting date --type date --json
rvn schema add field meeting project --type ref --target project --json
rvn schema add field meeting people --type ref[] --target person --json
rvn schema validate --json
rvn reindex --json
rvn check --json
```

If the user's line-level facts need a trait beyond the defaults (`todo` / `due` / `priority`) — for example a research workflow that highlights sources — define it once:

```bash
rvn schema add trait highlight --type bool --json
```

Explain, briefly:
- Types define files/objects; fields live in frontmatter and are validated by type.
- Traits are defined here but *written* inline in body text, not set as
  frontmatter; see [Seed real data](#seed-real-data-from-their-world).
- `ref` and `ref[]` fields must name a `--target` type.

If a target type does not exist, stop and ask whether to create it or cut the field. Keep the set to what the user agreed — do not add "nice to have" types on your own.

## Seed real data from their world

Use the user's **real** projects, people, and tasks. Do not invent "Demo User" / "Website Redesign" unless the user genuinely has nothing to offer yet. Substitute their names into the commands:

```bash
rvn new project "<a real project they named>" --json
rvn new person "<a real collaborator>" --json
rvn set project/<their-project-id> status=active --json
rvn read project/<their-project-id> --json
```

Then capture a real line-level fact as a trait, linked to a real object:

```bash
rvn daily --json
rvn add "@todo <a real next action> for [[project/<their-project-id>]]" --json
rvn add "@due(2026-02-01) <a real deadline they mentioned>" --json
```

Notes:
- `rvn new` auto-populates the type's name field from the title. Use
  `data.id` from its JSON response (or `link as <id>` in human output) for the
  follow-up `set`, `read`, and `[[reference]]` examples above. Use
  `--field name=value` for fields you already know.
- If required fields are missing, read the error details and retry with `--field` or `--fields-json`.
- `rvn add` appends to the configured capture destination, which defaults to today's daily note.

## Teach by doing — against their data

Keep every mechanic attached to the content you just seeded, not a separate generic tour.

Query what they created, and narrate the schema-to-query loop:

```bash
rvn query 'type:project' --count-only --json
rvn query 'type:project' --limit 10 --json
rvn query 'trait:todo' --limit 10 --json
rvn query 'trait:due .value<today' --limit 10 --json
rvn search '<a word from their real content>' --json
```

Show the reference graph without manual bookkeeping:

```bash
rvn resolve project/<their-project-id> --json
rvn backlinks project/<their-project-id> --json
rvn outlinks YYYY-MM-DD --json
```

Teach the two write homes concretely by contrast:
- Update a **field**: `rvn set project/<their-project-id> status=paused --json`.
- Mark a **line** with a **trait**: `rvn add "@todo review the plan" --json`.

Then verify health:

```bash
rvn check --json     # explain a clean result, or walk any issues
rvn reindex --json   # only if the index is stale
```

Use `raven-query` for deeper RQL examples once the user is comfortable with objects, traits, and references.

## Existing vault tour

When a vault already exists, stay intent-first: understand what's there, then ask what isn't working, then propose small additions.

```bash
rvn vault list --json
rvn vault list --path-only --json
rvn vault stats --json
rvn schema --json
rvn query 'type:project' --limit 5 --json
rvn query 'trait:todo' --limit 5 --json
```

Summarize what exists before proposing changes. Then run the **Discover intent** conversation framed around gaps: "What are you trying to do here that isn't working yet?" Propose a small, additive change (a field or trait, rarely a whole new type), confirm, and apply it the same way as a new vault. If the schema does not define `project` or `todo`, pick another existing type or trait from `rvn schema --json`.

## Config surfaces (reference)

Explain these only if the user asks or the setup calls for it:

```bash
rvn config show --json
rvn vault config show --json
```

- Global config tracks named vaults, default vault, editor, and Markdown style; `state.toml` always lives beside `config.toml`.
- Vault-local config in `raven.yaml` controls directories, capture settings, auto-reindex, deletion policy, protected prefixes, and exclude patterns.

## Handoff checklist

Before ending onboarding, make sure the user has:
- A clear picture of which vault is active and where it lives.
- A tiny schema they agreed to — 2–4 types and the traits that matter — described in their own words.
- Understood the field-vs-trait rule (whole file vs one line).
- At least one real object created through `rvn new` and one real line captured through `rvn add`.
- One `[[reference]]` plus a backlinks check against their data.
- One query against their content and a clean (or explained) `rvn check --json`.
- A next step for tomorrow: `rvn daily` to capture, `rvn add "@todo ..."` for tasks, `rvn query ...` to retrieve.

## Related guidance

- [Back to Raven Onboarding](../body.md)
- Canonical human setup path:
  `rvn docs getting-started first-vault --json`
- Agent and MCP setup:
  `rvn docs getting-started agent-setup --json`
