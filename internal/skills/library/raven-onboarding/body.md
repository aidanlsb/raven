# Raven Onboarding

Use this skill when a user wants to learn Raven, set up their first vault, or have an agent help them start using Raven. It works end-to-end from a clean machine with **no vault yet**, as well as for users who already have a vault.

A good first session produces a **small, personalized setup the user actually wants** — a handful of types that match what they are trying to track, seeded with their real content — **not** a generic tour of Raven's features. Design the model *from a conversation about their work*, propose it in plain English, and only then run commands. Prefer this wizard-style dialogue over pushing canned presets; offer example shapes only as soft suggestions when their intent is genuinely vague.

This skill is CLI-first. Use MCP as a fallback when CLI access is unavailable, preserving the same JSON expectations.

## Operating rules

- Begin by detecting state, not by assuming a vault exists. A brand-new user may have zero vaults, no default, and no active vault — that is a normal starting point, not an error.
- Lead with intent, not mechanics. Ask what the user is trying to keep track of before naming a single Raven feature, type, or command.
- Design before you write. Propose a tiny schema in plain English and get a "yes" before running any `rvn schema` command.
- Keep the starter schema small — 2–4 types, only what their stated intent needs. Actively resist type proliferation.
- Read the user's posture from how they respond (power / curious / low-effort) and adjust your pace; never ask them to pick a "mode."
- Seed and demonstrate with the user's real world — their projects, people, and tasks — not placeholder "Demo User" data, unless they genuinely have nothing to offer yet.
- Use `rvn ... --json` for all commands so results are deterministic.
- Prefer Raven commands over direct file edits for onboarding.
- Ask before changing schema, global config, or default/active vault routing.
- Let `rvn init` own vault registration and default/active routing. Read its `post_init` output and do not blindly re-run `vault add` / `vault use` when init already handled them.

## First-session flow

1. **Detect vault state.** Run `rvn vault list --json` and read the result:
   - Empty `vaults` (or `meta.count` of `0`) means there is no vault yet — follow the new-vault path below.
   - One or more entries means at least one vault exists — target that vault, then still run the intent conversation (steps 4+) against what they already have. Use `active_vault` / `default_vault` to see whether one is already selected.
   - There is no need to run `rvn vault current --json` first when no vault exists; detection above already covers the empty state.
2. **If there is no vault yet, create one (primary path):**
   - Ask where to create it. Suggest a sensible default such as `~/notes` or `~/raven`, but let the user choose.
   - Run `rvn init <path> --json`.
   - Read `post_init` from the result and honor what init already did:
     - If `is_first_vault` is `true`, this was the first vault on the machine: it is registered, set as default, and active, and `post_init.actions` is empty — proceed directly, no further routing needed.
     - If `already_registered` is `true`, the vault is registered — do not run `rvn vault add` again.
     - If `is_default` is `true` and/or `is_active` is `true`, routing is already set — do not run `rvn vault pin` / `rvn vault use` for those.
     - If `is_first_vault` is `false` with `needs_user_choice_for_activate` / `needs_user_choice_for_default` set to `true`, another vault already exists; `post_init.actions` lists the `activate` / `set_default` actions — ask the user before running them.
     - If init did **not** register or route the vault (fields are `false`, or `post_init` is absent on older builds), offer to finish setup using the commands in `post_init.commands` / `post_init.next_steps`. Ask before setting a default or active vault, since routing is machine-wide config.
   - After init, work against this vault (pass `--vault <name>` or `--vault-path <path>` if it is not yet the active vault).
