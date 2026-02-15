# Core Concepts

**Raven** is a plain-markdown knowledge system with:
- **Types**: Schema definitions for what things are (e.g., `person`, `project`, `book`) — defined in `schema.yaml`
- **Objects**: Instances of types — each file declares its type in frontmatter (e.g., `people/freya.md` is an object of type `person`)
- **Traits**: Inline annotations on content (`@due`, `@priority`, `@highlight`)
- **References**: Wiki-style links between notes (`[[people/freya]]`)
- **Schema**: User-defined in `schema.yaml` — types and traits must be defined here to be queryable

Type and field definitions can include optional `description` values in `schema.yaml`. Use `raven_schema`/`raven_schema type <name>` to read them and ground tool decisions in the user's own terminology.

**Built-in types:**

- `page`: freeform note objects (use this when a new note doesn’t fit a schema type)
- `date`: daily notes
- `section`: embedded objects inside a file

### File Format Quick Reference

**Frontmatter** (YAML at top of file):
```markdown
---
type: project
status: active
owner: "[[people/freya]]"
---
```

**Embedded objects** (typed heading):
```markdown
## Weekly Standup
:::meeting(time=09:00, attendees=[[[people/freya]]])
```

**Traits** (inline annotations):
```markdown
- @due(2026-01-15) Send proposal to [[clients/acme]]
- @priority(high) Review the API design
- @highlight This insight is important
```

**References** (wiki-style links):
```markdown
[[people/freya]]              Full path
[[freya]]                     Short reference (if unambiguous)
[[people/freya|Freya]]        With display text
```

### Reference Resolution

When using object IDs in tool calls:
- **Full path**: `people/freya` — always works
- **Short reference**: `freya` — works if unambiguous (only one object matches)
- **With .md extension**: `people/freya.md` — also works

If a short reference is ambiguous, Raven returns an `ambiguous_reference` error listing all matches. Ask the user which one they meant, or use the full path.
