# Workflows

Workflows are reusable prompt templates (with optional deterministic `context:` prefetch) defined in `raven.yaml`.

They’re designed for agent usage:
1. Raven runs deterministic context prefetch (optional)
2. Raven renders a prompt (it does not call an LLM)
3. If the workflow declares `outputs:`, the agent responds with a JSON envelope `{ "outputs": { ... } }`
4. Apply any desired changes via normal Raven commands (e.g., `add`, `set`, `query --apply`)

## Run a workflow

```bash
rvn workflow list
rvn workflow show meeting-prep
rvn workflow run meeting-prep --input meeting_id=meetings/alice-1on1
```

Reference: `reference/workflows.md`.

