# Raven Workflows

Use this skill for workflow definition lifecycle, step mutation, and run operations.

## Operating rules

- Prefer `rvn workflow scaffold` or `rvn workflow add` over manual `raven.yaml` edits.
- Prefer Raven MCP workflow tools when operating inside MCP; otherwise use `rvn ... --json`.
- Validate after any workflow-file or step mutation.
- Inspect a workflow before running it so you know the expected inputs and step shape.
- Treat run checkpoints as durable state that can be resumed with `rvn workflow continue --expected-revision <revision>`.
- Do not guess an awaiting-agent output shape; continue runs with the declared `{"outputs": ...}` contract.

## Authoring loop

1. Create or register (`workflow scaffold` or `workflow add`).
2. Validate (`workflow validate`) and inspect (`workflow show`).
3. Iterate on steps (`workflow step add|batch|update|remove`), then validate again.
## Run operations

1. Execute with `workflow run`.
2. If the run pauses on an agent step, inspect step summaries and fetch large step outputs with `workflow runs step`.
3. Resume with `workflow continue` using the required `{"outputs": ...}` payload and the returned `--expected-revision`.
4. Inspect retained runs with `workflow runs list`, then prune with `workflow runs prune` when needed.

## Safety

- Step edits validate before Raven writes the workflow file.
- `workflow continue` must match the declared output contract for the awaiting step.
- `workflow runs prune` is preview-only until `--confirm` is provided.

## Reference

- Step-type patterns, run lifecycle, and maintenance flows: `references/workflow-authoring.md`
