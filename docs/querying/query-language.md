# Query Language

> **Shell Tip:** Wrap query strings in single quotes to avoid shell interpretation of `(`, `)`, `|`, and `!`.
> This page is a reference and decision guide for Raven Query Language (RQL).

## Start Here

### Choose the Right Tool

| Use this | When you want |
|----------|---------------|
| `rvn query` | Structured filtering by type/section/trait/asset, field values, scope, and references |
| `rvn search` | Free-text discovery when you do not know the structure yet |
| `rvn backlinks` | All incoming references to one specific object or asset |
| `rvn outlinks` | All outgoing references from one specific target |
| `rvn read` | Full file content after you already identified relevant objects |
| `rvn resolve` | Map a shorthand/alias/date to its target object ID (no content) |
| `rvn date` / `rvn daily` | Everything for one date / open or create a daily note |

Retrieval commands overlap. When several could work, this table picks the sharpest tool for each intent:

| Intent | Prefer | Alternatives / notes |
|--------|--------|----------------------|
| Find files mentioning a word | `rvn search "word"` | `rvn query 'type:<t> content("word")'` scopes the text match to a type/section/trait root; use it when you know the structure |
| Find **real** todo/due traits (not prose) | `rvn query 'trait:todo'` | `rvn search "@todo"` matches the literal text `@todo` and cannot tell a real trait from a mention |
| What links here (incoming refs) | `rvn backlinks <id>` | `rvn query '... refd(...)'` for structured/bulk use; `rvn read <id>` appends backlinks after content |
| What does this link to (outgoing) | `rvn outlinks <id>` | `rvn query '... refs(...)'` when you need structured filtering |
| Resolve a shorthand/alias/date | `rvn resolve <ref>` | `rvn read <ref>` also resolves, but returns full content |
| Everything on a date | `rvn date <date>` / `rvn daily <date>` | `rvn query 'type:date .date==<date>'` for the daily-note object; `rvn query 'trait:due .value==<date>'` for items due that date |

### Choose Query Type

| Query type | Returns | Best for |
|------------|---------|----------|
| `type:<type> ...` | Objects | Find file-backed objects by frontmatter fields and structural relationships |
| `section ...` | Sections | Find Markdown heading sections by title, slug, file, line range, and scope |
| `trait:<name> ...` | Trait instances | Find inline annotations (`@todo`, `@due`, etc.) and surrounding content|
| `asset ...` | Assets | Find indexed non-Markdown files by path, metadata, size, or references |

Core rules:
1. Every query returns exactly one kind of result (objects, sections, traits, or assets).
2. Queries can nest arbitrarily, e.g. `type:project contains(trait:...)`.
3. Boolean composition is `AND` (space), `OR` (`|`), and `NOT` (`!`).

Assets can participate as reference targets in object and trait queries, and `asset` queries return asset rows directly.

## Predicate-by-Root Capability Matrix

Which predicates are legal depends on the query root. Predicates rejected for a root produce a validation error. Field, string, and array predicates are also subject to schema field types (see the per-root predicate sections below).

| Predicate / family | `type:<t>` | `section` | `trait:<name>` | `asset` |
|--------------------|:----------:|:---------:|:--------------:|:-------:|
| Field compares (`==`, `!=`, `<`, `>`, `<=`, `>=`), `exists(.field)` | yes (schema fields) | yes (built-in section fields) | yes (`.value` only) | yes (asset fields) |
| `oneof(.field, [...])` | yes | yes | yes (`.value`) | yes |
| String funcs (`includes`, `startswith`, `endswith`, `matches`) | yes (string fields) | yes (non-numeric fields) | yes (`.value` only) | yes (string fields) |
| Array quantifiers (`any`, `all`, `none`) | yes (array fields) | no | yes (array-valued `.value`) | no |
| `has(...)` — direct downward | yes | yes | no | no |
| `contains(...)` — recursive downward | yes | yes | no | no |
| `in(...)` — direct upward scope | no | yes | yes | no |
| `within(...)` — recursive upward scope | no | yes | yes | no |
| `at(trait:...)` — co-located trait | no | no | yes | no |
| `refs(...)` — outgoing reference | yes | yes | yes | no |
| `refd(...)` — incoming reference | yes | yes | no | yes |
| `content("term")` — full-text | yes | yes | yes | no |

Reading the scope rows:

- **Downward** predicates (`has`, `contains`) look from a container to what it holds, so they live on the container roots `type:` and `section`.
- **Upward** predicates (`in`, `within`) look from an inner item to the scope around it, so they live on the contained roots `trait:` and `section`.
- `has`/`in` match the **direct** relationship only; `contains`/`within` match **recursively** through the section tree.

### Common Mistake: Todos Under a Heading

Traits attach to the **nearest section**, not to the file object. A `@todo` written under a `## Tasks` heading is nested in that section, so it is not *directly* on the project object. As a result:

