# Raven: Agent-Friendly Improvements

This document details specific improvements to make Raven an excellent tool for LLM agents to interact with. The goal is to make Raven a first-class "memory layer" that agents can read, write, and query reliably.

> **Note**: This document contains both implemented features and design proposals. See the Implementation Status table below for current state. Some sections describe proposed `rvn object` commands that were implemented with different names (`rvn new`, `rvn set`, `rvn delete`, `rvn read`). See `README.md` and `docs/SPECIFICATION.md` for the authoritative command reference.

---

## Implementation Status

| Feature | Status | Notes |
|---------|--------|-------|
| `--json` flag on all commands | ✅ Implemented | Standard envelope for all responses |
| Standard response envelope | ✅ Implemented | `ok`, `data`, `error`, `meta`, `warnings` |
| Structured error codes | ✅ Implemented | See `internal/cli/errors.go` |
| Schema introspection (`rvn schema`) | ✅ Implemented | Types, traits, commands discovery |
| MCP Server (`rvn serve`) | ✅ Implemented | Full JSON-RPC 2.0 over stdin/stdout |
| Object creation (`rvn new`) | ✅ Implemented | With `--field` flags for required fields |
| Object update (`rvn set`) | ✅ Implemented | Update frontmatter fields |
| Object deletion (`rvn delete`) | ✅ Implemented | Trash by default, backlink warnings |
| Schema editing (`rvn schema add/update/remove`) | ✅ Implemented | Full schema modification with integrity checks |
| Read raw content (`rvn read`) | ✅ Implemented | For agent file access |
| Quick capture (`rvn add`) | ✅ Implemented | With reference validation, timestamps opt-in |
| Audit log | ✅ Implemented | Configurable in `raven.yaml` |
| Batch operations | 🔮 Future | See docs/FUTURE.md |
| Full-text search | 🔮 Future | See docs/FUTURE.md |
| File watching | 🔮 Future | See docs/FUTURE.md |

---

## Table of Contents