3. **Set up the editor (once a vault exists).** Keep this short — it is one onboarding step, not an editor deep-dive.
   - Inspect current settings with `rvn config show --json` (and note `$EDITOR` if it is relevant to the user's choice).
   - If `editor` is unset, or the user wants to change it, ask which editor they use (common: `cursor`, `code`, `nvim`).
   - Apply with `rvn config set --editor <cmd> --editor-mode auto --json`. Ask before changing machine-wide config, consistent with the rule about global/default vault routing.
   - Confirm with `rvn config show --json`.
   - **LSP pointer (awareness only).** Mention that Raven ships a built-in LSP (`rvn lsp`) that gives diagnostics, completion, go-to-definition, and find-references in any editor with an LSP client, and point at `docs/using-your-vault/editor-integration.md` (or the `rvn docs` equivalent). Do not auto-configure nvim/vscode plugins here — make the user aware and hand off.
4. **Discover intent.** This is the heart of the session — do it before touching the schema.
   - Ask about their world, not Raven's features: "What are you trying to keep track of?", "What keeps slipping through the cracks?", "What do you look up or write down over and over?"
   - Listen for the nouns (projects, clients, papers, meetings, recipes, workouts) and the verbs (follow up, review, decide, remember). The recurring nouns are candidate types; the recurring verbs are candidate traits.
   - Read their posture from how they answer and adapt (see **Read posture from behavior**). Do not ask them to choose a mode.
   - If their intent is vague, offer one or two concrete example shapes to react to ("some people track projects + people + meetings; others track sources + notes") — as soft prompts, not presets to install.
5. **Propose a tiny schema in plain English.** Before running anything, describe the model in words and get agreement (see **Schema design rules**).
   - Name 2–4 candidate types tied directly to what they said, and say what each is for in one line.
   - For each type, list only the handful of fields worth filtering on, and call out which line-level facts should be traits instead (see the field-vs-trait rule).
   - Sketch the reference topology in a sentence ("meetings point at the people who were there and the project they're about") before adding any `ref` fields.
   - Flag anything that does not need a type yet — capture buckets can stay plain pages or daily notes until a pattern emerges.
   - Warn, out loud, against starting with ten types. A few well-chosen types beat a sprawling model they will abandon.
6. **Confirm, then apply the schema.** Only after the user agrees to the plain-English design:
   - Inspect what already exists so you extend rather than duplicate: `rvn schema --json` (and `rvn schema type <name> --json` for detail).
   - Reuse the defaults: a fresh vault already ships `person` and `project` types plus `todo` / `due` / `priority` traits — extend those instead of re-creating them.
   - Apply the agreed set with `rvn schema add type ... --json`, `rvn schema add field ... --json`, and `rvn schema add trait ... --json` (or edit `schema.yaml` directly for larger changes).
   - Validate with `rvn schema validate --json`, then run `rvn reindex --json` and `rvn check --json` if a change touched the index.
7. **Seed real data from their world.** Put their actual content in, not placeholders.
   - Create real objects: `rvn new <type> "<a real thing they named>" --json` (add `--field name=value` for a field you already know).
   - Capture real line-level facts as traits: `rvn add "@todo <a real next action> for [[<type>/<id>]]" --json`.
   - Only fall back to a placeholder like "Demo User" if the user truly cannot name anything real yet.
8. **Teach by doing — against their data.** Keep mechanics attached to their content instead of a separate feature tour.
   - Query what you just seeded: `rvn query 'type:<their type>' --json` and `rvn query 'trait:todo' --json`. Narrate that this is the schema-to-query loop — they wrote Markdown, and Raven made it retrievable by structure.
   - Show the graph: `rvn backlinks <their object id> --json` (and `rvn outlinks <id> --json`) so they see references without maintaining them by hand.
   - Contrast the two write homes concretely: update a field with `rvn set <id> status=... --json`; mark a line with a trait via `rvn add "@todo ..." --json`.
   - Run `rvn check --json` and explain a clean result (or walk any issues), using `rvn reindex --json` only if the index is stale.
9. **Handoff.** Close with a short, concrete summary:
   - Which vault is active and where it lives.
   - The types and traits you set up and what each is for, in their words.
   - How to continue tomorrow: `rvn daily --json` to capture, `rvn add "@todo ..." --json` for tasks, `rvn query ...` to pull things back out.
   - Point at `raven-core`, `raven-query`, and `raven-schema` for going deeper.

## Read posture from behavior

Infer the user's working style from how they respond and match it. Never ask "which mode do you want?" — just adapt.

- **Power user** (uses jargon, asks for the exact command or flag, says "just set it up", moves fast): be concise, point straight at files and commands, skip the hand-holding.
- **Curious user** (asks "why", wants the difference between things, likes tradeoffs): explain the reasoning, name the tradeoffs, and actively catch over-engineering before it happens.
- **Low-effort user** (short answers, "you decide", "whatever's easiest"): propose a concrete default, ask a single yes/no question, and do the work for them.

Watch for the posture to shift mid-session — a curious user may go low-effort once they trust you, and a low-effort user may get curious once they see results.

## Schema design rules

Teach these once, clearly, while proposing the model — not as an abstract lecture.

### Field vs trait

- **Field** = a fact about the **whole object**. Stable, usually one value per object, lives in frontmatter. Examples: a project's `status`, a project's `owner`, a meeting's `date`.
- **Trait** = a fact about **one line** of prose. Many per object, lives inline in the body, written `@name` or `@name(value)`. Examples: `@todo` on a bullet, `@due(2026-02-01)` on a task.
- Decision rule: *Is this about the whole file, or about one line among many in it?* Whole file → field. One line → trait.
- Update a field with `rvn set <id> field=value --json`; add a trait by writing a line with `rvn add "@trait ..." --json`.
- Avoid giving one concept both homes in a starter schema (for example, a `due` **field** *and* a `@due` **trait**). Pick one home per concept so queries stay unambiguous.

### Keep it small

- Start with 2–4 types. Add more only when the user hits a real, repeated need.
- A type earns its place when you want to query across many instances of it or store the same fields on each. Otherwise it is probably a page.

### References

- Sketch the topology first: say, in words, what points at what ("meetings point at people and projects").
- Add `ref` / `ref[]` fields only for structural relationships you will query; use plain `[[wikilinks]]` in prose for incidental mentions.
- Prefer a few meaningful links over wiring everything to everything — ref spaghetti is hard to reason about later.

### When a page is enough

- Not everything needs a type. If a category has no fields worth querying and no repeated structure, keep it as a plain page or a daily-note entry.
- Promote a bucket to a type only once a pattern emerges — you keep wanting the same fields, or you want to query across the instances.

## Teaching points

- Everything durable lives in plain Markdown the user owns; `schema.yaml` only tells Raven which parts to treat as structured, queryable data, and `.raven/` is a rebuildable cache.
- Types describe whole objects/files (projects, people, meetings, notes, books, issues). Fields are their frontmatter properties.
- Traits are inline annotations in body text — tasks, decisions, priorities, highlights — one line at a time.
- References are `[[id]]` links to objects or sections; use exact resolved IDs when automation depends on them.
- Daily notes are a built-in capture workflow, not a replacement for typed project or meeting objects.

## Cross-references

- Use `raven-vault-admin` for vault setup, active/default vault selection, and deeper config changes (editor, UI, and other machine-level settings).
- Use `raven-schema` when the agreed design needs new types, fields, or traits, or a later migration.
- Use `raven-core` for creating objects, adding notes, editing content, daily notes, and references.
- Use `raven-query` for search, structured query examples, and saved queries.
- Use `raven-maintenance` for `rvn check`, `rvn check fix`, and reindexing.

## Load references as needed

- Intent-discovery prompts, schema-proposal templates, the field-vs-trait cheat sheet, and seeding scripts: `references/onboarding-playbook.md`
