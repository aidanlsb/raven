# Raven Templates

Use this skill for template file authoring plus schema/type/core bindings.

## Operating rules

- Treat file lifecycle and schema binding lifecycle as separate concerns.
- Keep template files under `directories.template` (default `templates/`).
- Prefer explicit type/core bindings over implicit assumptions.

## Typical flow

1. Create or update template files with `rvn template write`.
2. Register schema template IDs with `rvn schema template set`.
3. Bind template IDs with `rvn schema template bind ... --type ...` or `--core ...`.
4. Set defaults only after bindings are in place.
5. Remove in reverse order: unbind from type/core, remove schema template, then delete file.

## Safety

- `rvn template delete` blocks when schema templates still reference that file unless `--force` is used.
- For default templates, clear defaults before removing the bound template ID.

## Reference

- End-to-end lifecycle and command snippets: `references/template-lifecycle.md`
