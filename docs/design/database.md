# Database (design)

Raven maintains a SQLite index under `.raven/` to support fast queries. It is a cache: it can be deleted and rebuilt with `rvn reindex`.

## What’s indexed

- **Objects**: file-level objects + embedded objects + auto-generated sections
- **Traits**: inline `@trait` annotations (only if the trait is defined in `schema.yaml`)
- **References**: `[[...]]` links, including those inside frontmatter

## Notes

- Objects store all frontmatter fields in a JSON blob (`objects.fields`) for flexible schema evolution.
- Traits store a single `value` (or NULL for bare/boolean traits).
- Aliases are extracted from the `alias` field (if present) into an indexed column for fast resolution.

If you need the exact schema, see `internal/index/database.go` (it is the canonical source).

