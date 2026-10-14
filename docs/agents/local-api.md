# Local HTTP API

Raven ships no UI. Companion apps such as Perch talk to a vault through
`rvn api`, a headless HTTP server bound to `127.0.0.1` only. Markdown files
remain the source of truth. Every invoke runs through the same canonical
command layer as the CLI and MCP (`commandexec.Invoker.Execute`) and returns
the standard `{ ok, data, error, warnings, meta }` envelope unchanged.

## Start the server

```bash
rvn api --vault-path /path/to/vault
rvn api --json --port 0
```

Human startup prints the listen URL and bearer token. `--json` prints one
line a launcher can parse:

```json
{"ok":true,"data":{"url":"http://127.0.0.1:7437","token":"…","host":"127.0.0.1","port":7437,"api_version":"1"}}
```

| Flag / env | Meaning |
|---|---|
| `--port` / `RAVEN_API_PORT` | Listen port. Default `7437`. `0` lets the OS assign a free port. |
| `--token` / `RAVEN_API_TOKEN` | Bearer token. Generated per run when omitted. |
| `--cors-origin` / `RAVEN_API_CORS_ORIGINS` | Allowed CORS origins. Default localhost Vite/SvelteKit/Tauri ports. |

SIGINT and SIGTERM shut the server down cleanly.

## Versioning

The contract version is `1`, advertised as `api_version` on startup, health,
and `GET /v1/commands`, and as the `/v1/` URL prefix.

- Additive catalog changes (new allowlisted commands or extra response fields)
  stay on `/v1/`. Clients should ignore unknown fields.
- Breaking changes (removed routes, changed envelope shape, auth scheme, or
  command argument meaning) get a new prefix (`/v2/`) and a bumped
  `api_version`.

`GET /v1/commands` is the typed surface: each command includes `args_schema`
and `schema_hash` generated from the command registry.

## Authentication and browser safety

Every request except CORS preflight requires `Authorization: Bearer <token>`.
Missing or wrong tokens return HTTP 401 with `error.code` `UNAUTHORIZED`.

The listener is `127.0.0.1` only. Requests whose `Host` header is not loopback
(`localhost`, `127.0.0.1`, `::1`) return HTTP 403 with `FORBIDDEN`, which
blocks DNS rebinding. CORS is an allowlist of origins; unknown origins are
rejected.

## Endpoints

### `GET /v1/health`

Liveness. `{ ok: true, data: { status: "ok", api_version: "1" } }`.

### `GET /v1/schema`

Vault schema from the canonical `schema` command: types, fields (including
enum `values`), traits, and related schema data, wrapped in the standard
envelope.

### `GET /v1/commands`

Machine-readable catalog of **allowlisted** commands and their arguments,
plus `api_version`. Generated from the command registry so a UI can be typed
against it.

### `POST /v1/invoke`

```json
{ "command": "read", "args": { "reference": "people/freya" } }
```

Runs `commandexec.Invoker.Execute`. The HTTP status is 200 when the body
parsed; command success or failure is in `ok` / `error`, matching CLI `--json`.

If the derived index is locked for rebuild (`ErrIndexLocked`), the API waits
and retries for about two seconds with backoff before returning the
`DATABASE_ERROR` envelope.

## Allowlist

The MVP invoke surface is read-only vault commands plus a small write set:

**Read:** `query`, `query_saved_list`, `query_saved_get`, `read`, `backlinks`,
`outlinks`, `search`, `daily`, `date`, `schema`, `schema_validate`,
`schema_template_list`, `schema_template_get`, `vault_stats`, `resolve`,
`check`, `trash_list`.

**Write:** `set`, `unset`, `new`, `move`.

Anything else, including `query` `--apply` of a non-allowlisted nested
command such as `delete`, returns `ok: false` with `API_COMMAND_DENIED`.

Preview versus applied is unchanged: mutating responses carry
`meta.mutation.phase` (`preview` or `applied`). Bulk moves still preview
until `confirm`; single-object `set` / `new` / `move` apply unless `dry-run`
is set. Read `meta.mutation.phase`; do not infer from `data.status`.
