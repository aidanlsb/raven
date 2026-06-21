# Response Contract

Use this guide to interpret Raven MCP results safely and consistently.

## Standard JSON envelope

All commands return:

```json
{
  "ok": true,
  "data": {},
  "error": null,
  "warnings": [],
  "meta": {}
}
```

## Compact invoke flow

1. `raven_discover` to fetch the authoritative command catalog.
2. `raven_describe(command="...")` to fetch the strict arg schema and command guidance. The response includes a short `summary` plus a fuller `description` with command-specific syntax (e.g. RQL examples for `query`).
3. `raven_invoke(command="...", args={...})` to execute.

Important:
- Command params belong under `args`.
- Top-level keys are only `command`, `args`, `vault`, `vault_path`, `schema_hash`, `strict_schema`.
- Use `vault` for a configured vault name or `vault_path` for an explicit vault directory on a single invocation.
- Do not pass both `vault` and `vault_path`.

For `resources/read`, the vault-scoped Raven URIs `raven://schema/current`, `raven://queries/saved`, and `raven://vault/agent-instructions` also accept optional top-level `vault` or `vault_path` params.
- Use one or the other for that read.
- `resources/list` still reflects the server's pinned/current vault.

## Error handling rules

1. If `ok=false`, treat the operation as failed.
2. Branch on stable `error.code`.
3. Prefer `error.details.retry_with` when present.
4. Ask before retrying with assumptions.

## Preview and apply semantics

There are two mutation classes with different defaults:

1. Single-object writes apply immediately: `set`, `add`, `update`, `edit`, and
   single-object `delete`/`move`. Pass `dry-run=true` to get a preview (the
   response carries `preview=true` or `status="preview"`) without writing.
2. High-blast-radius operations are preview-first and require `confirm=true` to
   apply: any bulk write (`stdin=true`), `query` with `apply`, `schema rename`,
   and `check` fixes (`fix`, `create-missing`).

Examples:

```text
# Applies immediately (single-object):
raven_invoke(command="edit", args={"path":"project/website.md", "old_str":"A", "new_str":"B"})
raven_invoke(command="delete", args={"object_id":"project/old"})

# Preview a single-object write first (optional):
raven_invoke(command="edit", args={"path":"project/website.md", "old_str":"A", "new_str":"B", "dry-run":true})

# Preview-first; apply only after explicit approval with confirm=true:
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo", "apply":["update done"]})
raven_invoke(command="delete", args={"stdin":true, "confirm":true})
raven_invoke(command="move", args={"stdin":true, "destination":"archive/", "confirm":true})
```

Because single-object writes apply on the first call, only invoke them when the
user intent is clear. When unsure about a `delete`/`move`, inspect the object
(and run `backlinks` for deletes) or call with `dry-run=true` first.

## Vault context

Vault-bound responses include a `vault_context` block in `meta`:

```json
{
  "meta": {
    "vault_context": {
      "name": "work",
      "path": "/home/user/vaults/work",
      "source": "active_vault"
    }
  }
}
```

Fields:
- `path` — resolved absolute vault path (always present).
- `source` — how the vault was selected: `vault_path` (explicit path override), `vault` (named vault from invocation), `pinned` (server pinned path), `base_args` (from serve flags), `active_vault`, `default_vault`, or `default_vault_fallback`.
- `name` — configured vault name (omitted when no name could be resolved).

Commands that do not require vault resolution (e.g. `version`, `config show`) omit `vault_context`.

## Warnings

- Warnings are action items, not noise.
- Surface warnings that affect correctness or safety.
- If warnings indicate stale state, run corrective steps such as `reindex` before continuing.
- `REF_NOT_FOUND` on a successful write means the object was created/modified but a reference
  points at a target that does not exist yet. The response also includes `data.missing_refs`
  and `data.missing_ref_items` (with an inferred `type` when known). Create the missing
  targets with `check create-missing` or the suggested `create_command` when appropriate.

## Related topics

- `raven://guide/error-handling`
- `raven://guide/issue-types`
