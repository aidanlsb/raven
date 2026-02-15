# Lessons Reference

Lessons provide a guided learning path. Canonical technical detail lives in the
guide/reference docs.

## Content Boundaries

- Docs are canonical for commands, syntax, and behavior.
- Lessons are curated onboarding and refresher paths.
- Lessons should summarize and direct users to canonical docs, not duplicate
  full reference content.

## Lesson Frontmatter

Lessons live in `internal/lessons/defaults/lessons/*.md` and use YAML
frontmatter:

```yaml
---
title: Objects and Types
prereqs:
  - refs
docs:
  - docs/guide/core-concepts.md#files-are-objects
  - docs/reference/file-format.md#frontmatter
---
```

Fields:

- `title` (required): lesson display title
- `prereqs` (optional): advisory prerequisite lesson IDs
- `docs` (optional): ordered canonical doc links for further reading

## Manual Bidirectional Mapping

Keep mappings in both directions:

1. **Lesson -> Docs**
   - Add `docs:` links in lesson frontmatter.
   - Prefer one primary link plus up to two secondary links.
   - Use stable anchors when practical.
2. **Docs -> Lesson**
   - Add a `## Related Lessons` section in mapped docs with commands like:
     - `rvn learn open objects`
     - `rvn learn open refs`

## Authoring Checklist

When adding or editing a lesson:

1. Update `internal/lessons/defaults/syllabus.yaml` as needed.
2. Add or update `docs:` links in lesson frontmatter.
3. Ensure mapped docs include `## Related Lessons`.
4. Run `rvn learn validate` to catch catalog integrity issues.
5. Run relevant tests for lesson parsing and CLI output.
