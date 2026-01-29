# Workflows Reference

Workflows are reusable **prompt templates** (with optional deterministic context prefetch) defined in `raven.yaml`.

They are designed for agent-driven automation:
- Deterministic context prefetch gathers structured context (`query`, `read`, `search`, `backlinks`)
- Raven renders a prompt (it does **not** call an LLM)
- Optionally, workflows can declare output contracts (`outputs:`) for agents
- Apply any desired changes via normal Raven commands (e.g., `add`, `set`, `query --apply`)

## Definition Location

Workflows live under `workflows:` in `raven.yaml` and are keyed by name.

Workflows can be defined inline or via `file:`.

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
        description: Meeting object ID
    context:
      meeting:
        read: "{{inputs.meeting_id}}"
      mentions:
        backlinks: "{{inputs.meeting_id}}"
    # Optional output contract for agents (omit for “just render a prompt” workflows)
    outputs:
      markdown:
        type: markdown
        required: true
    prompt: |
      Prepare me for this meeting.

      ## Meeting
      {{context.meeting.content}}

      ## Mentions
      {{context.mentions}}
```

### File-Backed Definition

```yaml
workflows:
  meeting-prep:
    file: workflows/meeting-prep.yaml
```

External workflow file (same fields, without the workflow name key):

```yaml
# workflows/meeting-prep.yaml
description: Prepare a brief for a meeting
inputs:
  meeting_id:
    type: ref
    target: meeting
    required: true
context:
  meeting:
    read: "{{inputs.meeting_id}}"
outputs:
  markdown:
    type: markdown
    required: true
prompt: |
  Prepare me for this meeting.
  {{context.meeting.content}}
```

## Inputs

Inputs are values provided when running the workflow.

Input properties:
- `type`: `string`, `ref`, `date`, `boolean` (inputs are passed as strings; type is for validation/presentation)
- `required`: boolean
- `default`: string
- `description`: string
- `target`: string (for `ref` type)

## Context Prefetch

`context:` is an optional map of prefetch items.

Supported forms:

- `query`: Raven query language string
- `read`: reference/object id (resolved canonically)
- `search`: term string (+ optional `limit`, default 20)
- `backlinks`: target reference/object id

The prompt can reference prefetched values via `{{context.<name>}}` and `{{context.<name>.<path>}}`.

## Prompt Rendering

Workflows only render a prompt; they do not call an LLM.

If `outputs:` is provided, Raven prepends a strict JSON “return shape” contract to the rendered prompt.

```yaml
outputs:
  markdown:
    type: markdown
    required: true
```

#### Prompt output envelope

When `outputs:` is present, prompt output must be a JSON envelope:

```json
{ "outputs": { "markdown": "..." } }
```

Output types:
- `markdown`: JSON string

## Legacy `steps:` workflows

For advanced use cases, workflows may be defined as explicit ordered `steps:` pipelines (with step interpolation via `{{steps.<id>...}}`).

## Protected paths

Workflows and plan application refuse to operate on protected/system-managed paths.

Hardcoded:
- `.raven/`, `.trash/`, `.git/`
- `raven.yaml`, `schema.yaml`

User-configurable (additive) via `raven.yaml`:
- `protected_prefixes: ["templates/", "private/"]`

## CLI Commands

```bash
# List workflows
rvn workflow list

# Show workflow definition
rvn workflow show <name>

# Run deterministic steps until first prompt step (returns prompt + output schema)
rvn workflow run <name> --input key=value --input key2=value2
```

