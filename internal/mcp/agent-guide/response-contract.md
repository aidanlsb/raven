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
- `resources/list` accepts the same `vault`/`vault_path` params, so list and read stay consistent.
- Both `resources/list` and `resources/read` return a `vault_context` object for vault-scoped requests so you can confirm the target vault.

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
   and the `check fix` / `check create-missing` repair subcommands.

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

## Paging fields

Commands that return result windows expose paging affordances in `data` so you
can loop without guessing:

- `query` and `docs search` include `total`/`returned`/`offset`/`limit` and a
  `has_more` boolean.
- When `has_more` is `true`, `query` also returns `next_offset` — use it as the
  next request's `offset`. Loop while `has_more` is `true`; stop when it is
  `false`.

`query` is unlimited by default (`limit` 0): the full result set comes back in
one call, so `has_more` is `false` and `next_offset` is omitted. See
`raven://guide/query-at-scale` for the count-then-page pattern.

## Vault context

Vault-bound responses always include a `vault_context` block in `meta`, so you can
confirm which vault was used on every vault-scoped call:

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

Vault-scoped `resources/list` and `resources/read` responses also return a
top-level `vault_context` object with the same fields.

Commands that do not require vault resolution (e.g. `version`, `config show`) omit `vault_context`.

### Explicit vs ambient vaults

`vault_path`, `vault`, `pinned`, and `base_args` are *explicit* sources you (or the
server operator) chose. `active_vault`, `default_vault`, and
`default_vault_fallback` are *ambient* — they come from mutable global state, so a
call that omits an explicit vault can silently target whatever vault was last made
active. To be certain which vault a write hits, pass `vault` or `vault_path`.

- `VAULT_FALLBACK` warning: a write resolved its vault from ambient state while
  more than one vault is configured. Confirm `vault_context` is the vault you
  intended, or re-issue the call with an explicit `vault`/`vault_path`.
- `VAULT_AMBIGUOUS` error: the server runs in strict vault mode
  (`rvn serve --strict-vault` or `[mcp] strict_vault = true`) and the call lacked
  an explicit `vault`/`vault_path` with no server-pinned vault. Retry with an
  explicit vault.

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
