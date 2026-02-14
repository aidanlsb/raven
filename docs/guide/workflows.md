# Workflows

Workflows are reusable, steps-based pipelines defined in `raven.yaml`.

They’re designed for agent usage:
1. Raven executes deterministic `tool` steps in order
2. Raven stops at the first `agent` step and returns its rendered prompt
3. If the `agent` step declares `outputs`, the agent responds with `{ "outputs": { ... } }`
4. Use additional `tool` steps (for example `raven_upsert`) for deterministic persistence

## Run a workflow

```bash
rvn workflow list
rvn workflow show meeting-prep
rvn workflow run meeting-prep --input meeting_id=meetings/alice-1on1
```

Reference: `reference/workflows.md`.

