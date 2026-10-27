# Local HTTP API

Raven ships no UI. Companion apps such as Perch talk to a vault through
`rvn api`, a headless local HTTP server. Markdown files stay the source of
truth. Every mutating or read operation goes through the same canonical
command layer as the CLI and MCP.

## Start the server

```bash
rvn api --vault-path /path/to/vault
rvn api --vault personal --port 7438 --json
```

The process binds **127.0.0.1 only**. It never listens on a public or LAN
address. Default port is `7438`; `--port 0` asks the OS for an ephemeral port.

On startup Raven prints the listen URL and a bearer token. With `--json` it
prints a single compact envelope line so a launcher (for example Tauri) can
parse it:

```json
{"ok":true,"data":{"url":"http://127.0.0.1:7438","token":"...","port":7438,"bind":"127.0.0.1","api_version":"1","vault_path":"/path/to/vault","cors_origins":["http://localhost:5173"]}}
```

Override the token with `--token` or `RAVEN_API_TOKEN`. Override the port with
`--port` or `RAVEN_API_PORT`. SIGINT and SIGTERM shut the server down.

## Authentication

Every request except CORS preflight (`OPTIONS`) requires:

```
Authorization: Bearer <token>
```

Missing or wrong tokens return HTTP 401 with `error.code` `UNAUTHORIZED`.

## Host check

Requests whose `Host` header is not a loopback name (`127.0.0.1`, `localhost`,
or another loopback IP, with or without a port) are rejected with HTTP 403 and
`INVALID_HOST`. This blocks DNS-rebinding attacks against the loopback listener.

## CORS

Allowed origins default to local dev ports (`http://localhost:5173`,
`http://127.0.0.1:5173`, `4173`, and `1420`). Replace the list with repeated
`--cors-origin` flags or comma-separated `RAVEN_API_CORS_ORIGINS`. Origins that
are not listed do not receive `Access-Control-Allow-Origin`.

## Versioning

The HTTP path prefix is `/v1/`. The same version string is advertised as
`api_version` on `GET /v1/commands`, `GET /v1/health`, `GET /v1/session`,
`GET /v1/revision`, the startup envelope, and the `X-Raven-API-Version`
response header.

Breaking changes to the HTTP contract increment this version (new prefix and
startup field). Additive fields on command envelopes follow the existing CLI/MCP
result contract and do not bump the API version.

## Endpoints

All JSON responses use the standard Raven envelope:
`{ ok, data, error?, warnings?, meta? }`. Command failures from `/v1/invoke`
still use HTTP 200; the envelope's `ok` field is the success signal.

### `GET /v1/health`

Liveness. `data.status` is `"ok"` and `data.api_version` is `"1"`. Requires the
bearer token. Health does not identify the vault or the user's editor; use
`GET /v1/session` for that.

### `GET /v1/session`

Bound session for attach-only clients (a UI that did not spawn `rvn api` and
therefore never saw the startup envelope). Requires the bearer token.

```json
{"ok":true,"data":{"api_version":"1","vault_path":"/path/to/vault","editor":"code","editor_mode":"auto","daily":{"directory":"daily","today_id":"2026-10-15","today_path":"daily/2026-10-15.md","today_exists":false},"revision":{"generation":"a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6","counter":12}}}
```

| Field | Meaning |
| --- | --- |
| `vault_path` | Absolute vault root this process is pinned to |
| `editor` | Command `rvn open` would launch: `config.toml` `editor`, else `$EDITOR`, else `""` |
| `editor_mode` | Canonical launch mode `rvn open` applies: `auto`, `terminal`, or `gui` (empty or unrecognized config is `auto`) |
| `daily.directory` | Vault-relative daily-note directory (`directories.daily`, default `daily`) |
| `daily.today_id` | Canonical object ID for today (bare `YYYY-MM-DD`, same as `rvn date today`) |
| `daily.today_path` | Vault-relative path for today's note (`daily/YYYY-MM-DD.md`) |
| `daily.today_exists` | Whether that file is already on disk. Session never creates it; call `daily` to create |
| `api_version` | Same contract version as health and `/v1/commands` |
| `revision` | Disposable index identity `{generation, counter}`. Generation is random and created when the index is built from scratch. Counter is monotonic and rises with every index projection. Deleting the index or a full rebuild yields a new generation. |

`daily` uses Raven's existing today/timezone rules (`time.Now()` on the host,
the same calendar date as `rvn daily` / `rvn date today`) and the vault's
`directories.daily` layout. It does not write files.

This lives on `/v1/session` rather than `/v1/health` so health stays a cheap
liveness probe. Session is the attach handshake: which vault, how to open
files the way Raven itself would, where today's daily note lives, and which
index revision the process currently serves, without reimplementing
config/`$EDITOR`/date resolution or invoking `vault_path`.

### `GET /v1/revision`

Cheap poll for the same `revision` object session returns:

```json
{"ok":true,"data":{"generation":"a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6","counter":12}}
```

There is no event stream. A companion UI attaches via session, then polls this
endpoint. When `generation` changes, drop cached index state and refetch. When
only `counter` rises, refetch whatever the UI derived from the previous
revision.

