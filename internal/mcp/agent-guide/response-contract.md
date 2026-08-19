# Response Contract

Use this guide to interpret Raven MCP results safely and consistently.

## Standard JSON envelope

Every command returns `ok`. Other envelope fields are present only when
applicable (`omitempty` on the wire). A typical success is:

```json
{
  "ok": true,
  "data": {}
}
```

Failures carry `error`; non-empty `warnings` and applicable `meta` are added to
successful responses. Do not require absent fields to be serialized as `null`
or `[]`.

## Collection keys

The primary homogeneous collection in a successful response is always
`data.items`, including query/search windows, docs search matches, ambiguous
`resolve` candidates, import outcomes, bulk previews/applies, and the
`raven_discover` command catalog. When there are no items, the value is `[]`, not
`null`.

Semantically distinct or secondary collections keep descriptive keys, such as
`ids` for `query --ids`, `issues` for `check`, `sections` for read outlines, and
`items_by_target` / `items_by_source` for grouped link traversal. Ambiguous
reference failures also keep candidate IDs in `error.details.matches`; that is
error detail, not a successful result collection.

## Compact invoke flow

1. `raven_discover` to fetch the authoritative command catalog.
2. `raven_describe(command="...")` to fetch the strict arg schema and command guidance. The response includes a short `summary` plus a fuller `description` with command-specific syntax (e.g. RQL examples for `query`).
3. `raven_invoke(command="...", args={...})` to execute.

Important:
- Command params belong under `args`.
- Top-level keys are only `command`, `args`, `vault`, `vault_path`, `schema_hash`, `strict_schema`.
- Use `vault` for a configured vault name or `vault_path` for an explicit vault directory on a single invocation.
- Do not pass both `vault` and `vault_path`.
- Registry flags keep hyphens in `args` (`dry-run`, `errors-only`,
  `fields-json`); named positional arguments keep their declared underscores
  (`query_string`, `old_str`, `section_id`).

For `resources/read`, the vault-scoped Raven URIs `raven://schema/current`, `raven://queries/saved`, and `raven://vault/agent-instructions` also accept optional top-level `vault` or `vault_path` params.
- Use one or the other for that read.
- `resources/list` accepts the same `vault`/`vault_path` params, so list and read stay consistent.
- Both `resources/list` and `resources/read` return a `vault_context` object for vault-scoped requests so you can confirm the target vault.

## Error handling rules

1. If `ok=false`, treat the operation as failed.
2. Branch on stable `error.code`.
3. Prefer `error.details.retry_with` when present.
4. Ask before retrying with assumptions.

Failed bulk mutations retain parsed inputs in `error.details` under the
command's canonical input key (`trait_ids`, `references`, or `object_ids`) plus
`total`. The inputs preserve their original order so they can be inspected or
retried even when failure occurs before preview or apply.

## CLI process exit status

When invoking the `rvn` binary with `--json`, a response with `ok=false` exits
with status 1, including startup and load failures such as invalid config,
unresolved vaults, and fatal schema errors. Successful commands exit 0.

`rvn check` is a lint-style exception: it can return `ok=true` and exit 1 when
issues are found (or warnings are found with `--strict`). Cancelling `rvn pick`
exits 130. MCP tool calls do not expose a shell process status; use the envelope
and tool error signal there.

## Mutation phase (applied vs preview)

Every mutating command reports whether it wrote to the vault in a single, uniform
field:

```json
{
  "ok": true,
  "meta": { "mutation": { "phase": "applied" } }
}
```

- `meta.mutation.phase = "applied"` — the change was written to the vault.
- `meta.mutation.phase = "preview"` — nothing was written; the response describes
  what a subsequent apply would do.

Read this field to decide whether a write happened. Do **not** infer it from
heterogeneous `data` fields (`data.status`, `data.preview`, `data.needs_confirm`,
etc.), which vary by command and remain only for backward compatibility. The
phase is consistent across every mutating command (`new`, `upsert`, `add`, `set`,
`unset`, `delete`, `move`, `section_create`, `section_move`, `section_rename`,
`reclassify`, `update`, `edit`, `import`, `check fix`,
`check create-missing`, `schema` writes/renames, `template` writes, saved-query
writes, and skill installs) and across the CLI and MCP surfaces.

`meta.mutation` is present only on mutating commands. Read-only commands (`query`
without `apply`, `read`, `search`, `schema`, etc.) omit it. A failed mutation
(`ok=false`) also omits it, because nothing was previewed or applied.

`query` with `apply` carries the phase of the write it delegates to. When a
mutation is blocked pending confirmation (for example a `move` into a mismatched
type directory, or a `reclassify` that would drop fields without `force`), the
phase is `"preview"` because nothing was written.

## Write identity: file path vs link ID

Object-creating writes (`new`, `upsert`, and `daily`) surface an identity pair in
`data` so you never have to guess a reference from a file path:

- `data.file` — where the file lives (vault-relative path, includes `.md`).
- `data.id` — the canonical object ID to use in `[[refs]]` and follow-up commands
  (`set`, `add`, `edit`, `delete`, …).

These two often differ, and the mapping is config-dependent:

- A typed object file `type/person/freya.md` links as `person/freya`.
- A daily note file `daily/2026-03-15.md` links as the bare date `2026-03-15`.

