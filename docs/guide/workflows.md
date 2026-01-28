# Workflows

Workflows are ordered pipelines (`steps:`) defined in `raven.yaml`.

They’re designed for agent usage:
1. Raven runs deterministic steps to gather structured context
2. Raven renders a `prompt` step and tells the agent what outputs are required
3. The agent responds with a JSON envelope `{ "outputs": { ... } }`
4. If the agent produced a `plan`, you can preview/apply it back into the vault

## Run a workflow

```bash
rvn workflow list
rvn workflow show meeting-prep
rvn workflow run meeting-prep --input meeting_id=meetings/alice-1on1
```

## Apply a plan

```bash
# Preview
rvn workflow apply-plan daily-todo-triage --plan plan.json

# Apply
rvn workflow apply-plan daily-todo-triage --plan plan.json --confirm
```

Reference: `reference/workflows.md`.

