# Raven Workflows

Use this skill for workflow definition lifecycle, step mutation, and run operations.

## Operating rules

- Prefer `rvn workflow scaffold` or `rvn workflow add` over manual `raven.yaml` edits.
- Prefer Raven MCP workflow tools when operating inside MCP; otherwise use `rvn ... --json`.
- Validate after any workflow-file or step mutation.
- Treat run checkpoints as durable state that can be resumed with `rvn workflow continue`.

## Typical flow

1. Create or register (`workflow scaffold` or `workflow add`).
2. Validate (`workflow validate`) and inspect (`workflow show`).
3. Iterate on steps (`workflow step add|update|remove`), then validate again.
4. Execute with `workflow run`; if paused on an agent step, resume with `workflow continue`.
5. Inspect retained runs (`workflow runs list`, `workflow runs step`) and prune when needed.

## Safety

- Step edits return previews and re-validate before applying.
- `workflow runs prune` is preview-only until `--confirm` is provided.

## Reference

- Step-type patterns, run lifecycle, and maintenance flows: `references/workflow-authoring.md`
