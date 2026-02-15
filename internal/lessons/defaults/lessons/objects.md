---
title: Objects and Types
docs:
  - docs/guide/core-concepts.md#files-are-objects
  - docs/reference/file-format.md#frontmatter
  - docs/reference/schema.md#type-definitions
---

# Objects and Types

Raven stores knowledge as plain Markdown files. Each file can have a `type` in
frontmatter, and schema types help you keep structure consistent over time.

## Example

```md
---
type: person
name: Alice
---

# Alice
```

## Try It

Create a person:

```bash
rvn new person "Alice"
```
