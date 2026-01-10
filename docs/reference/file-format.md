# File format (reference)

Raven files are plain markdown with optional YAML frontmatter and optional embedded type declarations.

## File-level object (frontmatter)

If present, frontmatter must be valid YAML between `---` markers at the start of the file:

```markdown
---
type: person
name: Freya
---

# Freya
```

- `type` selects the object type (defaults to `page` if omitted)
- other keys are treated as **fields** (validated by `schema.yaml`)

### Reserved keys

These keys are always allowed in frontmatter:
- `type`
- `tags`
- `id` (primarily relevant for embedded objects)

### `alias`

`alias` is a *special field name* used for reference resolution (so `[[alias]]` can resolve to an object).

Important: `alias` is **not reserved**; if you use it, add it as a field on the relevant type(s) in `schema.yaml` so `rvn check` doesn’t report it as an unknown key.

## Sections

Every markdown heading creates an object of type `section` (unless it’s overridden by an embedded type declaration).

- Section IDs are `file-id#slug`
- Section fields include:
  - `title` (heading text)
  - `level` (1–6)

## Embedded type declarations (`::type(...)`)

If the line **immediately after** a heading is a `::type(...)` declaration, that heading becomes an object of that type instead of a `section`.

```markdown
## Weekly Standup
::meeting(time=09:00, attendees=[[[people/freya]]])
```

Rules:
- The declaration must be on the **next line** after the heading.
- `::type()` arguments are `key=value` pairs (commas separate args).
- `id=...` is optional; if omitted, the heading text is slugified to make the `#fragment`.
- Object ID is `file-id#fragment` (e.g., `daily/2026-01-10#weekly-standup`).

## References (`[[...]]`)

```markdown
[[people/freya]]
[[people/freya|display text]]
[[projects/website#tasks]]
```

Short refs (e.g., `[[freya]]`) resolve if unambiguous; use full paths to avoid ambiguity.

