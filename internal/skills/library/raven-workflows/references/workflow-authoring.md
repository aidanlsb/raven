# Workflow Authoring

## Bootstrap a workflow

```bash
rvn workflow scaffold daily-brief --json
rvn workflow validate daily-brief --json
rvn workflow show daily-brief --json
```

Or register an existing file:

```bash
rvn workflow add meeting-prep --file workflows/meeting-prep.yaml --json
rvn workflow validate meeting-prep --json
```

## Safe step mutation loop

```bash
rvn workflow step add daily-brief --step-json '{"id":"fetch","type":"tool","tool":"raven_query","arguments":{"query_string":"object:project .status==active"}}' --json
rvn workflow step batch daily-brief --mutations-json '{"operations":[{"action":"add","step":{"id":"enrich","type":"tool","tool":"raven_search","arguments":{"query":"active projects","limit":3}},"after":"fetch"},{"action":"update","step_id":"compose","patch":{"description":"Draft the final response"}}]}' --json
rvn workflow step update daily-brief fetch --step-json '{"description":"Load active projects"}' --json
rvn workflow step remove daily-brief fetch --json
rvn workflow validate daily-brief --json
```

## Run and continue

```bash
rvn workflow run meeting-prep --input meeting_id=meetings/team-sync --json
rvn workflow continue wrf_abcd1234 --expected-revision 1 --agent-output-json '{"outputs":{"markdown":"..."}}' --json
```

Use `workflow continue` only with the required `{"outputs": ...}` contract and the latest run revision.

## Run inspection and retention

```bash
rvn workflow runs list --status awaiting_agent --json
rvn workflow runs step wrf_abcd1234 fetch --json
rvn workflow runs step wrf_abcd1234 fetch --path data.results --offset 0 --limit 50 --json
rvn workflow runs prune --status completed --older-than 14d --confirm --json
```

## Step type guidance

- `tool`: deterministic command invocation.
- `agent`: pauses and returns rendered prompt plus declared output schema.
- `foreach`: deterministic fanout over an array with nested deterministic steps.
- `switch`: deterministic branch selection with deterministic branch steps.
