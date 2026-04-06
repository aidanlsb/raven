# Raven Templates

Use this skill for template file authoring plus schema/type/core bindings.

## Operating rules

- Treat file lifecycle and schema binding lifecycle as separate concerns.
- Prefer Raven MCP tool equivalents when available in-session; otherwise use `rvn ... --json`.
- Keep template files under `directories.template` (default `templates/`).
- Prefer explicit type/core bindings over implicit assumptions.
- Inspect current bindings before changing defaults or removing templates.

## Typical flow

1. Inspect current state with `rvn template list` and `rvn schema template list [--type|--core]`.
2. Create or update template files with `rvn template write`.
3. Register schema template IDs with `rvn schema template set`.
4. Bind template IDs with `rvn schema template bind ... --type ...` or `--core ...`.
5. Set defaults only after bindings are in place.
6. Remove in reverse order: clear default if needed, unbind from type/core, remove schema template, then delete the file.

## Cross-references

- Use `raven-schema` for schema template binding commands (`schema template bind`, `schema template set`, etc.).
- Use `raven-core` for `rvn new --template` to create objects using bound templates.

## Safety

- `rvn template delete` blocks when schema templates still reference that file unless `--force` is used.
- For default templates, clear defaults before removing the bound template ID.

## Reference

- End-to-end lifecycle and command snippets: `references/template-lifecycle.md`
