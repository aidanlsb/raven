# Onboarding

Use this guide when a user asks you to help them learn Raven or onboard to their vault.

The goal is a hands-on walkthrough that teaches Raven's model through real actions in the user's vault — not a lecture. Every step should use the user's actual schema and data, not hardcoded examples.

## Step 1: Inspect the vault

Start by understanding what the user has.

```text
raven_invoke(command="schema", args={"subcommand":"types"})
raven_invoke(command="schema", args={"subcommand":"traits"})
raven_invoke(command="vault_stats")
raven_invoke(command="query_saved_list")
```

**Tell the user:** Summarize what you found — the types they have (e.g., "Your vault has project, person, and meeting types"), the traits available (e.g., "You have @todo, @due, and @priority for inline annotations"), and how many objects are in the vault. This gives the user a mental map before you start doing things.

## Step 2: Branch on vault state

Use the vault stats to decide which path to take.

### Path A: Empty or near-empty vault

If the vault has few or no objects, walk the user through creating their first content.

#### Create one object

Pick a type from the schema that has a `name_field` — this is the most natural starting point. Use a title that's relevant to the user's context, not a generic placeholder.

```text
raven_invoke(command="new", args={"type":"<type>", "title":"<relevant title>"})
```

**Tell the user:** Explain what just happened — Raven created a markdown file, populated the frontmatter from the schema (including any defaults), and indexed it. Point out the file path and the object ID.

#### Add body content with a trait

```text
raven_invoke(command="add", args={"text":"@todo Review the initial setup", "to":"<file from previous step>"})
```

**Tell the user:** Explain that `@todo` is a trait — a structured annotation that Raven indexes and makes queryable. It's not just text; it's data you can filter and act on.

#### Query for the trait

```text
raven_invoke(command="query", args={"query_string":"trait:todo"})
```

**Tell the user:** This is the core loop — you write naturally in markdown, use traits to mark structured data, and Raven lets you retrieve it later by structure, not just text search. Show how the query returned the trait with its containing object and line content.

#### Run a health check

```text
raven_invoke(command="check")
```

**Tell the user:** `check` validates the entire vault against the schema — missing fields, broken references, undefined traits. A clean check means the vault is healthy. Walk through any issues found and explain what they mean.

### Path B: Populated vault

If the vault already has content, focus on showing the user what Raven can do with their existing data.

#### Show what's in the vault

Summarize the types and their counts from `vault_stats`. Then run a query against their most populated type:

```text
raven_invoke(command="query", args={"query_string":"type:<most-populated-type>", "limit":5})
```

**Tell the user:** Walk through the results — what fields each object has, how the schema shapes the data. Point out any patterns.

#### Demonstrate the reference graph

Pick an object that likely has connections (meeting notes, project pages) and show its backlinks:

```text
raven_invoke(command="backlinks", args={"target":"<well-connected object ID>"})
```

**Tell the user:** Backlinks show everything that links *to* this object — you don't have to maintain these manually. Raven builds the graph from wiki-link references in your content and frontmatter ref fields.

#### Show trait queries

If the vault has traits in use, run a meaningful query:

```text
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo"})
```

Or for date-based traits:

```text
raven_invoke(command="query", args={"query_string":"trait:due .value<today"})
```

**Tell the user:** Queries let you pull structured data out of your notes by combining type filters, field values, traits, and reference relationships. This is what makes Raven different from just searching file contents.

#### Run a health check

```text
raven_invoke(command="check")
```

**Tell the user:** Walk through the results. If there are issues, explain the most important ones and offer to help fix them. If the vault is clean, say so — it means the content and schema are well-aligned.

## Step 3: Ensure docs are available

Check whether the user has local docs fetched so `rvn docs` works:

```text
raven_invoke(command="docs_list")
```

If this fails with `NOT_FOUND`, fetch docs:

```text
raven_invoke(command="docs_fetch")
```

**Tell the user:** Raven has built-in documentation you can browse with `rvn docs`. These are now available locally in your vault.

## Step 4: Wrap up

Summarize what the user learned and point them to next steps based on what they seemed most interested in.

**Possible next directions to suggest:**

- Daily notes and quick capture: "Try `rvn daily` to create today's note, then `rvn add` to capture thoughts quickly"
- Schema customization: "You can add new types and fields — see `raven://guide/core-concepts`"
- Querying power: "The query language can do a lot more — see `raven://guide/querying` and `raven://guide/query-cheatsheet`"
- Bulk operations: "For working with many objects at once, see `raven://guide/key-flows`"
- Templates: "You can set up templates for consistent note structure"

## Narration principles

- **Show, don't lecture.** Every explanation should follow a concrete action the user can see.
- **Use their data.** Never say "Website Redesign" if they have real projects. Never say "person/freya" if they have real people.
- **Name the concepts.** When you create something, say "this is an object of type X." When you query, say "this is a trait query." Build vocabulary through use.
- **Connect the dots.** After creating and querying, explicitly connect: "You wrote `@todo` in markdown, and Raven made it queryable — that's the schema-to-query loop."
- **Keep it interactive.** Pause after each major step. Ask if the user wants to try something or explore further before moving on.