Always take the reference ID from `data.id`. Deriving an ID from `data.file`
(e.g. stripping `.md`) produces non-canonical refs and triggers
`non_canonical_ref` / `non_canonical_path`. The human CLI prints the same pair as
a `Created <file>` line followed by a `link as <id>` hint.

## Preview and apply semantics

There are two mutation classes with different defaults:

1. Single-object writes apply immediately (`meta.mutation.phase = "applied"`):
   `set`, `add`, `update`, `edit`, `section_create`,
   `section_move`, `section_rename`, `reclassify`, and single-object
   `delete`/`move`. Commands that expose `dry-run` accept `dry-run=true` to get
   a preview (`meta.mutation.phase = "preview"`) without writing.
2. High-blast-radius operations are preview-first (`meta.mutation.phase =
   "preview"`) and require `confirm=true` to apply: any bulk write (an MCP bulk
   ID array or CLI `--stdin`), `query` with `apply`, `schema rename`, `schema
   convert`, and the `check fix` / `check create-missing` repair subcommands.

Examples:

```text
# Applies immediately (single-object):
raven_invoke(command="edit", args={"reference":"project/website.md", "old_str":"A", "new_str":"B"})
raven_invoke(command="delete", args={"reference":"project/old"})
raven_invoke(command="section_create", args={"file":"project/website", "title":"Notes", "level":2})
raven_invoke(command="section_move", args={"section_id":"project/website#notes", "after":"project/website#tasks"})
raven_invoke(command="section_rename", args={"section_id":"project/website#tasks", "new_heading_text":"Completed Tasks"})

# Preview a single-object write first (optional):
raven_invoke(command="edit", args={"reference":"project/website.md", "old_str":"A", "new_str":"B", "dry-run":true})

# Preview-first; apply only after explicit approval with confirm=true:
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo", "apply":["update done"]})
raven_invoke(command="delete", args={"references":["project/one","project/two"], "confirm":true})
raven_invoke(command="move", args={"object_ids":["project/one","project/two"], "destination":"archive/", "confirm":true})
raven_invoke(command="reclassify", args={"references":["page/one","page/two"], "new-type":"note"})
raven_invoke(command="reclassify", args={"references":["page/one","page/two"], "new-type":"note", "confirm":true})
```

MCP bulk mode takes an ID array rather than stdin bytes. Use the key from
`raven_describe`: generally `references`, with `object_ids` for bulk `add` and
`move`, and `trait_ids` for bulk `update`. Failed bulk responses retain that
same ordered array in `error.details`.

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
      "source": "vault"
    }
  }
}
```

Fields:
- `path` — resolved absolute vault path (always present).
- `source` — how the vault was selected: `vault_path` (explicit path override), `vault` (named vault from invocation), `focus` (in-memory MCP session focus), `pinned` (constructor launch pin), or `base_args` (from serve flags).
- `name` — configured vault name (omitted when no name could be resolved).

Vault-scoped `resources/list` and `resources/read` responses also return a
top-level `vault_context` object with the same fields.

Commands that do not require vault resolution (e.g. `version`, `config show`) omit `vault_context`.

### Explicit vaults are required

Every vault-scoped MCP operation requires an explicit source: per-call
`vault_path` / `vault`, an in-memory session focus set with `vault_focus`, or a
server launch pin supplied with `--vault-path` / `--vault`. Per-call values
override session focus for one invocation. MCP never resolves from the CLI's
`active_vault` or config's `default_vault`.

Use `vault_list` to inspect the machine-level vault registry and its
`default_vault`, `active_vault`, and `current_vault` selection in one result.
For a path-only response, invoke `vault_list` with
`args={"path-only":true}`; that mode is vault-scoped and follows the explicit
MCP resolution rules above. The compatibility IDs `vault_current` and
`vault_path` remain invokable but are not returned by discovery.

- `VAULT_AMBIGUOUS` error: the call lacked an explicit `vault`/`vault_path` and
  the server has neither session focus nor a launch pin. Retry with an explicit
  vault or invoke `vault_focus`.

## Warnings

- Warnings are action items, not noise.
- Surface warnings that affect correctness or safety.
- If warnings indicate stale state, run corrective steps such as `reindex` before continuing.
- `DATABASE_OUTDATED` means `.raven/index-dirty.json` contains pending
  post-mutation projection work. Run `reindex`, then retry the read.
- `REFERENCE_RESOLUTION_INCOMPLETE` means the changed file was indexed
  successfully, but the follow-up reference pass failed and backlinks may be
  stale. Do not retry the mutation; run `reindex` to complete resolution.
- `REF_TARGET_MISSING` on a successful write means the object was created/modified but a
  reference points at a target that does not exist yet. This benign write-time warning is
  intentionally distinct from the fatal `REF_NOT_FOUND` error (returned when a read/resolve
  cannot find a target), so you can branch on the code alone. The response also includes
  `data.missing_refs` and `data.missing_ref_items` (with an inferred `type` when known).
  Each warning carries a structured `create_invoke` (`{command, args}`) — pass it directly
  to `raven_invoke` — alongside the equivalent `create_command` string. Create the missing
  targets with `check create-missing` or the suggested `create_invoke` when appropriate.

## Related topics

- `raven://guide/error-handling`
- `raven://guide/issue-types`
