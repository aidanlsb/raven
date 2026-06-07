# Raven Templates

Use this skill for template file authoring plus schema/type/core bindings.

This skill is CLI-first. Use MCP as a fallback when CLI access is unavailable, preserving the same JSON and preview/apply expectations.

## Operating rules

- Treat file lifecycle and schema binding lifecycle as separate concerns.
- Use `rvn ... --json` for all template and schema-template operations.
- Keep template files under `directories.template` (default `templates/`).
- `rvn template write` replaces the full template file body.
- Template content is copied at object creation time; editing a template does not update existing notes.
- Prefer explicit type/core bindings over implicit assumptions.
- Inspect current bindings before changing defaults or removing templates.

## Path semantics

- `rvn template write meeting/standard.md ...` writes under `directories.template`, producing `templates/meeting/standard.md` by default.
- `rvn schema template set ... --file ...` resolves and stores the vault-relative template file path, for example `templates/meeting/standard.md`.

## Typical flow

1. Inspect current state with `rvn template list` and `rvn schema template list [--type|--core]`.
2. Create or update template files with `rvn template write`.
3. Register schema template IDs with `rvn schema template set`.
4. Bind template IDs with `rvn schema template bind ... --type ...` or `--core ...`.
5. Set defaults only after bindings are in place, or use `rvn schema template bind <id> --type <type> --default --json` as a shortcut.
6. Smoke test creation with `rvn new <type> <title> --json` or `rvn daily <date> --json` for core `date` templates.
7. Remove in reverse order: clear default if needed, unbind from type/core, remove schema template, then delete the file.

## Cross-references

- Use `raven-schema` for type, field, and trait modeling before binding templates to those schema targets.
- Use `raven-core` for `rvn new --template` to create objects using bound templates.

## Safety

- `rvn template delete` blocks when schema templates still reference that file unless `--force` is used.
- `rvn schema template remove` blocks while the template ID is still bound to any type or core type.
- For default templates, clear defaults before removing the bound template ID.

## Reference

- End-to-end lifecycle and command snippets: `references/template-lifecycle.md`
