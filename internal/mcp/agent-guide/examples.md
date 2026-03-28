# Examples

Use these examples as translations from user intent to compact-surface MCP usage.

## Find items due today or tomorrow

```text
raven_invoke(command="query", args={"query_string":"trait:due in(.value, [today,tomorrow])"})
```

## Validate project objects

```text
raven_invoke(command="check", args={"type":"project"})
```

## Create a project

```text
raven_invoke(command="schema", args={"subcommand":"type", "name":"project"})
raven_invoke(command="new", args={"type":"project", "title":"Website Redesign"})
```

## Import contacts

```text
raven_invoke(command="import", args={"type":"person", "file":"contacts.json", "dry_run":true})
raven_invoke(command="import", args={"type":"person", "file":"contacts.json", "confirm":true})
```

## Build a reusable meeting template

```text
raven_invoke(command="schema", args={"subcommand":"type", "name":"meeting"})
raven_invoke(command="template_write", args={"path":"meeting.md", "content":"# {{title}}\n\n## Attendees\n\n## Agenda\n\n## Notes"})
raven_invoke(command="schema_template_set", args={"template_id":"meeting_standard", "file":"templates/meeting.md"})
raven_invoke(command="schema_template_bind", args={"template_id":"meeting_standard", "type":"meeting", "default":true})
```

## Run a workflow

```text
raven_invoke(command="workflow_list")
raven_invoke(command="workflow_run", args={"name":"meeting-prep", "input":{"person_id":"people/freya"}})
```

## Continue an awaiting-agent run

```text
raven_invoke(command="workflow_runs_list", args={"status":"awaiting_agent"})
raven_invoke(command="workflow_continue", args={"run_id":"wrf_...", "expected_revision":1, "outputs":{"markdown":"Final answer text"}})
```

## Bulk preview then apply

```text
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo", "count-only":true})
raven_invoke(command="query", args={"query_string":"trait:todo .value==todo", "limit":50, "offset":0})
```
