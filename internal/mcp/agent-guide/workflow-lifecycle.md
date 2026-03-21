# Workflow Lifecycle

Use this guide when running, continuing, inspecting, or pruning workflow runs.

## 1. Discover and inspect workflows

```text
raven_invoke(command="workflow_list")
raven_invoke(command="workflow_show", args={"name":"meeting-prep"})
```

Use `workflow_show` before running to verify required inputs.

## 2. Run a workflow

```text
raven_invoke(command="workflow_run", args={
  "name":"meeting-prep",
  "input":{"meeting_id":"meetings/team-sync"}
})
```

## 3. Inspect step output

```text
raven_invoke(command="workflow_runs_step", args={"run_id":"wrf_...", "step_id":"fetch-context"})
raven_invoke(command="workflow_runs_step", args={
  "run_id":"wrf_...",
  "step_id":"fetch-context",
  "path":"data.results",
  "offset":0,
  "limit":50
})
```

## 4. Continue an awaiting-agent run

```text
raven_invoke(command="workflow_continue", args={
  "run_id":"wrf_...",
  "output":"Final answer text"
})
```

If the step expects structured output, follow the schema from `workflow_show` or the run payload.

## 5. List runs

```text
raven_invoke(command="workflow_runs_list")
raven_invoke(command="workflow_runs_list", args={"status":"awaiting_agent"})
raven_invoke(command="workflow_runs_list", args={"workflow":"meeting-prep", "status":"completed,failed"})
```

## 6. Prune old runs

```text
raven_invoke(command="workflow_runs_prune", args={"status":"completed", "older-than":"14d"})
raven_invoke(command="workflow_runs_prune", args={"status":"completed", "older-than":"14d", "confirm":true})
```

## Practical rules

- Inspect a workflow before running it.
- Validate inputs early to avoid repeated failures.
- Use step pagination on large payloads.
- Treat workflow continuation as a normal registry command invoked through `raven_invoke`.
