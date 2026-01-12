# Workflows

Workflows are reusable prompt templates stored in your vault (`raven.yaml`). Raven renders them by:
1. validating inputs
2. gathering context (`read`, `query`, `backlinks`, `search`)
3. substituting variables into a prompt template

## Define a workflow

Inline in `raven.yaml`:

```yaml
workflows:
  meeting-prep:
    description: Prep for a meeting
    inputs:
      meeting_id:
        type: ref
        target: meeting
        required: true
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

Or reference a file:

```yaml
workflows:
  meeting-prep:
    file: workflows/meeting-prep.yaml
```

## Run workflows

```bash
rvn workflow list
rvn workflow show meeting-prep
rvn workflow render meeting-prep --input meeting_id=meetings/alice-1on1
```

Reference: `reference/workflows.md`.

## Next steps

- See `reference/workflows.md` for complete workflow configuration
- See `reference/query-language.md` for context query syntax
- See `reference/mcp.md` for using workflows with MCP agents

