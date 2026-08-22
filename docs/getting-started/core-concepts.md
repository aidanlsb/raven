# Core concepts

This page is the mental model. Types, references, traits, sections, file links, daily notes, queries. Syntax lives in the linked pages.

## Types and objects

Every Markdown file in a Raven vault is an instance of a type. Those instances are called objects. You define types in `schema.yaml`. Raven ships with starter types you can modify or replace. Here is an example:

```yaml
types:
  project:
    default_path: project/
    fields:
      name:
        required: true
        type: string
      company:
        target: company
        type: ref
      status:
        default: active
        required: true
        type: enum
        values:
            - backlog
            - active
            - paused
            - done
    name_field: name
```

This defines a `project` type with a name, status (both required), and an optional company reference. Create objects via the CLI:

```bash
rvn new project "Midgard Security Review"
```

This creates a file in `project/` with frontmatter populated from the schema:

```markdown
---
type: project
name: Midgard Security Review
status: active
---

```

`name_field` is the field Raven fills from the positional title. Required fields with defaults are filled too. Here `status` has `default: active`, so you don't pass it.

### Validation

Raven validates at three levels:

| Command | What it checks |
|---------|---------------|
| `rvn new` | Validates while creating the object |
| `rvn schema validate` | Checks `schema.yaml` for internal consistency |
| `rvn check` | Validates existing vault content against the schema |

```bash
rvn check                                # Entire vault
rvn check project/midgard-security-review  # One object
```

`rvn check` reports issues like unknown fields, missing required data, broken
references, broken file links, and schema mismatches.

### Built-in types

Raven has three built-in types that always exist:

| Type | Purpose | Created by |
|------|---------|------------|
| `page` | Fallback for files without a `type:` in frontmatter | Any markdown file |
| `section` | Represents headings inside files | Automatic from markdown structure |
| `date` | Daily notes | `rvn daily` |

You cannot redefine built-in types. Custom types (`project`, `meeting`, `person`) sit on top of them. See `types-and-traits/schema.md` for the full reference.

## References

References connect objects and sections into a graph. Wiki-style links are the
Raven-native syntax:

```markdown
Met with [[person/freya]] about [[project/website]].
See the tasks: [[project/website#tasks]]
```

References also appear in frontmatter `ref` fields (`owner: person/freya`).

Normal Markdown links/images point to non-Markdown files:

```markdown
Read [the paper](../files/paper.pdf).
![Diagram](../files/system.png)
```

Author references with canonical object IDs. After `rvn new`, `rvn upsert`, or
`rvn daily`, use the returned
`data.id` in JSON output (or the human CLI's `link as <id>` hint). Bare short
forms still resolve when unambiguous, but they are resolution sugar and can
become ambiguous as the vault grows. Use `rvn backlinks` to see what links to an
item:

```bash
rvn backlinks person/freya
```

See `types-and-traits/references.md` for the full reference guide.

## File links

Non-Markdown files such as PDFs and images are ordinary files in the vault, not
Raven entities. Copy them into the vault directly, run `rvn reindex`, and link
them with standard Markdown. Raven indexes the outgoing link edges, reports
`broken_file_link`, and rewrites inbound file links when `rvn move` relocates a
file. See `using-your-vault/file-links.md`.

## Traits

Traits are inline annotations that add structured, queryable metadata to a line:

```markdown
- @due(2026-02-15) Finish homepage design
- @priority(high) Review pull request
- @todo Refactor the auth module
- @highlight Key insight about the architecture
```

A trait must be defined in `schema.yaml` to be indexed and queryable. Values can be typed (date, enum, string, boolean):

```bash
rvn query 'trait:due .value<today'
rvn query 'trait:todo within(type:project .status==active)'
```

See `types-and-traits/file-format.md` for trait syntax and `types-and-traits/schema.md` for defining traits.

## Headings and sections

Every markdown heading automatically creates a `section` object. Raven can query that hierarchy:

```markdown
# Website Redesign        → section (level 1)
## Tasks                  → section (level 2), child of above
### High Priority         → section (level 3), child of Tasks
```

Sections can be referenced (`[[project/website#tasks]]`) and queried with scope predicates like `in(...)`, `within(...)`, `has(...)`, and `contains(...)`.

Manage heading lifecycle explicitly:

```bash
rvn section create project/website "Tasks" --level 2
rvn section move project/website#tasks --after project/website#overview
rvn section rename project/website#tasks "Completed Tasks"
rvn section delete project/website#completed-tasks          # Preview
rvn section delete project/website#completed-tasks --confirm
```

Create requires plain title text plus an explicit level. Move preserves the
heading's identity and carries its complete subtree. Rename changes the
heading-derived slug and rewrites inbound fragment references. Delete previews
the exact subtree and affected inbound references by default, then removes the
subtree only with `--confirm`; reported references are left for explicit repair.
`rvn add` only appends body content and rejects Markdown headings.

## Daily notes

Daily notes are one file per day:

```bash
rvn daily                              # Today's note
rvn daily yesterday                    # Yesterday's
rvn add "@todo Review PR"              # Capture to today's note
```

Daily notes are `date`-typed items. They support templates, structured headings, and the same query/trait features as any other item. See `using-your-vault/daily-notes.md`.

Their canonical object ID is always the bare ISO date (`YYYY-MM-DD`), regardless
of `directories.daily`; author links as `[[2026-03-15]]`.

## Queries

RQL finds objects and traits by structure. Use `rvn search` when you only have a word to look for:

```bash
# All active projects
rvn query 'type:project .status==active'

# Todos linked to a specific project
rvn query 'trait:todo within(type:meeting refs([[project/midgard-security-review]]))'

# Overdue items
rvn query 'trait:due .value<today'
```

A query returns exactly one result kind: objects, sections, traits, or
outgoing link edges. Queries can nest, and they compose with
`AND`, `OR`, and `NOT`. See
`querying/query-language.md` for the full syntax.

## Type and field descriptions

Add optional `description` text to types and fields in `schema.yaml`. Humans and agents both read it:

```yaml
types:
  experiment:
    description: Controlled product change with hypothesis and measured outcome
    fields:
      hypothesis:
        type: string
        description: Falsifiable statement of expected behavior change
```

Describe intent and constraints. Repeating the field name wastes the slot.

## Where to go next

| Goal | Read |
|------|------|
| Set up an AI agent | `getting-started/agent-setup.md` |
| Work with daily notes | `using-your-vault/daily-notes.md` |
| Link files like PDFs and images | `using-your-vault/file-links.md` |
| Learn everyday commands | `using-your-vault/common-commands.md` |
| Design your schema | `types-and-traits/schema-intro.md` |
| Understand file format details | `types-and-traits/file-format.md` |
| Learn the query language | `querying/query-language.md` |
| Configure your vault | `using-your-vault/configuration.md` |
