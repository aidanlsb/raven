# Workflow Lifecycle

Use this guide for running, resuming, inspecting, and cleaning up workflow runs.

## 1. Discover and inspect workflows

```text
raven_workflow_list()
raven_workflow_show(name="meeting-prep")
```

Use `show` before running to verify required inputs.

## 2. Start a run

```text
raven_workflow_run(name="meeting-prep", input={"meeting_id":"meetings/team-sync"})
```

Behavior:
- Deterministic steps execute until completion or agent step.
- If an agent step is reached, response includes a rendered prompt, declared outputs, and `step_summaries`.

## 3. Inspect step outputs (incremental retrieval)

```text
raven_workflow_runs_step(run-id="wrf_...", step-id="fetch-context")
raven_workflow_runs_step(run-id="wrf_...", step-id="fetch-context", path="data.results", offset=0, limit=50)
```

Use this to avoid pulling large step payloads all at once.

## 4. Continue an awaiting-agent run

Provide agent output with top-level `outputs`:

```text
raven_workflow_continue(
  run-id="wrf_...",
  agent-output-json={"outputs":{"markdown":"..."}}
)
```

Optional optimistic concurrency:

```text
raven_workflow_continue(
  run-id="wrf_...",
  expected-revision=3,
  agent-output-json={"outputs":{"markdown":"..."}}
)
```

If revisions diverge, re-read run state and retry.

## 5. List and filter historical runs

```text
raven_workflow_runs_list()
raven_workflow_runs_list(status="awaiting_agent")
raven_workflow_runs_list(workflow="meeting-prep", status="completed,failed")
```

Use this to resume paused runs instead of starting duplicates.

## 6. Prune old checkpoints

```text
# Preview
raven_workflow_runs_prune(status="completed", older-than="14d")

# Apply
raven_workflow_runs_prune(status="completed", older-than="14d", confirm=true)
```

Always preview first before deleting run history.

## Operational rules

- Prefer continuing an existing `awaiting_agent` run over rerunning from scratch.
- Validate inputs early (`raven_workflow_show`) to avoid repeated failures.
- Surface workflow error codes exactly and ask for user intent when recovery is ambiguous.

## Related topics

- `raven://guide/key-workflows` - practical end-to-end workflow usage
- `raven://guide/response-contract` - error/warning envelope handling
- `raven://guide/error-handling` - recovery patterns
