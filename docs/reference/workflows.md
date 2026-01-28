# Workflows Reference

Workflows are ordered `steps:` pipelines defined in `raven.yaml`.

They are designed for agent-driven automation:
- Deterministic steps gather structured context (`query`, `read`, `search`, `backlinks`)
- `prompt` steps render an agent prompt and declare required outputs
- Agents return a JSON envelope `{ "outputs": { ... } }`
- Plans (`outputs.plan`) can be previewed/applied safely via `rvn workflow apply-plan`

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
    steps:
      - id: meeting
        type: read
        ref: "{{inputs.meeting_id}}"

      - id: mentions
        type: backlinks
        target: "{{inputs.meeting_id}}"

      - id: prompt
        type: prompt
        outputs:
          markdown:
            type: markdown
            required: true
        template: |
          Prepare me for this meeting.

          ## Meeting
          {{steps.meeting.content}}

          ## Mentions
          {{steps.mentions.results}}
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
steps:
  - id: meeting
    type: read
    ref: "{{inputs.meeting_id}}"
  - id: prompt
    type: prompt
    outputs:
      markdown:
        type: markdown
        required: true
    template: |
      Prepare me for this meeting.
      {{steps.meeting.content}}
```

## Inputs

Inputs are values provided when running the workflow.

Input properties:
- `type`: `string`, `ref`, `date`, `boolean` (inputs are passed as strings; type is for validation/presentation)
- `required`: boolean
- `default`: string
- `description`: string
- `target`: string (for `ref` type)

## Step Types (v1)

### Deterministic steps

- `query`
  - `rql`: Raven query language string
  - output: `steps.<id>.results`
- `read`
  - `ref`: reference/object id (resolved canonically)
  - output: parsed document object (includes `content`, `fields`, etc.)
- `search`
  - `term`: search string
  - `limit`: optional (default 20)
  - output: `steps.<id>.results`
- `backlinks`
  - `target`: reference/object id
  - output: `steps.<id>.results`

### `prompt`

`prompt` steps only render a prompt; they do not call an LLM.

They declare required outputs via `outputs:`:

```yaml
- id: triage
  type: prompt
  outputs:
    markdown:
      type: markdown
      required: true
    plan:
      type: plan
      required: false
  template: |
    Return JSON: { "outputs": { ... } }
```

#### Prompt output envelope (required)

Prompt output must always be a JSON envelope:

```json
{ "outputs": { "markdown": "..." } }
```

Output types (v1):
- `markdown`: JSON string
- `plan`: JSON object `{ "plan_version": 1, "ops": [...] }` (strictly validated)

## Patch plans (v1)

`outputs.plan` is a deterministic set of Raven-native operations.

Top-level:

```json
{
  "plan_version": 1,
  "ops": [
    { "op": "add", "why": "Write summary", "args": { "to": "daily/2026-01-28", "heading": "## Plan", "text": "..." } }
  ]
}
```

Supported ops (v1):
- `add`: `{to, heading?, text}`
- `edit`: `{path, old_str, new_str}`
- `set`: `{object_id, fields}`
- `move`: `{source, destination, update_refs?}`
- `update_trait`: `{trait_id, value}`

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

# Apply an agent-produced plan (preview by default)
rvn workflow apply-plan <name> --plan plan.json
rvn workflow apply-plan <name> --plan plan.json --confirm
```

