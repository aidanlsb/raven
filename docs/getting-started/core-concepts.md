# Core Concepts

This page gives you the mental model for Raven. After reading it, you should understand the key building blocks — types, references, traits, sections, assets, daily notes, and queries — even if you don't yet know every syntax detail.

## Types & Objects

Every Markdown file in a Raven vault is an instance of a type; these instances are called "objects." You define types in `schema.yaml`. Raven ships with starter types you can modify or replace. Here is an example:

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

The `name_field` tells Raven which field to auto-populate from the positional title. Required fields with defaults are filled automatically — `status` has `default: active`, so you don't need to pass it explicitly.

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

`rvn check` reports issues like unknown fields, missing required data, broken references, missing assets, and schema mismatches.

### Built-in types

Raven has three built-in types that always exist:

| Type | Purpose | Created by |
|------|---------|------------|
| `page` | Fallback for files without a `type:` in frontmatter | Any markdown file |
| `section` | Represents headings inside files | Automatic from markdown structure |
| `date` | Daily notes | `rvn daily` |

Built-in types cannot be redefined. Your custom types (`project`, `meeting`, `person`, etc.) provide the domain model on top of this foundation. See `types-and-traits/schema.md` for the full reference.

### Embedded types

You can turn a heading into a typed item without splitting it into a separate file by adding a `::type(...)` declaration on the line immediately after the heading:

```markdown
## Weekly Standup
::meeting(time=09:00, attendees=[[[person/freya]], [[person/thor]]])

Meeting notes here...
```

This creates an embedded `meeting` object within the file. See `types-and-traits/file-format.md` for the full syntax.

## References

References connect objects, sections, and assets into a graph. Wiki-style links are the Raven-native syntax for objects and sections:

```markdown
Met with [[person/freya]] about [[project/website]].
See the tasks: [[project/website#tasks]]
```

References also appear in frontmatter `ref` fields (`owner: person/freya`) and in embedded type declarations.

Normal Markdown links/images to vault-local non-Markdown files are asset references:

```markdown
Read [the paper](assets/pdfs/paper.pdf).
![Diagram](assets/photos/system.png)
```

Raven resolves references to canonical IDs. Short references like `[[freya]]` and `[[paper]]` work when unambiguous. Use `rvn backlinks` to see what links to an item:

```bash
rvn backlinks person/freya
rvn backlinks assets/pdfs/paper.pdf
```

See `types-and-traits/references.md` for the full reference guide.

## Assets

Assets are vault-local non-Markdown files such as PDFs, images, audio, videos, and datasets. They are first-class graph resources, but they are not schema object types.

Asset behavior is configured in `raven.yaml`, not `schema.yaml`:

```yaml
assets:
  root: assets/
  kinds:
    pdf:
      extensions: [pdf]
      default_path: pdfs/
```

Asset kinds are organization and validation rules. If you need authored metadata about an asset, create a Markdown object that references it.

## Traits

Traits are inline annotations that add structured, queryable metadata to your content:

```markdown
- @due(2026-02-15) Finish homepage design
- @priority(high) Review pull request
- @todo Refactor the auth module
- @highlight Key insight about the architecture
```

Traits must be defined in `schema.yaml` to be indexed and queryable. They can have typed values (date, enum, string, boolean) and participate in queries:

```bash
rvn query 'trait:due .value<today'
rvn query 'trait:todo within(type:project .status==active)'
```

See `types-and-traits/file-format.md` for trait syntax and `types-and-traits/schema.md` for defining traits.

## Headings & Sections

Every markdown heading automatically creates a `section` object. This gives your content hierarchy that Raven can query:

```markdown
# Website Redesign        → section (level 1)
## Tasks                  → section (level 2), child of above
### High Priority         → section (level 3), child of Tasks
```

Section objects can be referenced (`[[project/website#tasks]]`) and queried with hierarchy predicates like `parent(...)`, `ancestor(...)`, and `descendant(...)`.

## Daily Notes

Daily notes give you a date-stamped file for each day:

```bash
rvn daily                              # Today's note
rvn daily yesterday                    # Yesterday's
rvn add "@todo Review PR"              # Capture to today's note
```

Daily notes are `date`-typed items. They support templates, structured headings, and all the same query/trait features as any other item. See `using-your-vault/daily-notes.md` for the full guide.

## Queries

Raven Query Language (RQL) lets you retrieve objects and traits by structure, not just text:

```bash
# All active projects
rvn query 'type:project .status==active'

# Todos linked to a specific project
rvn query 'trait:todo within(type:meeting refs(midgard-security-review))'

# Overdue items
rvn query 'trait:due .value<today'
```

Queries return either objects or traits, can nest arbitrarily, and support boolean composition (`AND`, `OR`, `NOT`). See `querying/query-language.md` for the full syntax.

## Agent-friendly descriptions

Add optional `description` text to types and fields in `schema.yaml` to give context to both humans and agents:

```yaml
types:
  experiment:
    description: Controlled product change with hypothesis and measured outcome
    fields:
      hypothesis:
        type: string
        description: Falsifiable statement of expected behavior change
```

Good descriptions focus on intent and constraints, not just repeating the field name.

## Where to go next

| Goal | Read |
|------|------|
| Set up an AI agent | `getting-started/agent-setup.md` |
| Work with daily notes | `using-your-vault/daily-notes.md` |
| Organize files like PDFs and images | `using-your-vault/assets.md` |
| Learn everyday commands | `using-your-vault/common-commands.md` |
| Design your schema | `types-and-traits/schema-intro.md` |
| Understand file format details | `types-and-traits/file-format.md` |
| Learn the query language | `querying/query-language.md` |
| Configure your vault | `using-your-vault/configuration.md` |
