# Onboarding

Use this guide when teaching a new user how Raven works.

## Teaching sequence

1. Show the schema and traits.
2. Show vault stats and saved queries.
3. Create one object.
4. Add body content.
5. Run one query.
6. Run one check.

## Discovery sequence

```text
raven_invoke(command="schema", args={"subcommand":"types"})
raven_invoke(command="schema", args={"subcommand":"traits"})
raven_invoke(command="vault_stats")
raven_invoke(command="query", args={"list":true})
raven_invoke(command="workflow_list")
```

## First create flow

```text
result = raven_invoke(command="new", args={"type":"project", "title":"Website Redesign"})
raven_invoke(command="add", args={"text":"## Notes\n- Kickoff next week", "to":result.data.file})
raven_invoke(command="set", args={"object_id":result.data.id, "fields":{"status":"active"}})
```

## First query flow

```text
raven_invoke(command="query", args={"query_string":"object:project .status==active"})
raven_invoke(command="query", args={"query_string":"trait:due in(.value, [today,tomorrow])"})
```

## Saved query example

```text
raven_invoke(command="query_add", args={
  "name":"reading-list",
  "query_string":"object:book .status==reading"
})
```

## Import example

```text
raven_invoke(command="import", args={"type":"project", "file":"projects.json", "dry_run":true})
raven_invoke(command="import", args={"type":"project", "file":"projects.json", "confirm":true})
```

## Practical rules

- Teach the compact surface, not legacy direct MCP tools.
- Use canonical command IDs in examples and prompts.
- Keep the first session concrete: one create, one query, one check.
