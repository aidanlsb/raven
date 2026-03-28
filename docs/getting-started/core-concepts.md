# Core Concepts

## Types & Objects
Every file in a Raven vault is an instance of a type; these instances are referred to as "objects." You define the types that you want to use in your vault in the `schema.yaml` file that is created with vault initialization. Raven ships with a couple example types already defined, which you can modify/delete/replace. Here is an example of what a type definition looks like in the schema:

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

This defines a `project` type that has a name and status field (both required) and an optional field to track what company the project is associated with. Once a type is defined in the schema, the best way to create objects (files) of that type is via the CLI: `rvn new project "Midgard Security Review"`. Running this command will create a new file in the `project/` directory. The `name_field` tells Raven which string field should be auto-populated from the positional title you pass to `rvn new`. The created file looks like this:

```markdown
---
type: project
name: Midgard Security Review
status: active
---

```

Raven checks required fields during object creation.

- If a required field is satisfied by `name_field`, you do not need to pass it separately.
- If a required field has a schema default, Raven fills that value in automatically.
- If a required field has no default and you did not provide it, `rvn new` fails and tells you what is missing.

In the example schema above, `status` is marked `required: true` but also has `default: active`, so:

```bash
rvn new project "Midgard Security Review"
```

is valid even without `--field status=active`, because the default satisfies the requirement.

### How to validate an object against the schema

There are two different validation questions:

1. Is the schema itself valid?
2. Does a specific object conform to that schema?

Use:

```bash
rvn schema validate
```

to validate `schema.yaml` itself.

Use:

```bash
rvn check
rvn check project/midgard-security-review
```

to validate vault content against the schema. `rvn check` is what tells you about issues in objects such as unknown fields, missing required data, bad references, or other content/schema mismatches.

In practice:
- `rvn new` validates while creating the object
- `rvn schema validate` validates the schema definition
- `rvn check` validates existing content against the schema

### Embedded Types

An embedded type lets you turn a heading inside a markdown file into a typed object without splitting it into its own file. This is useful when one file contains several structured sub-items, such as multiple meetings inside a project note or multiple tasks inside a daily note.

The syntax is:
- write a markdown heading
- put a `::type(...)` declaration on the very next line
- optionally pass fields inside the parentheses

Example:

```markdown
# Project Notes

## Weekly Standup
::meeting(id=standup, time=09:00, attendees=[[[people/freya]], [[people/thor]]])

Reviewed blockers and confirmed the rollout plan.
```

In this example, `## Weekly Standup` is not treated as a plain `section`. Raven indexes it as an embedded object of type `meeting`, with the body content under that heading belonging to that object. If you omit `id=...`, Raven derives the embedded object's fragment ID from the heading.

### Built-in types

Raven has three built-in types that always exist, even if you never define them in `schema.yaml`:

- `page`
- `section`
- `date`

These are Raven-managed types with fixed definitions. You do not redefine them under `types:` in `schema.yaml`.

#### `page`

`page` is the fallback type for ordinary markdown files that do not declare an explicit `type:` in frontmatter.

Example:

```markdown
# Scratch note

This file has no frontmatter type, so Raven treats it as a `page`.
```

The built-in `page` type has a `title` field and uses `title` as its `name_field`.

In practice, `page` is useful for:
- durable notes that do not justify a custom type yet
- imported markdown
- scratch notes that you may later reclassify into a richer type

#### `section`

`section` is the built-in type Raven creates automatically for headings inside markdown files.

For a file like:

```markdown
# Website Redesign

## Tasks

- Draft kickoff agenda
```

Raven creates a section object for `# Website Redesign` and another for `## Tasks`.

The built-in `section` type has:
- `title`
- `level` (heading depth from 1 to 6)

You do not usually create `section` objects directly. Raven derives them from markdown structure so that headings can participate in references, hierarchy, and queries.

#### `date`

`date` is the built-in type Raven uses for daily notes.

When you work with daily-note flows such as:

```bash
rvn daily
```

the resulting object is treated as a `date`.

Unlike user-defined types, `date` is a Raven-managed core type. You use it when working with daily-note behavior, not when designing your own schema for things like projects or meetings.

### Important constraint

Built-in types are always available and Raven overwrites their definitions internally to keep them consistent. That means:

- you cannot redefine `page`, `section`, or `date` under `types:`
- you should create your own types for domain concepts like `project`, `meeting`, or `person`
- built-in types provide Raven's structural foundation, while your schema provides the domain model

-- stop --

A typed file uses YAML frontmatter:

```markdown
---
type: project
status: active
---

# Website Redesign
```

If there’s no frontmatter `type:`, the file is treated as type `page`.

## Headings create structure

Every markdown heading creates a **section object** automatically. This provides hierarchy for:
- where traits/refs “belong”
- `#fragment` references like `[[projects/website#tasks]]`
- hierarchical queries (`parent(...)`, `ancestor(...)`, `encloses(...)`, etc.)

## Traits

Traits are inline, single-valued annotations:

```markdown
- @due(2026-01-10) Send proposal 
- @highlight Buffer time is the key to good estimates
```

Traits must be defined in `schema.yaml` to be indexed/queryable.

## Agent-friendly descriptions

Add optional `description` text to types and fields in `schema.yaml` to give extra context to both humans and agents.

Good descriptions focus on intent and constraints, not just repeating the field name.

```yaml
types:
  experiment:
    description: Controlled product change with hypothesis and measured outcome
    fields:
      hypothesis:
        type: string
        description: Falsifiable statement of expected behavior change
      run_date:
        type: date
        description: Planned launch date (YYYY-MM-DD)
```


