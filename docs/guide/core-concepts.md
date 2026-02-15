# Core concepts

Raven is plain markdown plus a few structured conventions:

- **Types**: what an object *is* (e.g., `person`, `project`) — defined in `schema.yaml`
- **Objects**: instances of types (a file, or an embedded object under a heading)
- **Traits**: inline annotations on content (e.g., `@due(2026-01-10)`)
- **References**: wiki-style links between objects (e.g., `[[people/freya]]`)

## Files are objects

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

## Embedded (typed) objects

If the line immediately after a heading is a `::type(...)` declaration, that heading becomes an object of that type instead of a `section`.

```markdown
## Weekly Standup
::meeting(time=09:00, attendees=[[[people/freya]]])
```

See `reference/file-format.md` for the exact rules and ID generation.

## Traits

Traits are inline, single-valued annotations:

```markdown
- @due(2026-01-10) Send proposal to [[clients/midgard]]
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

## Querying

Raven queries are type-constrained:
- `object:<type> ...` returns objects
- `trait:<name> ...` returns trait instances

See `reference/query-language.md` for the full language.

## Next steps

- See `cli.md` for common CLI patterns and workflows
- See `reference/schema.md` for defining types and traits
- See `reference/file-format.md` for the complete file format specification

