# Workflows Reference

Workflows are reusable **steps-based pipelines** defined in `raven.yaml`.

Workflow v3 uses a breaking, steps-only model:
- top-level `context` / `prompt` / `outputs` are removed
- deterministic work happens in `type: tool` steps
- agent handoff happens at the first `type: agent` step

Raven executes deterministic steps only. It does not call an LLM.

## Definition Location

Workflows live under `workflows:` in `raven.yaml` and are keyed by name.
Definitions can be inline or file-backed (`file:`).

### Inline Definition

```yaml
workflows:
  meeting-prep:
    description: Prepare a brief for a meeting
    inputs:
      meeting_id:
        type: ref
        target: meeting
        required: true
    steps:
      - id: meeting
        type: tool
        tool: raven_read
        arguments:
          path: "{{inputs.meeting_id}}"
          raw: true

      - id: mentions
        type: tool
        tool: raven_backlinks
        arguments:
          target: "{{inputs.meeting_id}}"

      - id: compose
        type: agent
        outputs:
          markdown:
            type: markdown
            required: true
        prompt: |
          Prepare me for this meeting.

          ## Meeting
          {{steps.meeting.data.content}}

          ## Mentions
          {{steps.mentions.data.results}}
```

### File-Backed Definition

```yaml
workflows:
  meeting-prep:
    file: workflows/meeting-prep.yaml
```

External file (same fields, without the workflow name key):

```yaml
description: Prepare a brief for a meeting
inputs:
  meeting_id:
    type: ref
    target: meeting
    required: true
steps:
  - id: meeting
    type: tool
    tool: raven_read
    arguments:
      path: "{{inputs.meeting_id}}"
      raw: true
  - id: compose
    type: agent
    outputs:
      markdown:
        type: markdown
        required: true
    prompt: |
      Prepare me for this meeting.
      {{steps.meeting.data.content}}
```

## Step Types

- `type: tool`
  - `tool`: Raven MCP tool name (for example `raven_query`, `raven_read`, `raven_upsert`)
  - `arguments`: map passed to the tool
- `type: agent`
  - `prompt`: rendered prompt string
  - `outputs`: optional typed output contract for the agent response

## Inputs

Inputs are provided at run-time via `--input key=value`.

Input fields:
- `type`: `string`, `ref`, `date`, `boolean` (validated as strings at run-time)
- `required`: boolean
- `default`: string
- `description`: string
- `target`: string (for `ref` inputs)

## Interpolation

Interpolation is supported in prompts and tool arguments:
- `{{inputs.name}}`
- `{{steps.step_id}}`
- `{{steps.step_id.data.results}}`

Interpolation behavior:
- agent prompts always interpolate to strings
- tool arguments preserve native types when the value is exactly `{{...}}` (array/object/bool/number/string)

## Agent Output Envelope

When an `agent` step declares `outputs`, Raven prepends a strict JSON return contract.
Agent responses should use:

```json
{ "outputs": { "markdown": "..." } }
```

Supported output types:
- `markdown`
- `string`
- `number`
- `bool`
- `object`
- `array`

## Runtime Behavior

`rvn workflow run`:
- validates inputs
- executes tool steps in order
- stops at the first agent step
- returns `next.prompt`, declared `next.outputs`, and accumulated `steps` output

## Migrating Legacy Workflows

Workflow v3 rejects legacy top-level keys: `context`, `prompt`, `outputs`.

Migration map:
- `context.<id>.query` -> `steps: - id: <id>, type: tool, tool: raven_query, arguments.query_string: ...`
- `context.<id>.read` -> `tool: raven_read, arguments.path: ...`
- `context.<id>.search` -> `tool: raven_search, arguments.query: ...`
- `context.<id>.backlinks` -> `tool: raven_backlinks, arguments.target: ...`
- top-level `prompt` -> `steps: - type: agent, prompt: ...`
- top-level `outputs` -> `agent.outputs`

Example migration:

```yaml
# before (legacy)
context:
  results:
    query: "object:project .status==active"
prompt: |
  Summarize:
  {{context.results}}
outputs:
  markdown:
    type: markdown
    required: true
```

```yaml
# after (v3)
steps:
  - id: results
    type: tool
    tool: raven_query
    arguments:
      query_string: "object:project .status==active"
  - id: compose
    type: agent
    outputs:
      markdown:
        type: markdown
        required: true
    prompt: |
      Summarize:
      {{steps.results.data.results}}
```

## Protected Paths

Workflows and plan application refuse to operate on protected/system-managed paths.

Hardcoded:
- `.raven/`, `.trash/`, `.git/`
- `raven.yaml`, `schema.yaml`

Configurable in `raven.yaml`:
- `protected_prefixes: ["templates/", "private/"]`

## CLI Commands

```bash
rvn workflow list
rvn workflow show <name>
rvn workflow run <name> --input key=value --input key2=value2
```