While `rvn api` runs it watches the vault (recursive, skipping `.raven/` and
excluded paths), debounces roughly 200ms, and reindexes changed, deleted, and
moved files through the same journal and projection path as CLI writes. Editor
saves, `git pull`, and other `rvn` processes therefore move the revision.
Own writes are skipped when the indexed mtime or content hash already matches.
Watcher overflow or a large burst runs an incremental reindex instead of
per-file projection. SIGINT and SIGTERM stop the watcher with the server.

### `GET /v1/schema`

The vault schema: types, fields (including enum `values`), traits, and related
introspection. This is the same payload as `rvn schema --json`, produced by the
canonical `schema` command. When the vault defines saved queries, `data.queries`
includes each query's `name`, `query`, `args` (the declared `{{args.*}}` names,
always an array, empty if none), and `description`. `POST /v1/invoke` of
`query_saved_list` / `query_saved_get` returns the same `args` field so a client
can prompt for them.

### `GET /v1/commands`

Machine-readable description of every invokable command, generated from the
command registry (the same catalog MCP discovery returns): id, summary,
description, `read_only`, `destructive`, `preview_mode`, `args_schema`, and
`schema_hash`, plus `api_version`. Type the UI against this document rather
than a hard-coded command list. HTTP routes
(`/v1/health`, `/v1/session`, `/v1/revision`, `/v1/schema`, `/v1/commands`, `/v1/invoke`) are
not commands; they are documented above. `objects` is a command on
`/v1/invoke`, not a dedicated HTTP route.

### `POST /v1/invoke`

```json
{"command":"read","args":{"reference":"people/freya"}}
```

Saved queries take the same `inputs` map MCP uses. Pass the saved query name as
`query_string` and bind `{{args.*}}` placeholders from `args.inputs`. Discover
declared names from `GET /v1/schema` `data.queries[].args` or
`query_saved_list`:

```json
{"command":"query","args":{"query_string":"project-todos","inputs":{"project":"projects/raven"}}}
```

The `objects` command is the cheap all-object corpus for fuzzy pickers. It is
served from the index (no file bodies) and paginates like `query`
(`limit` `0` means no limit):

```json
{"command":"objects","args":{"limit":200,"offset":0}}
```

```json
{"ok":true,"data":{"items":[{"id":"people/freya","type":"person","name":"Freya","file_path":"people/freya.md"}],"total":1,"returned":1,"offset":0,"limit":200,"has_more":false}}
```

`objects` is a command on `/v1/invoke` rather than a dedicated `GET /v1/objects`
route: listing needs the same `limit`/`offset` contract as `query`, and the
command spine already exposes that to CLI, MCP, and `/v1/invoke`. `/v1/schema`
stays a GET shortcut because it has no paging arguments.

Enriched `read` JSON includes `data.links`: each `[[wikilink]]` in `data.content`
with UTF-8 byte and UTF-16 code-unit offsets, the raw text, the target as
written, and identity-first resolution. Offsets are into that `content` string
(including section slices). `--raw` omits `links`. `GET /v1/schema` is the vault
type/trait schema and does not describe command result shapes; command argument
metadata stays on `GET /v1/commands`.

Runs `commandexec.Invoker.Execute` with `caller=api`, the same path as CLI and
MCP. The response envelope is unchanged, including `meta.mutation.phase`
(`preview` or `applied`) on writes. Mutating commands that update the index
advance `GET /v1/revision` the same way CLI and MCP writes do.

If the index is locked for rebuild (`INDEX_LOCKED`), the API waits and retries
for about two seconds with backoff, then returns the error.

## Command surface

`GET /v1/commands` lists every discoverable invokable registry command — the
same catalog MCP discovery returns. `POST /v1/invoke` accepts any command the
registry policy marks invokable. Companion clients such as Perch choose which
commands to call; Raven does not keep a parallel subset.

Runtime, bootstrap, and grouping commands (`api`, `mcp_serve`, `lsp`, `config`,
`vault`, `template`, and the `mcp install/remove/status/show` helpers) return
`ok: false` with `COMMAND_NOT_INVOKABLE`. Unknown command IDs return
`COMMAND_NOT_FOUND`.

`open` launches the configured editor in this process. `init` creates a vault
at `args.path` and does not use the API's pinned vault. Vault-scoped commands
run against the pinned vault; `VaultScopeNone` commands (for example `version`,
`config_show`, `vault_list` without `--path-only`) omit that path.

Bulk writes still preview unless `confirm` is set. Single-object `set` / `move`
apply immediately unless `dry-run` is set. Read `meta.mutation.phase`; do not
infer apply-vs-preview from ad-hoc data fields.

## Example

```bash
TOKEN=...
curl -sS -H "Authorization: Bearer $TOKEN" http://127.0.0.1:7438/v1/health
curl -sS -H "Authorization: Bearer $TOKEN" http://127.0.0.1:7438/v1/session
curl -sS -H "Authorization: Bearer $TOKEN" http://127.0.0.1:7438/v1/revision
curl -sS -H "Authorization: Bearer $TOKEN" http://127.0.0.1:7438/v1/schema
curl -sS -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"command":"query","args":{"query_string":"type:person"}}' \
  http://127.0.0.1:7438/v1/invoke
curl -sS -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"command":"query","args":{"query_string":"project-todos","inputs":{"project":"projects/raven"}}}' \
  http://127.0.0.1:7438/v1/invoke
curl -sS -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"command":"objects","args":{"limit":200,"offset":0}}' \
  http://127.0.0.1:7438/v1/invoke
```
