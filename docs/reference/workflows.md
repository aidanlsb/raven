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

Inputs are provided at run-time via:
- `--input key=value` (string values)
- `--input-json '{...}'` (typed JSON object)
- `--input-file ./inputs.json` (typed JSON object from file)

Input fields:
- `type`: `string`, `markdown`, `ref`, `date`, `datetime`, `number`, `bool`, `object`, `array`
- `required`: boolean
- `default`: any JSON/YAML scalar/object/array compatible with `type`
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
- persists a run checkpoint under `.raven/workflow-runs/`
- returns `run_id`, `status`, `revision`, `next.prompt`, declared `next.outputs`, and accumulated `steps` output

`rvn workflow continue`:
- loads a persisted run by `run_id`
- validates `{"outputs": ...}` against the awaiting agent step output contract
- resumes deterministic steps after the agent boundary
- pauses again at the next agent step or marks the run `completed`

### Run Retention

Workflow run records are kept using `workflows.runs` settings in `raven.yaml`:
- `storage_path` (default `.raven/workflow-runs`)
- `auto_prune` (default `true`)
- `keep_completed_for_days` (default `7`)
- `keep_failed_for_days` (default `14`)
- `keep_awaiting_for_days` (default `30`)
- `max_runs` (default `1000`)
- `preserve_latest_per_workflow` (default `5`)

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
rvn workflow run <name> --input key=value
rvn workflow run <name> --input-json '{"date":"2026-02-14"}'
rvn workflow continue <run-id> --agent-output-json '{"outputs":{"markdown":"..."}}'
rvn workflow runs list --status awaiting_agent
rvn workflow runs prune --status completed --older-than 14d --confirm
```

