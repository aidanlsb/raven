# Workflows (reference)

Workflows are reusable prompt templates defined in `raven.yaml`. A workflow can:
- validate inputs (required/defaults)
- gather context (read/query/backlinks/search)
- render a prompt by substituting `{{inputs.*}}` and `{{context.*}}`

## Definition

Workflows live under `workflows:` and are keyed by name.

Inline form:

```yaml
workflows:
  meeting-prep:
    description: Prepare a brief for a meeting
    inputs:
      meeting_id:
        type: ref
        target: meeting
        required: true
        description: Meeting object ID
    context:
      meeting:
        read: "{{inputs.meeting_id}}"
      mentions:
        backlinks: "{{inputs.meeting_id}}"
    prompt: |
      Prepare me for this meeting.

      ## Meeting
      {{context.meeting}}

      ## Mentions
      {{context.mentions}}
```

File-backed form:

```yaml
workflows:
  meeting-prep:
    file: workflows/meeting-prep.yaml
```

External workflow file shape (same fields, without the workflow name key):

```yaml
description: Prepare a brief for a meeting
inputs: {}
context: {}
prompt: |
  ...
```

Rules:
- A workflow must have either `file:` or an inline `prompt:`.
- If both `file:` and inline `prompt:` are present, it’s an error.

## Input types

Workflow input definitions support:
- `type`: `string` | `ref` | `date` | `boolean`
- `required`: boolean
- `default`: string
- `description`: string
- `target`: string (only for `ref`)

Note: inputs are stored/handled as strings at render time; type information is mainly for validation/presentation.

## Context queries

Each context entry must be one of:
- `read: <object-id>`
- `query: "<rql>"`
- `backlinks: <object-id>`
- `search: "<term>"` (optional `limit`, default 20)

`{{inputs.*}}` substitution happens before execution.

If a context query fails, the workflow still renders; the context slot is filled with `{ "error": "..." }`.

## Template substitution

### Inputs

`{{inputs.name}}` is replaced in:
- context queries
- the prompt

Unknown inputs are left as-is (unsubstituted).

### Context

`{{context.key}}` and `{{context.key.path}}` are replaced in the prompt.

Unknown context paths are left as-is (unsubstituted).

### Escaping

Use `\{{` and `\}}` to produce literal `{{` / `}}` in output.

## CLI

```bash
rvn workflow list
rvn workflow show <name>
rvn workflow render <name> --input key=value
```

`--input` is repeatable.

## MCP tools

Workflows are available via:
- `raven_workflow_list`
- `raven_workflow_show`
- `raven_workflow_render`

The render tool accepts `input` as an object (key/value map), which Raven converts to repeatable `--input key=value` flags.