1. [Design Principles](#design-principles)
2. [Output Format Standardization](#output-format-standardization)
3. [Input Format Standardization](#input-format-standardization)
4. [Error Handling](#error-handling)
5. [Schema Introspection](#schema-introspection)
6. [Mutation Commands](#mutation-commands)
7. [Timestamps and Temporal Queries](#timestamps-and-temporal-queries)
8. [Rich Context in Responses](#rich-context-in-responses)
9. [ID Stability](#id-stability)
10. [Batch Operations](#batch-operations)
11. [Validation and Dry Run](#validation-and-dry-run)
12. [MCP Server Integration](#mcp-server-integration)
13. [Command Reference Updates](#command-reference-updates)

---

## Design Principles

### 1. Explicit Over Implicit

Agents struggle with magic and inference. Every command should:
- Accept explicit parameters
- Return explicit, complete responses
- Never require the agent to "figure out" what happened

### 2. Structured Input, Structured Output

- All commands support `--json` for JSON output
- Create/update commands accept `--json` for JSON input
- No parsing of human-formatted text required

### 3. Predictable and Idempotent

- Same input → same output (where possible)
- IDs are stable across reindexes
- Errors are deterministic and parseable

### 4. Rich Responses by Default

- Single API call should return enough context to continue
- Don't force agents to make multiple round-trips
- Include parent context, resolved refs, timestamps

### 5. Self-Documenting

- Schema is queryable
- Agents can discover what types, traits, and fields exist
- Error messages include valid options

---

## Output Format Standardization

### The `--json` Flag

Every read command must support `--json` for machine-readable output.

**Current commands to update:**

```bash
rvn trait <name> --json
rvn query <name> --json
rvn backlinks <target> --json
rvn stats --json
rvn untyped --json
rvn check --json
rvn date <date> --json
```

### Standard Response Envelope

All JSON responses follow a consistent envelope:

**Success response:**
```json
{
  "ok": true,
  "data": { ... },
  "meta": {
    "count": 42,
    "query_time_ms": 12
  }
}
```

**Error response:**
```json
{
  "ok": false,
  "error": {
    "code": "INVALID_TYPE",
    "message": "Type 'meeting' not found in schema",
    "details": {
      "available_types": ["person", "project", "daily", "page", "section"]
    }
  }
}
```

### List Response Format

For commands returning multiple items:

```json
{
  "ok": true,
  "data": {
    "items": [
      { "id": "...", "type": "...", ... },
      { "id": "...", "type": "...", ... }
    ]
  },
  "meta": {
    "count": 2,
    "total": 150,
    "has_more": true
  }
}
```

### Object Response Format

When returning a single object:

```json
{
  "ok": true,
  "data": {
    "object": {
      "id": "people/alice",
      "type": "person",
      "file_path": "people/alice.md",
      "heading": null,
      "heading_level": null,
      "fields": {
        "name": "Alice Chen",
        "email": "alice@example.com",
        "tags": ["engineering", "platform"]
      },
      "parent": null,
      "line_start": 1,
      "line_end": null,
      "first_seen_at": "2025-01-15T10:30:00Z",
      "last_modified_at": "2025-01-28T14:22:00Z"
    }
  }
}
```

### Trait Response Format

When returning traits:

```json
{
  "ok": true,
  "data": {
    "items": [
      {
        "id": "trait-abc123",
        "trait_type": "due",
        "value": "2025-02-01",
        "content": "Send revised estimate",
        "file_path": "daily/2025-02-01.md",
        "line_number": 23,
        "parent": {
          "id": "daily/2025-02-01#standup",
          "type": "meeting",
          "heading": "Weekly Standup"
        },
        "refs": [
          {
            "target": "people/alice",
            "type": "person",
            "display": "Alice"
          }
        ],
        "other_traits": {
          "priority": "high",
          "assignee": "people/alice"
        },
        "created_at": "2025-01-28T09:15:00Z"
      }
    ]
  },
  "meta": {
    "count": 1
  }
}
```

**Key additions for agents:**
- `parent` object with full context (not just ID)
- `refs` extracted and resolved from content
- `other_traits` shows sibling traits on the same line
- `created_at` timestamp

---

## Input Format Standardization

### JSON Input for Mutations

Create and update commands accept JSON input via `--input` flag or stdin:

```bash
# Via flag
rvn object create --input '{"type": "person", "path": "people/bob", "fields": {"name": "Bob Smith"}}'

# Via stdin
echo '{"type": "person", "path": "people/bob", "fields": {"name": "Bob Smith"}}' | rvn object create --stdin

# Via file
rvn object create --input-file new-person.json
```

### Create Object Input Schema

```json
{
  "type": "person",
  "path": "people/bob",
  "fields": {
    "name": "Bob Smith",
    "email": "bob@example.com"
  },
  "content": "# Bob Smith\n\nNotes about Bob..."
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `type` | Yes | Object type (must exist in schema) |
| `path` | No | File path (without `.md`). If omitted, derived from `default_path` + slugified name |
| `fields` | No | Field values for frontmatter |
| `content` | No | Markdown content after frontmatter |

### Create Trait Input Schema

```json
{
  "trait_type": "due",
  "value": "2025-02-15",
  "parent_id": "daily/2025-02-01#standup",
  "content": "Review the proposal"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `trait_type` | Yes | Trait name (must exist in schema) |
| `value` | Depends | Trait value (required unless boolean trait) |
| `parent_id` | Yes | Object ID to attach trait to |
| `content` | Yes | The content/text this trait annotates |
| `other_traits` | No | Additional traits on the same line |

### Update Input Schema

```json
{
  "id": "people/alice",
  "set": {
    "email": "alice.chen@newcompany.com"
  },
  "unset": ["phone"]
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Object or trait ID |
| `set` | No | Fields to add or update |
| `unset` | No | Fields to remove |

---

## Error Handling

### Error Codes

Standardized error codes for programmatic handling:

| Code | HTTP-like | Description |
|------|-----------|-------------|
| `NOT_FOUND` | 404 | Object/trait/file not found |
| `INVALID_TYPE` | 400 | Type not in schema |
| `INVALID_TRAIT` | 400 | Trait not in schema |
| `INVALID_FIELD` | 400 | Field validation failed |
| `INVALID_VALUE` | 400 | Value doesn't match expected type |
| `INVALID_REF` | 400 | Reference doesn't resolve |
| `DUPLICATE_ID` | 409 | ID already exists |
| `MISSING_REQUIRED` | 400 | Required field not provided |
| `AMBIGUOUS_REF` | 400 | Short reference matches multiple objects |
| `SCHEMA_ERROR` | 500 | Schema file is invalid |
| `INDEX_ERROR` | 500 | Database error |
| `FILE_ERROR` | 500 | File system error |

### Error Response Structure

```json
{
  "ok": false,
  "error": {
    "code": "INVALID_FIELD",
    "message": "Field 'rating' must be between 1 and 5",
    "details": {
      "field": "rating",
      "value": 10,
      "constraint": {
        "min": 1,
        "max": 5
      }
    },
    "location": {
      "file": "books/some-book.md",
      "line": 5
    }
  }
}
```

### Validation Errors (Multiple)

When `rvn check` or create/update finds multiple issues:

```json
{
  "ok": false,
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "3 validation errors found",
    "errors": [
      {
        "code": "MISSING_REQUIRED",
        "message": "Required field 'name' is missing",
        "location": {"file": "people/bob.md", "line": 1}
      },
      {
        "code": "INVALID_REF",
        "message": "Reference [[alice]] is ambiguous",
        "location": {"file": "people/bob.md", "line": 15},
        "details": {
          "matches": ["people/alice", "clients/alice"]
        }
      },
      {
        "code": "INVALID_VALUE",
        "message": "Invalid enum value 'urgent' for field 'priority'",
        "location": {"file": "people/bob.md", "line": 20},
        "details": {
          "allowed": ["low", "medium", "high"]
        }
      }
    ]
  }
}
```

---

## Schema Introspection

Agents need to discover what's possible. Add schema query commands:

### List Types

```bash
rvn schema types --json
```

Response:
```json
{
  "ok": true,
  "data": {
    "types": [
      {
        "name": "person",
        "builtin": false,
        "default_path": "people/",
        "fields": ["name", "email", "company"],
        "traits": ["due", "priority"],
        "required_fields": ["name"]
      },
      {
        "name": "meeting",
        "builtin": false,
        "default_path": null,
        "fields": ["time", "attendees"],
        "traits": ["remind"],
        "required_fields": []
      },
      {
        "name": "page",
        "builtin": true,
        "default_path": null,
        "fields": [],
        "traits": [],
        "required_fields": []
      },
      {
        "name": "section",
        "builtin": true,
        "default_path": null,
        "fields": ["title", "level"],
        "traits": [],
        "required_fields": []
      }
    ]
  }
}
```

### Get Type Details

```bash
rvn schema type person --json
```

Response:
```json
{
  "ok": true,
  "data": {
    "type": {
      "name": "person",
      "builtin": false,
      "default_path": "people/",
      "fields": {
        "name": {
          "type": "string",
          "required": true
        },
        "email": {
          "type": "string",
          "required": false
        },
        "company": {
          "type": "ref",
          "target": "company",
          "required": false
        }
      },
      "traits": {
        "due": {
          "required": false,
          "default": null
        },
        "priority": {
          "required": false,
          "default": "medium"
        }
      }
    }
  }
}
```

### List Traits

```bash
rvn schema traits --json
```

Response:
```json
{
  "ok": true,
  "data": {
    "traits": [
      {
        "name": "due",
        "type": "date",
        "default": null
      },
      {
        "name": "priority",
        "type": "enum",
        "values": ["low", "medium", "high"],
        "default": "medium"
      },
      {
        "name": "status",
        "type": "enum",
        "values": ["todo", "in_progress", "done", "blocked"],
        "default": "todo"
      },
      {
        "name": "highlight",
        "type": "boolean",
        "default": false
      },
      {
        "name": "assignee",
        "type": "ref",
        "target": null
      }
    ]
  }
}
```

### Get Trait Details

```bash
rvn schema trait due --json
```

Response:
```json
{
  "ok": true,
  "data": {
    "trait": {
      "name": "due",
      "type": "date",
      "default": null,
      "used_by_types": ["person", "project"],
      "description": "Due date for tasks and deadlines"
    }
  }
}
```

### Full Schema Dump

```bash
rvn schema --json
```

Returns complete schema including types, traits, saved queries, and vault config.

### Available Commands

Agents can discover available commands and their capabilities:

```bash
rvn schema commands --json
```

Response:
```json
{
  "ok": true,
  "data": {
    "commands": {
      "read": {
        "description": "Read raw file content",
        "args": ["path"],
        "examples": ["rvn read daily/2025-02-01.md --json"]
      },
      "add": {
        "description": "Append content to any file in the vault",
        "default_target": "Today's daily note",
        "flags": {
          "--to": {
            "type": "path",
            "description": "Target file path (any file in vault)",
            "examples": [
              "projects/website.md",
              "inbox.md",
              "people/alice.md"
            ]
          }
        },
        "use_cases": [
          "Quick capture to daily note",
          "Add tasks to project files",
          "Append notes to any document",
          "Insert references to build knowledge graph"
        ],
        "examples": [
          "rvn add \"Quick thought\" --json",
          "rvn add \"New task\" --to projects/website.md --json",
          "rvn add \"@priority(high) Urgent\" --to inbox.md --json",
          "rvn add \"Discussed [[projects/mobile-app]]\" --json"
        ]
      },
      "object": {
        "subcommands": ["list", "get", "create", "update", "delete"],
        "description": "CRUD operations on typed objects"
      },
      "trait": {
        "subcommands": ["list", "get", "create", "update", "delete"],
        "description": "Query and manage traits"
      },
      "query": {
        "description": "Run saved queries",
        "examples": ["rvn query tasks --json", "rvn query overdue --json"]
      },
      "backlinks": {
        "description": "Find objects that reference a target",
        "examples": ["rvn backlinks people/alice --json"]
      },
      "type": {
        "description": "List objects by type",
        "examples": ["rvn type person --json", "rvn type meeting --json"]
      },
      "tag": {
        "description": "Query objects by tags",
        "examples": ["rvn tag important --json", "rvn tag --list --json"]
      },
      "date": {
        "description": "Date hub - all activity for a date",
        "examples": ["rvn date today --json", "rvn date 2025-02-01 --json"]
      }
    }
  }
}
```

This allows agents to discover what operations are available without hardcoding command knowledge.

---

## Mutation Commands

The current spec is read-heavy. Add full CRUD for agents:

### Object Commands

```bash
# Create
rvn object create --type person --path people/bob --field name="Bob Smith" --json
rvn object create --input '{"type": "person", ...}' --json

# Read
rvn object get people/alice --json
rvn object get daily/2025-02-01#standup --json

# Update (using rvn set)
rvn set people/alice email="new@email.com" --json

# Delete
rvn object delete people/old-contact --json

# List
rvn object list --json
rvn object list --type person --json
rvn object list --type meeting --field attendees=people/alice --json
```

### Trait Commands (Extended)

```bash
# Create (add trait to existing content)
rvn trait create --type due --value 2025-02-15 --parent daily/2025-02-01#standup --content "Review proposal" --json

# Read
rvn trait get trait-abc123 --json

# Update
rvn trait update trait-abc123 --set value=2025-02-20 --json
rvn trait update trait-abc123 --set status=done --json

# Delete
rvn trait delete trait-abc123 --json

# List (existing, enhanced)
rvn trait due --json
rvn trait due --value today --json
rvn trait status --value todo --json
```

### Read Command

A simple, read-only command to dump file contents for agent context-gathering:

```bash
rvn read daily/2025-02-01.md --json
```

**Response:**
```json
{
  "ok": true,
  "data": {
    "path": "daily/2025-02-01.md",
    "content": "---\ntype: date\n---\n\n# Thursday, February 1, 2025\n\n## Tasks\n\n- @due(2025-02-03) Send estimate...",
    "line_count": 45
  }
}
```

**Why this exists:**
- Read-only — no mutation concerns
- Full context — agent sees raw markdown, not just extracted fields
- Simple — just read and return
- Enables summarization, context-gathering, understanding structure

**Difference from `rvn object get`:**
- `rvn object get` → structured data (type, fields, line numbers)
- `rvn read` → raw markdown content (for full context or summarization)

### Quick Capture (`rvn add`)

The existing `rvn add` command is enhanced for agent use with reference validation:

```bash
# Default: append to daily note
rvn add "Met with [[people/bob]] about [[projects/website]]" --json

# Append to ANY file with --to
rvn add "New feature idea: dark mode" --to projects/website.md --json
rvn add "@priority(high) Review contract" --to inbox.md --json
rvn add "Follow up on proposal" --to people/alice.md --json
```

**Response (with reference validation):**
```json
{
  "ok": true,
  "data": {
    "file": "daily/2025-02-01.md",
    "line": 45,
    "content": "- 14:30 Met with [[people/bob]] about [[projects/website]]"
  },
  "warnings": [
    {
      "code": "REF_NOT_FOUND",
      "ref": "people/bob",
      "suggested_type": "person",
      "create_command": "rvn object create person --title \"bob\" --json"
    }
  ]
}
```

**Key features for agents:**
- **`--to` flag**: Append to ANY file in the vault (not just daily notes)
- **Reference validation**: Warns about broken refs with suggested fix
- **Trait preservation**: Inline traits like `@priority(high)` are parsed on reindex
- **Auto-reindex**: Configurable, ensures new content is immediately queryable

**Use cases:**
- Quick capture to daily note
- Add tasks to project files
- Append notes to any document
- Build the knowledge graph by inserting references

### File Commands (Deferred)

> **Note**: Raw file write/append commands are deferred for MVP. Use structured commands (`rvn object create/update`, `rvn trait create/update`, `rvn add`) instead to reduce error surface.

See `docs/FUTURE.md` for details on potential future `rvn file write/append` commands.

### Response Format for Mutations

All mutations return the affected object/trait:

```json
{
  "ok": true,
  "data": {
    "action": "created",
    "object": {
      "id": "people/bob",
      "type": "person",
      "fields": {"name": "Bob Smith"},
      ...
    }
  },
  "meta": {
    "file_modified": "people/bob.md",
    "reindex_required": false
  }
}
```

---

## Timestamps and Temporal Queries

### Design Constraint

A core principle of Raven is that **text files are the source of truth** and the database is disposable. This means we can't rely solely on database timestamps—they'd be lost on reindex.

File modification times (mtime) are also unreliable:
- Editing any line updates mtime for everything in the file
- Git clone/sync can reset mtimes
- No granularity below file level

### Solution: Audit Log

An append-only log file tracks all operations with timestamps:

```
.raven/audit.log
```

This file is:
- **Append-only** — Raven only adds to it, never modifies
- **Plain text** — Human-readable JSONL format
- **Recoverable** — Survives database deletion
- **Not user-edited** — Users should not modify this file

### Audit Log Format

JSONL (one JSON object per line):

```jsonl
{"ts":"2025-02-01T10:30:00Z","op":"create","entity":"object","id":"people/bob","type":"person"}
{"ts":"2025-02-01T10:32:00Z","op":"create","entity":"trait","id":"due-a3f2b1","parent":"daily/2025-02-01","trait":"due","value":"2025-02-05","content":"Review proposal"}
{"ts":"2025-02-01T14:15:00Z","op":"update","entity":"trait","id":"due-a3f2b1","changes":{"value":{"old":"2025-02-05","new":"2025-02-10"}}}
{"ts":"2025-02-01T16:00:00Z","op":"delete","entity":"object","id":"people/old-contact"}
{"ts":"2025-02-01T18:00:00Z","op":"reindex","discovered":["trait:due-xyz"],"removed":["trait:due-old123"]}
```

### What Gets Logged

| Event | Op Type | Precision |
|-------|---------|-----------|
| Object created via `rvn` | `create` | Exact timestamp |
| Object updated via `rvn` | `update` | Exact timestamp |
| Trait created via `rvn` | `create` | Exact timestamp |
| Trait updated via `rvn` | `update` | Exact timestamp |
| Entity deleted via `rvn` | `delete` | Exact timestamp |
| User edits file directly, then reindex | `reindex.discovered` | Reindex timestamp (not edit time) |
| Content removed externally, then reindex | `reindex.removed` | Reindex timestamp |

### Handling External Edits

When users edit files outside of `rvn`, changes are detected on reindex:

```jsonl
{"ts":"2025-02-01T18:00:00Z","op":"reindex","file":"daily/2025-02-01.md","discovered":["trait:due-abc"],"removed":["trait:due-xyz"],"modified":["object:daily/2025-02-01#standup"]}
```

This is honest about what we know: we can't tell exactly when the user made the edit, only when we discovered it.

### Cached Timestamps in Database

For fast queries, timestamps are cached in the database (rebuilt from audit log on reindex):

```sql
CREATE TABLE entity_timestamps (
    entity_type TEXT NOT NULL,      -- 'object' or 'trait'
    entity_id TEXT NOT NULL,
    first_seen_at INTEGER NOT NULL, -- Unix timestamp
    last_modified_at INTEGER NOT NULL,
    last_op TEXT,                   -- 'create', 'update', 'reindex.discovered'
    PRIMARY KEY (entity_type, entity_id)
);

CREATE INDEX idx_timestamps_first_seen ON entity_timestamps(first_seen_at);
CREATE INDEX idx_timestamps_last_modified ON entity_timestamps(last_modified_at);
```

On reindex:
1. Parse audit log file
2. Rebuild `entity_timestamps` table
3. For entities not in log (pre-existing), use file mtime as fallback

### Temporal Query Filters

```bash
# Objects created/modified in time range
rvn object list --created-after 2025-01-20 --json
rvn object list --modified-today --json
rvn object list --modified-since "2 days ago" --json

# Traits created in time range
rvn trait due --created-after 2025-01-20 --json

# Combined filters
rvn trait status --value todo --created-this-week --json
```

### Temporal Filter Syntax

| Filter | Meaning |
|--------|---------|
| `--created-today` | First seen today |
| `--created-this-week` | First seen this week |
| `--created-after YYYY-MM-DD` | First seen after date |
| `--created-before YYYY-MM-DD` | First seen before date |
| `--created-since "N days ago"` | First seen in last N days |
| `--modified-today` | Last modified today |
| `--modified-this-week` | Last modified this week |
| `--modified-after YYYY-MM-DD` | Last modified after date |
| `--modified-before YYYY-MM-DD` | Last modified before date |
| `--modified-since "N days ago"` | Last modified in last N days |

### Querying the Audit Log Directly

For detailed history:

```bash
# Recent activity
rvn log --since yesterday --json

# Activity on specific entity
rvn log --id people/alice --json

# Specific operation types
rvn log --op create --entity trait --json

# Full history export
rvn log --all --json
```

### Log Response Format

```json
{
  "ok": true,
  "data": {
    "entries": [
      {
        "ts": "2025-02-01T10:30:00Z",
        "op": "create",
        "entity": "object",
        "id": "people/bob",
        "type": "person"
      },
      {
        "ts": "2025-02-01T10:32:00Z",
        "op": "create",
        "entity": "trait",
        "id": "due-a3f2b1",
        "parent": "daily/2025-02-01",
        "trait": "due",
        "value": "2025-02-05"
      }
    ]
  },
  "meta": {
    "count": 2,
    "from": "2025-02-01T00:00:00Z",
    "to": "2025-02-01T23:59:59Z"
  }
}
```

### Log Rotation

For long-running vaults, logs can be rotated:

```
.raven/audit.log              # Current
.raven/audit.2025-01.log      # Archived by month
.raven/audit.2024-12.log
```

Rotation is optional and manual. The log compresses well if users want to archive old entries.

### Use Case: "What's New Since We Last Talked"

```bash
# Agent can query for recent changes
rvn object list --modified-since "24 hours ago" --json
rvn trait due --created-since "24 hours ago" --json

# Or check the raw log for detailed activity
rvn log --since "24 hours ago" --json
```

### Limitations for Agents to Understand

1. **External edits are imprecise** — If user edits a file in their editor, we only know when reindex discovered it, not when they actually made the edit.

2. **Pre-audit-log content** — Content that existed before Raven was initialized won't have creation timestamps. Falls back to file mtime.

3. **Log is append-only** — No "undo" capability. The log records what happened, not previous states.

4. **Reindex required for external changes** — If user edits files and doesn't run `rvn reindex`, changes won't appear in the log until next reindex.

---

## Rich Context in Responses

### Problem

Minimal responses force agents to make multiple calls:

```bash
# Call 1: Get task
rvn trait due --value today --json
# Returns: {"parent_object_id": "daily/2025-02-01#standup", ...}

# Call 2: Agent needs to understand parent
rvn object get daily/2025-02-01#standup --json
# Returns: {"type": "meeting", "fields": {"attendees": [...]}}

# Call 3: Agent needs to resolve attendee refs
rvn object get people/alice --json
```

### Solution: Include Context by Default

Trait responses include resolved parent and refs:

```json
{
  "ok": true,
  "data": {
    "items": [
      {
        "id": "trait-abc123",
        "trait_type": "due",
        "value": "2025-02-01",
        "content": "@due(2025-02-01) @assignee([[people/alice]]) Send estimate",
        "content_text": "Send estimate",
        "file_path": "daily/2025-02-01.md",
        "line_number": 23,
        
        "parent": {
          "id": "daily/2025-02-01#standup",
          "type": "meeting",
          "heading": "Weekly Standup",
          "fields": {
            "time": "09:00",
            "attendees": ["people/alice", "people/bob"]
          }
        },
        
        "file_root": {
          "id": "daily/2025-02-01",
          "type": "daily",
          "fields": {
            "date": "2025-02-01",
            "tags": ["work"]
          }
        },
        
        "refs": [
          {
            "target_id": "people/alice",
            "target_type": "person",
            "target_name": "Alice Chen",
            "display_text": null
          }
        ],
        
        "sibling_traits": {
          "assignee": "people/alice",
          "priority": "high"
        },
        
        "first_seen_at": "2025-01-28T09:15:00Z",
        "last_modified_at": "2025-01-28T09:15:00Z"
      }
    ]
  }
}
```

**Key additions:**
- `content_text`: Content with trait annotations stripped (just the human text)
- `parent`: Full parent object, not just ID
- `file_root`: The file-level object (useful for daily notes context)
- `refs`: Resolved references with target details
- `sibling_traits`: Other traits on the same line
- `first_seen_at`: When this trait was first indexed (from audit log)
- `last_modified_at`: When this trait was last changed (from audit log)

### Slim Mode

For bulk operations where context isn't needed:

```bash
rvn trait due --json --slim
```

Returns minimal data:
```json
{
  "ok": true,
  "data": {
    "items": [
      {"id": "trait-abc123", "trait_type": "due", "value": "2025-02-01", "parent_id": "daily/2025-02-01#standup"}
    ]
  }
}
```

### Depth Control

Control how much context to include:

```bash
rvn trait due --json --depth 0   # Same as --slim
rvn trait due --json --depth 1   # Include parent (default)
rvn trait due --json --depth 2   # Include parent + parent's parent + resolved refs
```

---

## ID Stability

### The Problem

If IDs change between reindexes, agent workflows break:

```bash
# Agent stores task ID
task_id = "trait-abc123"

# User edits file, adds a line above the task
# Reindex runs

# Agent tries to mark task done
rvn trait update trait-abc123 --set status=done
# ERROR: Not found (ID changed to trait-def456)
```

### Solution: Content-Anchored IDs

**For objects:**
- File-level: ID = file path (stable unless file renamed)
- Embedded: ID = file path + `#` + explicit `id` field (stable, user-controlled)
- Sections: ID = file path + `#` + slug (stable unless heading renamed)

**For traits:**
- Generate ID from: `parent_id` + `trait_type` + content hash
- Content hash uses first N significant characters of content
- This means the same trait text in the same location = same ID

### Trait ID Generation

```go
func generateTraitID(parentID, traitType, content string) string {
    // Normalize content: trim, remove other trait annotations
    normalized := normalizeContent(content)
    
    // Take first 50 chars for hash stability
    if len(normalized) > 50 {
        normalized = normalized[:50]
    }
    
    // Hash: parent + type + content
    hash := sha256(parentID + ":" + traitType + ":" + normalized)
    
    // Return short, readable ID
    return fmt.Sprintf("%s-%s", traitType, hash[:8])
}
```

**Stability guarantees:**
- Same content in same location = same ID
- Moving content to different parent = new ID (unavoidable)
- Editing content significantly = new ID (unavoidable)
- Adding/removing other traits on same line = same ID (content normalized)

### ID Persistence Across Reindex

Store generated IDs and attempt to match on reindex:

```sql
CREATE TABLE id_mapping (
    generated_id TEXT PRIMARY KEY,
    file_path TEXT NOT NULL,
    line_number INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    first_seen_at INTEGER NOT NULL
);
```

On reindex:
1. Parse file, generate IDs for traits
2. Check if ID exists in mapping
3. If content hash matches, keep ID
4. If content hash differs but line is close (+/- 5 lines), keep ID (content edited)
5. Otherwise, generate new ID

### Explicit Trait IDs (Escape Hatch)

For critical traits that must be stable, allow explicit IDs:

```markdown
- @due(2025-02-01) @id(important-task) Complete the critical thing
```

The `@id(...)` annotation assigns a stable ID that survives any edit.

---

## Batch Operations

### The Problem

Agents often need to do multiple things atomically:
- Create a meeting with multiple tasks
- Update several traits at once
- Move items between objects

### Solution: Batch Command

```bash
rvn batch --input operations.json --json
rvn batch --stdin --json < operations.json
```

### Batch Input Format

```json
{
  "operations": [
    {
      "op": "create",
      "entity": "object",
      "data": {
        "type": "meeting",
        "path": "meetings/planning",
        "fields": {"time": "2025-02-01T14:00"}
      }
    },
    {
      "op": "create",
      "entity": "trait",
      "data": {
        "trait_type": "due",
        "value": "2025-02-05",
        "parent_id": "meetings/planning",
        "content": "Send agenda"
      }
    },
    {
      "op": "update",
      "entity": "trait",
      "data": {
        "id": "due-abc123",
        "set": {"value": "2025-02-10"}
      }
    }
  ],
  "options": {
    "atomic": true,
    "stop_on_error": true
  }
}
```

### Batch Options

| Option | Default | Description |
|--------|---------|-------------|
| `atomic` | `true` | Roll back all changes if any operation fails |
| `stop_on_error` | `true` | Stop processing on first error |
| `dry_run` | `false` | Validate without making changes |

### Batch Response

```json
{
  "ok": true,
  "data": {
    "results": [
      {
        "index": 0,
        "op": "create",
        "ok": true,
        "data": {"id": "meetings/planning", "type": "meeting"}
      },
      {
        "index": 1,
        "op": "create",
        "ok": true,
        "data": {"id": "due-xyz789", "trait_type": "due"}
      },
      {
        "index": 2,
        "op": "update",
        "ok": true,
        "data": {"id": "due-abc123", "updated_fields": ["value"]}
      }
    ]
  },
  "meta": {
    "total_operations": 3,
    "successful": 3,
    "failed": 0
  }
}
```

### Batch Error Response

```json
{
  "ok": false,
  "error": {
    "code": "BATCH_FAILED",
    "message": "Operation 2 failed: trait not found",
    "failed_at": 2
  },
  "data": {
    "results": [
      {"index": 0, "op": "create", "ok": true, "data": {...}},
      {"index": 1, "op": "create", "ok": true, "data": {...}},
      {"index": 2, "op": "update", "ok": false, "error": {"code": "NOT_FOUND", ...}}
    ],
    "rolled_back": true
  }
}
```

---

## Validation and Dry Run

### Pre-Flight Validation

Validate inputs before making changes:

```bash
rvn validate --type object --input '{"type": "person", "fields": {"name": "Bob"}}' --json
```

Response (valid):
```json
{
  "ok": true,
  "data": {
    "valid": true,
    "normalized": {
      "type": "person",
      "path": "people/bob",
      "fields": {"name": "Bob"}
    }
  }
}
```

Response (invalid):
```json
{
  "ok": true,
  "data": {
    "valid": false,
    "errors": [
      {
        "code": "MISSING_REQUIRED",
        "field": "email",
        "message": "Field 'email' is required for type 'person'"
      }
    ]
  }
}
```

### Dry Run Mode

Preview changes without committing:

```bash
rvn object create --type person --field name="Bob" --dry-run --json
```

Response:
```json
{
  "ok": true,
  "data": {
    "dry_run": true,
    "would_create": {
      "id": "people/bob",
      "type": "person",
      "fields": {"name": "Bob"}
    },
    "would_modify_files": [
      {
        "path": "people/bob.md",
        "action": "create",
        "content_preview": "---\ntype: person\nname: Bob\n---\n\n# Bob\n"
      }
    ],
    "side_effects": []
  }
}
```

### Dry Run for Updates

```bash
rvn trait update due-abc123 --set value=2025-02-20 --dry-run --json
```

Response:
```json
{
  "ok": true,
  "data": {
    "dry_run": true,
    "would_update": {
      "id": "due-abc123",
      "changes": {
        "value": {
          "old": "2025-02-01",
          "new": "2025-02-20"
        }
      }
    },
    "would_modify_files": [
      {
        "path": "daily/2025-02-01.md",
        "action": "modify",
        "line": 23,
        "old_content": "- @due(2025-02-01) Send estimate",
        "new_content": "- @due(2025-02-20) Send estimate"
      }
    ]
  }
}
```

---

## MCP Server Integration

### Overview

MCP (Model Context Protocol) allows Claude and other LLMs to call tools directly. Raven should expose itself as an MCP server.

```bash
rvn serve --mcp
```

### MCP Tool Definitions

Each CLI command becomes an MCP tool:

```json
{
  "name": "raven_trait_list",
  "description": "List traits (due dates, priorities, status, etc.) with optional filters. Use this to find tasks, reminders, and other annotated content.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "trait_type": {
        "type": "string",
        "description": "Trait to query: due, priority, status, highlight, remind, assignee"
      },
      "value": {
        "type": "string",
        "description": "Filter by value. For dates: today, tomorrow, this-week, past, YYYY-MM-DD"
      },
      "parent_type": {
        "type": "string",
        "description": "Filter by parent object type: meeting, daily, project"
      },
      "created_since": {
        "type": "string",
        "description": "Filter by creation time: today, this-week, YYYY-MM-DD, 'N days ago'"
      }
    },
    "required": ["trait_type"]
  }
}
```

```json
{
  "name": "raven_trait_create",
  "description": "Create a new trait annotation. Use this to add tasks, reminders, or other metadata to content.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "trait_type": {
        "type": "string",
        "description": "Trait type: due, priority, status, highlight, remind, assignee"
      },
      "value": {
        "type": "string",
        "description": "Trait value. For due: YYYY-MM-DD. For priority: low/medium/high."
      },
      "parent_id": {
        "type": "string",
        "description": "Object ID to attach trait to (e.g., daily/2025-02-01 or daily/2025-02-01#standup)"
      },
      "content": {
        "type": "string",
        "description": "The text content this trait annotates"
      }
    },
    "required": ["trait_type", "parent_id", "content"]
  }
}
```

```json
{
  "name": "raven_object_get",
  "description": "Get details about a specific object (person, project, meeting, etc.)",
  "inputSchema": {
    "type": "object",
    "properties": {
      "id": {
        "type": "string",
        "description": "Object ID (e.g., people/alice, daily/2025-02-01#standup)"
      }
    },
    "required": ["id"]
  }
}
```

```json
{
  "name": "raven_schema_types",
  "description": "List available object types in the vault. Call this to understand what kinds of objects exist.",
  "inputSchema": {
    "type": "object",
    "properties": {}
  }
}
```

### Full MCP Tool List

| Tool | Description |
|------|-------------|
| `raven_schema_types` | List available types |
| `raven_schema_traits` | List available traits |
| `raven_schema_type` | Get type details |
| `raven_schema_trait` | Get trait details |
| `raven_object_list` | List objects with filters |
| `raven_read` | Read raw file content |
| `raven_new` | Create new typed object |
| `raven_set` | Update frontmatter fields |
| `raven_delete` | Delete object |
| `raven_trait_list` | List traits with filters |
| `raven_trait_get` | Get single trait |
| `raven_trait_create` | Create new trait |
| `raven_trait_update` | Update trait |
| `raven_trait_delete` | Delete trait |
| `raven_backlinks` | Get backlinks to object |
| `raven_query` | Run saved query |
| `raven_date` | Get date hub (everything for a date) |
| `raven_log` | Query audit log for recent activity |
| `raven_batch` | Execute batch operations |

### MCP Server Config

Users add to Claude config:

```json
{
  "mcpServers": {
    "raven": {
      "command": "rvn",
      "args": ["serve", "--mcp"],
      "env": {
        "RAVEN_VAULT": "/Users/me/vault"
      }
    }
  }
}
```

---

## Command Reference Updates

### New Commands Summary

```bash
# Schema introspection
rvn schema --json                    # Full schema dump
rvn schema types --json              # List types
rvn schema type <name> --json        # Type details
rvn schema traits --json             # List traits
rvn schema trait <name> --json       # Trait details

# Object CRUD
rvn object list [--type TYPE] [--json]
rvn read <path> [--json]
rvn new <type> [title] [--field KEY=VAL]... [--json]
rvn set <id> KEY=VAL... [--json]
rvn delete <id> [--force] [--json]

# Trait CRUD
rvn trait <name> [filters] [--json]  # List (existing)
rvn trait get <id> [--json]          # Get single trait
rvn trait create --type TYPE --parent ID --content TEXT [--json]
rvn trait update <id> --set KEY=VAL [--json]
rvn trait delete <id> [--json]

# Read file content
rvn read <path> [--json]           # Raw markdown content for context

# Quick capture (append to any file)
rvn add <text> [--to PATH] [--json]  # Default: daily note, or any file via --to

# Audit log (temporal queries)
rvn log --since TIMESPEC [--json]           # Recent activity
rvn log --id ENTITY_ID [--json]             # History for specific entity
rvn log --op create|update|delete [--json]  # Filter by operation
rvn log --entity object|trait [--json]      # Filter by entity type
rvn log --all [--json]                      # Full history export

# Validation
rvn validate --type object|trait --input JSON [--json]

# Batch
rvn batch --input JSON [--atomic] [--dry-run] [--json]

# MCP Server
rvn serve --mcp [--port PORT]
```

### Global Flags

| Flag | Description |
|------|-------------|
| `--json` | Output in JSON format |
| `--slim` | Minimal output (no context) |
| `--depth N` | Context depth (0=slim, 1=default, 2=full) |
| `--dry-run` | Preview changes without committing |
| `--vault PATH` | Override vault path |

---

## Implementation Priority

### Phase 1: Foundation (Do First)

1. Standard JSON response envelope
2. Error codes and structured errors
3. `--json` flag on all existing commands
4. Audit log infrastructure (`.raven/audit.log`)
5. `rvn read <path> --json` — Read raw file content
6. Enhanced `rvn add` — Reference validation with warnings, `--to` for any file

### Phase 2: Introspection

1. `rvn schema types/traits/type/trait` commands
2. `rvn validate` command

### Phase 3: Mutations

1. `rvn new`, `rvn set`, `rvn delete` — object CRUD
2. `rvn add` — quick capture with reference validation
3. All mutations write to audit log

> **Note**: `rvn file write/append` are deferred. See `docs/FUTURE.md`.

### Phase 4: Advanced

1. Batch operations
2. Dry run mode
3. Rich context responses
4. Temporal query filters (`rvn log`, `--created-since`, etc.)

### Phase 5: Integration

1. MCP server implementation
2. Tool definitions
3. Documentation for agent developers

---

## Appendix: Example Agent Interactions

### "Add a task for tomorrow"

```
User: "Remind me to review Alice's PR tomorrow"

Agent thinks: Need to create a due trait for tomorrow

Agent calls: raven_trait_create({
  "trait_type": "due",
  "value": "2025-02-02",
  "parent_id": "daily/2025-02-01",
  "content": "Review Alice's PR"
})

Response: {
  "ok": true,
  "data": {
    "id": "due-a3f2b1",
    "trait_type": "due",
    "value": "2025-02-02",
    "content": "Review Alice's PR",
    "parent": {"id": "daily/2025-02-01", "type": "daily"}
  }
}

Agent: "Done! I've added 'Review Alice's PR' to your tasks for tomorrow."
```

### "What's on my plate this week?"

```
User: "What do I need to do this week?"

Agent thinks: Query for due traits this week

Agent calls: raven_trait_list({
  "trait_type": "due",
  "value": "this-week"
})

Response: {
  "ok": true,
  "data": {
    "items": [
      {
        "id": "due-a3f2b1",
        "value": "2025-02-02",
        "content_text": "Review Alice's PR",
        "parent": {"type": "daily", "fields": {"date": "2025-02-01"}}
      },
      {
        "id": "due-b4c3d2",
        "value": "2025-02-05",
        "content_text": "Submit quarterly report",
        "sibling_traits": {"priority": "high"},
        "parent": {"type": "project", "heading": "Q1 Planning"}
      }
    ]
  },
  "meta": {"count": 2}
}

Agent: "You have 2 tasks this week:
- Tomorrow (Feb 2): Review Alice's PR
- Friday (Feb 5): Submit quarterly report (high priority, from Q1 Planning project)"
```

### "What did I discuss with Bob recently?"

```
User: "What have I discussed with Bob recently?"

Agent thinks: Find meetings with Bob, plus backlinks

Agent calls: raven_object_list({
  "type": "meeting",
  "field": "attendees=people/bob",
  "created_since": "2 weeks ago"
})

Response: {
  "ok": true,
  "data": {
    "items": [
      {
        "id": "daily/2025-01-28#one-on-one",
        "type": "meeting",
        "heading": "1:1 with Bob",
        "fields": {"time": "14:00"},
        "file_root": {"fields": {"date": "2025-01-28"}}
      }
    ]
  }
}

Agent calls: raven_backlinks({"target": "people/bob"})

Response: {
  "ok": true,
  "data": {
    "items": [
      {"source_id": "daily/2025-01-28#one-on-one", "file_path": "daily/2025-01-28.md"},
      {"source_id": "projects/mobile-app", "file_path": "projects/mobile-app.md"}
    ]
  }
}

Agent: "I found a 1:1 meeting with Bob on January 28th. He's also mentioned in the Mobile App project. Would you like me to show the details from that meeting?"
```

### "Mark that task as done"

```
User: "Mark the PR review as done"

Agent thinks: Need to find and update the task

Agent calls: raven_trait_list({
  "trait_type": "due",
  "value": "today"
})

Response: {
  "ok": true,
  "data": {
    "items": [
      {"id": "due-a3f2b1", "content_text": "Review Alice's PR", ...}
    ]
  }
}

Agent calls: raven_trait_update({
  "id": "due-a3f2b1",
  "set": {"status": "done"}
})

Response: {
  "ok": true,
  "data": {
    "id": "due-a3f2b1",
    "updated_fields": ["status"],
    "new_values": {"status": "done"}
  }
}

Agent: "Done! I've marked 'Review Alice's PR' as complete."
```

### "Add some tasks to my website project"

```
User: "Add a couple tasks to the website project - we need to implement dark mode and fix the mobile nav"

Agent thinks: I need to append content to the website project file

Agent calls: raven_add({
  "text": "@priority(medium) Implement dark mode toggle",
  "to": "projects/website.md"
})

Response: {
  "ok": true,
  "data": {
    "file": "projects/website.md",
    "line": 28,
    "content": "- @priority(medium) Implement dark mode toggle"
  }
}

Agent calls: raven_add({
  "text": "@priority(high) Fix mobile navigation menu",
  "to": "projects/website.md"
})

Response: {
  "ok": true,
  "data": {
    "file": "projects/website.md",
    "line": 29,
    "content": "- @priority(high) Fix mobile navigation menu"
  }
}

Agent: "Done! I've added both tasks to your website project:
- Implement dark mode toggle (medium priority)
- Fix mobile navigation menu (high priority)"
```

### "Add a reference and create the person"

```
User: "I met Sarah Chen today, add her to my notes"

Agent thinks: I'll add a reference to today's daily note, then handle the missing person

Agent calls: raven_add({
  "text": "Met with [[people/sarah-chen]] - potential new client"
})

Response: {
  "ok": true,
  "data": {
    "file": "daily/2025-02-01.md",
    "line": 15,
    "content": "- 14:30 Met with [[people/sarah-chen]] - potential new client"
  },
  "warnings": [
    {
      "code": "REF_NOT_FOUND",
      "ref": "people/sarah-chen",
      "suggested_type": "person",
      "create_command": "rvn object create person --title \"sarah-chen\" --json"
    }
  ]
}

Agent thinks: The reference doesn't exist yet - I should create it

Agent calls: raven_object_create({
  "type": "person",
  "title": "Sarah Chen",
  "fields": {}
})

Response: {
  "ok": true,
  "data": {
    "id": "people/sarah-chen",
    "type": "person",
    "file": "people/sarah-chen.md"
  }
}

Agent: "I've added a note about meeting Sarah Chen to your daily notes and created her person page."
```

### "Summarize what happened last week"

```
User: "What happened last week? Give me a summary"

Agent thinks: I'll read the daily notes from last week to get full context

Agent calls: raven_read({"path": "daily/2025-01-27.md"})

Response: {
  "ok": true,
  "data": {
    "path": "daily/2025-01-27.md",
    "content": "---\ntype: date\n---\n\n# Monday, January 27, 2025\n\n## Standup\n\n::meeting\nattendees: [[people/alice]], [[people/bob]]\n\n- Sprint review prep\n- Discussed Q1 roadmap\n\n## Tasks\n\n- @due(2025-01-28) Finish API docs\n- @priority(high) Review Alice's PR\n\n## Notes\n\n- [[projects/mobile-app]] beta testing started\n",
    "line_count": 22
  }
}

Agent calls: raven_read({"path": "daily/2025-01-28.md"})
...

Agent: "Here's what happened last week:

**Monday (Jan 27)**
- Sprint review prep with Alice and Bob
- Mobile app beta testing kicked off
- Worked on API docs

**Tuesday (Jan 28)**
- Finished API documentation
- 1:1 with Bob about the mobile app
..."
```