- `type:project has(trait:todo)` usually returns **nothing** (no todo directly on the object).
- `type:project contains(trait:todo .value==todo)` is correct — it searches the whole section tree.
- From the trait side, use `trait:todo within(type:project)`, not `in(type:project)`.

Lead with the forgiving forms `contains`/`within` unless you specifically want a direct-only match.

### Naming Collision: `in()` vs `oneof()`

`in(...)` is a **scope** predicate (containment) on `trait:`/`section` queries — it has nothing to do with set membership. If you come from SQL and want "field is one of these values," use `oneof(.field, [a,b,c])` instead:

```text
trait:todo in(type:project)               # scope: todos directly in a project
type:project oneof(.status, [active,paused])  # membership: status is one of a set
```

There is no `in()` set-membership form and no `inside()` alias; the containment predicate is always `in()`/`within()`, and membership is always `oneof()`.

## Query Shapes

### Object Query

```text
type:<type> [predicates...]
```

Examples:

```text
type:project
type:project .status==active
type:meeting has(trait:due .value<today)
type:project contains(trait:todo .value==todo)
```

### Section Query

```text
section [predicates...]
```

Examples:

```text
section
section .title==Tasks
section .subtree_line_end>=20
section within(type:project .status==active)
section contains(trait:todo .value==todo)
```

Section rows expose structural fields including `.id`, `.file_object_id`, `.file_path`, `.slug`, `.title`, `.level`, `.line_start`, `.line_end`/`.direct_line_end`, `.subtree_line_end`, and `.parent_section_id`. `line_end` is the direct range end before the next heading of any level; `subtree_line_end` includes nested child sections up to the next same-or-higher heading.

### Trait Query

```text
trait:<name> [predicates...]
```

Examples:

```text
trait:due
trait:due .value<today
trait:highlight in(type:book .status==reading)
```

### Asset Query

```text
asset [predicates...]
```

Examples:

```text
asset
asset .extension==pdf
asset startswith(.media_type, "image/")
asset .size_bytes>1048576
asset refd(type:project .status==active)
```

## Syntax Building Blocks

| Element | Syntax | Example |
|---------|--------|---------|
| Field access | `.` prefix | `.status==active`, `.value<today` |
| Equality / inequality | `==`, `!=` | `.status!=done` |
| Comparison | `<`, `>`, `<=`, `>=` | `.priority>5` |
| Presence | `exists(.field)` | `exists(.email)`, `!exists(.email)` |
| Scalar membership | `oneof(.field, [a,b])` | `oneof(.status, [active,backlog])` |
| Array quantifiers | `any()` / `all()` / `none()` | `any(.tags, _ == "urgent")` |
| String functions | `includes()`, `startswith()`, `endswith()`, `matches()` | `includes(.name, "website")` |
| References | `[[...]]` | `[[person/freya]]` |
| Raw string | `r"..."` | `matches(.path, r"C:\Users\.*")` |

Notes:
- `.field==*` / `!.field==*` are not supported. Use `exists(.field)` / `!exists(.field)`.
- String functions are case-insensitive by default. Pass `true` as a third argument for case-sensitive matching.
- `matches()` accepts either a quoted pattern or regex literal (`/pattern/`).

## Object Query Predicates

### Field Predicates

| Predicate | Meaning |
|-----------|---------|
| `.field==value` | Field equals value |
| `.field!=value` | Field does not equal value |
| `.field>value`, `.field<value` | Numeric/date comparison |
| `.field>=value`, `.field<=value` | Inclusive comparison |
| `exists(.field)` | Field has a value |
| `oneof(.field, [a,b,c])` | Field matches any listed scalar value |

Examples:

```text
type:project .status==active
type:project .title=="Website Redesign"
type:person exists(.email)
type:person !exists(.email)
type:project oneof(.status, [active,paused])
type:date .date>=2026-05-01 .date<=2026-05-31
```

For `ref` and `ref[]` fields (from `schema.yaml`), comparison values are resolved as reference targets, including unbracketed shorthand such as `.company==cursor`.

The built-in `date` type has a generated `.date` field derived from the daily note's canonical `YYYY-MM-DD` object ID. It is queryable but not authored in frontmatter.

### String Matching

| Function | Meaning |
|----------|---------|
| `includes(.field, "str")` | Substring match |
| `startswith(.field, "str")` | Prefix match |
| `endswith(.field, "str")` | Suffix match |
| `matches(.field, "pattern")` / `matches(.field, /pattern/)` | Regex match |

Case-sensitive example:

```text
includes(.name, "API", true)
```

### Array Predicates

Use quantifiers for array fields. `_` represents the current array element.

```text
type:project any(.tags, _ == "urgent")
type:project all(.tags, startswith(_, "feature-"))
type:project none(.tags, _ == "deprecated")
```

