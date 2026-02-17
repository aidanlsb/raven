# Workflows Reference

Workflows are reusable **steps-based pipelines** defined in workflow YAML files and registered in `raven.yaml`.

Workflow v3 uses a breaking, steps-only model:
- top-level `context` / `prompt` / `outputs` are removed
- deterministic work happens in `type: tool` steps
- agent handoff happens at the first `type: agent` step

Raven executes deterministic steps only. It does not call an LLM.

## Definition Location

Workflows are explicitly registered under `workflows:` in `raven.yaml` and keyed by name.
Each declaration is a file reference (`file:`). Inline workflow bodies in `raven.yaml` are not supported.

Workflow files must live under `directories.workflow` (default `workflows/`).

### Creating via CLI/MCP (recommended for agents)

Instead of editing `raven.yaml` directly, agents can create workflows with:

```bash
rvn workflow scaffold <name>
rvn workflow add <name> --file <directories.workflow>/<name>.yaml
rvn workflow validate [name]
```

This gives immediate validation feedback and avoids YAML indentation mistakes.

### File-Backed Declaration

```yaml
directories:
  workflow: workflows/
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
- returns `run_id`, `status`, `revision`, `next.prompt`, declared `next.outputs`, and lightweight `step_summaries`

`rvn workflow runs step`:
- loads a persisted run and returns full output for one `step_id`
- enables incremental retrieval of large workflow context by step boundary

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

If you have inline workflow definitions in `raven.yaml`, migrate them to files first:

1. Set `directories.workflow` in `raven.yaml` if needed (default `workflows/`)
2. Move each inline workflow body into `<directories.workflow>/<name>.yaml`
3. Replace inline bodies with file references:

```yaml
directories:
  workflow: workflows/
workflows:
  daily-brief:
    file: workflows/daily-brief.yaml
```

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
rvn workflow scaffold <name>
rvn workflow add <name> --file workflows/meeting-prep.yaml
rvn workflow remove <name>
rvn workflow validate [name]
rvn workflow show <name>
rvn workflow run <name> --input key=value
rvn workflow run <name> --input-json '{"date":"2026-02-14"}'
rvn workflow continue <run-id> --agent-output-json '{"outputs":{"markdown":"..."}}'
rvn workflow runs list --status awaiting_agent
rvn workflow runs step <run-id> <step-id>
rvn workflow runs prune --status completed --older-than 14d --confirm
```