### Structural Predicates

| Predicate | Meaning |
|-----------|---------|
| `has(trait:...)` | Object has matching trait directly on itself |
| `has(section...)` | Object has matching section directly in the file |
| `contains(trait:...)` | Object recursively contains matching trait in its section tree |
| `contains(section...)` | Object recursively contains matching section in its section tree |
| `refs(...)` | Object references a target or query match |
| `refd(...)` | Object is referenced by a source or query match |
| `content("term")` | Full-text term in object content |

`refs` accepts direct targets or nested object/section queries.

`has` matches only traits/sections *directly* on the object; `contains` searches the whole section tree. Because traits attach to the nearest section, prefer `contains(trait:...)` for inline traits like `@todo`/`@due` that usually live under a heading. See [Common Mistake: Todos Under a Heading](#common-mistake-todos-under-a-heading).

Examples:

```text
type:project has(trait:due)
type:project has(section .title==Tasks)
type:project contains(trait:todo .value==todo)
type:meeting refs([[project/website]])
type:paper-notes refs([[assets/pdfs/paper.pdf]])
type:meeting refs(type:project .status==active)
type:project refd(type:meeting)
```

For assets, `refs(...)` can target a full asset path or an unambiguous short asset name. Standard Markdown links and images to vault-local non-Markdown files are indexed as references, so `rvn backlinks assets/pdfs/paper.pdf` and `refd(...)` queries can find Markdown files that link to the asset.

## Asset Query Predicates

Asset queries use a fixed set of derived metadata fields:

| Field | Type | Meaning |
|-------|------|---------|
| `.id` | string | Stable asset ID, currently the same as `.file_path` |
| `.file_path` | string | Vault-relative asset path |
| `.filename` | string | Basename including extension |
| `.extension` | string | Lowercase extension without the dot |
| `.media_type` | string | MIME type derived from the extension when known |
| `.size_bytes` | number | File size in bytes |

Examples:

```text
asset .extension==pdf
asset oneof(.extension, ["jpg", "jpeg", "png", "webp", "gif", "svg"])
asset startswith(.media_type, "image/")
asset startswith(.file_path, "assets/screenshots/")
asset includes(.filename, "diagram")
asset .size_bytes>1048576
```

`asset refd(...)` returns assets referenced by the selected source:

```text
asset refd([[project/raven]])
asset refd(type:note refs([[project/raven]]))
asset refd(trait:todo .value==todo)
```

Assets do not have outbound references, traits, authored fields, or scope, so `asset refs(...)`, `asset has(...)`, `asset content(...)`, and scope predicates are not valid.

## Trait Query Predicates

### Value Predicates

| Predicate | Meaning |
|-----------|---------|
| `.value==val`, `.value!=val` | Value equality/inequality |
| `.value>val`, `.value<val` | Numeric/date comparison |
| `.value>=val`, `.value<=val` | Inclusive comparison |
| `oneof(.value, [a,b,c])` | Value is one of listed values |

Date and datetime comparisons also support relative date keywords:
- `today`
- `tomorrow`
- `yesterday`

Relative keywords compare by calendar date. Datetime literals compare by datetime
when written as full datetime values such as `2026-03-01T09:30`.

The same date comparison values work for `type:date .date...` predicates and
for object fields declared as `ref` with `target: date`. Date-target ref fields
store calendar dates such as `2026-03-01`, while queries resolve them to the
corresponding daily note object.

Examples:

```text
trait:due .value<today
trait:due oneof(.value, [today,tomorrow])
trait:due .value<=2026-03-01
trait:remind .value>="2026-03-01T09:30"
type:brief .date==today
```

### Trait Structural Predicates

| Predicate | Meaning |
|-----------|---------|
| `in(...)` | Trait is directly on matching object or section scope |
| `within(...)` | Trait is anywhere within matching object or section scope |
| `at(trait:...)` | Co-located with matching trait (same file and line) |
| `refs(...)` | Trait's line references target or query match |
| `content("term")` | Trait's line contains term |
| `any(.value, ...)`, `all(.value, ...)`, `none(.value, ...)` | Element predicates for array-valued traits |

`in(...)` matches only when the trait sits *directly* in the named scope; `within(...)` matches any ancestor scope. Since a `@todo` under `## Tasks` sits in that section (not directly on the project object), prefer `within(type:project ...)` over `in(type:project ...)`. Note `in(...)` is scope containment, not set membership — for "value is one of a set" use `oneof(.value, [...])`. See [Naming Collision: `in()` vs `oneof()`](#naming-collision-in-vs-oneof).

Examples:

```text
trait:due in(type:meeting)
trait:todo within(type:project .status==active)
trait:due at(trait:todo)
trait:due refs([[person/freya]])
trait:todo content("refactor")
trait:tags any(.value, _ == "raven")
trait:reviewers any(.value, _ == [[person/freya]])
```

`refd(...)` is available on type queries, not trait queries.

## Boolean Composition

| Operator | Syntax | Precedence |
|----------|--------|------------|
| NOT | `!pred` | Highest |
| AND | `pred1 pred2` | Middle |
| OR | `pred1 \| pred2` | Lowest |
| Grouping | `( ... )` | Explicit |

Examples:

```text
type:project .status==active has(trait:due)
type:project (.status==active | .status==backlog) !.archived==true
type:meeting (has(trait:due .value<today) | has(trait:remind .value<today))
```

## Running and Applying Queries

### Inspect Results

```bash
rvn query 'type:project .status==active' --json
rvn query 'trait:due .value<today' --ids
rvn query 'asset .extension==pdf' --json
rvn query 'type:project refs([[company/acme]])' --refresh --json
rvn query 'type:project .status==active' --browse
rvn query 'trait:todo .value==todo' --pipe | rvn pick --multi | rvn update --stdin done --confirm
```

Key flags:
- `--json` — structured JSON output (recommended for agents and scripts)
- `--ids` — output one ID per line for piping to other commands
- `--pipe` — output tab-separated rows for pipe workflows, including `rvn pick`
- `--refresh` — reindex changed files before running the query (useful after editing files outside Raven)
- `--browse` — open an interactive Raven picker and open the selected result in your configured editor

Use `rvn pick` when you want Raven-native interactive selection in a pipeline. It reads `--pipe` output, opens a picker on the terminal, and writes selected IDs to stdout. Cancelling the picker exits with code 130 (and prints nothing), so scripts can distinguish cancellation from a selection.

Section query IDs are stable `file#slug` IDs and asset query IDs are stable asset paths. Section and asset queries do not support `--apply`; use `--ids` to pass IDs to commands that explicitly support them.

### Pagination

Use `--limit` and `--offset` to paginate large result sets:

```bash
rvn query 'type:project' --limit 20                   # First 20 results
rvn query 'type:project' --limit 20 --offset 20       # Next 20
rvn query 'trait:todo' --limit 50 --json                 # Cap results at 50
```

The response metadata includes total count information so you know whether more results exist.

### Save and Reuse Queries

Saved queries live in `raven.yaml` under `queries:` and are managed via dedicated commands:

```bash
rvn query saved list --json                                         # List all saved queries
rvn query saved get overdue --json                                  # Show one saved query
rvn query saved set overdue 'trait:due .value<today' --json         # Create or replace
rvn query saved remove overdue --json                               # Delete

rvn query overdue --json                                            # Run a saved query by name
```

Add `--description` to document the query, and `--arg <name>` (repeatable) to declare positional inputs that placeholders reference:

```bash
rvn query saved set project-todos \
  'trait:todo refs([[{{args.project}}]])' \
  --arg project \
  --description "Todos linked to a project" \
  --json
```

Pass inputs to a saved query positionally (in the order they were declared with `--arg`) or as `key=value` pairs:

```bash
rvn query project-todos project/raven --json                 # positional
rvn query project-todos project=project/raven --json         # key=value
```

Saved query placeholders use `{{args.<name>}}` syntax. Every placeholder must be declared with `--arg`, which also fixes the positional input order.

You can also save default query options with the same flags you would pass to `rvn query`:

```bash
rvn query saved set open-projects 'type:project .status==active' --browse --limit 100 --json
```

This stores the RQL separately from the default options in `raven.yaml`; explicit flags passed when running the saved query override those defaults.

A saved `browse` default only applies when the query runs on an interactive terminal. With `--json` or piped output, the query degrades to normal non-interactive output, so saved queries stay safe for scripts and agents.

### Bulk Operations by Query Type

- Object query `--apply` supports: `set`, `add`, `delete`, `move`.
- Trait query `--apply` supports only: `update <new_value>`.
- Section and asset queries do not support `--apply`.
- All `--apply` operations preview by default; use `--confirm` to apply.

Examples:

```bash
# Object query bulk update
rvn query 'type:project has(trait:due .value<today)' --apply 'set status=overdue' --confirm

# Trait query bulk update
rvn query 'trait:todo .value==todo' --apply 'update done' --confirm
```

## Related Docs

- RQL implementation notes: `querying/internals.md`
- Query-driven bulk changes: `vault-management/bulk-operations.md`
- Organizing and referencing assets: `using-your-vault/assets.md`
- Queryable field/trait definitions: `types-and-traits/schema.md`
- Object IDs and sections (`#fragment`): `types-and-traits/file-format.md`
- Saved query configuration in `raven.yaml`: `using-your-vault/configuration.md`
- Full-text search and other commands: `using-your-vault/common-commands.md`
- MCP query tool usage: `agents/mcp.md`
