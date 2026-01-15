# Workflows Reference

Workflows are reusable prompt templates defined in `raven.yaml`. They help agents perform complex, context-aware tasks by:
- Validating inputs (required fields, defaults, types)
- Gathering context (reading files, running queries, finding backlinks, searching)
- Rendering a prompt with substituted values

## Quick Example

```yaml
workflows:
  meeting-prep:
    description: Prepare a brief for a meeting
    inputs:
      meeting_id:
        type: ref
        target: meeting
        required: true
        description: Meeting object ID
    context:
      meeting:
        read: "{{inputs.meeting_id}}"
      mentions:
        backlinks: "{{inputs.meeting_id}}"
    prompt: |
      Prepare me for this meeting.

      ## Meeting
      {{context.meeting}}

      ## Mentions
      {{context.mentions}}
```

---

## Definition Location

Workflows live under `workflows:` in `raven.yaml` and are keyed by name.

### Inline Definition

```yaml
workflows:
  research:
    description: Research a topic in my notes
    inputs:
      question:
        type: string
        required: true
    context:
      results:
        search: "{{inputs.question}}"
    prompt: |
      Answer this question based on my notes:
      {{inputs.question}}

      ## Relevant notes
      {{context.results}}
```

### File-Backed Definition

```yaml
workflows:
  meeting-prep:
    file: workflows/meeting-prep.yaml
```

External workflow file (same fields, without the workflow name key):

```yaml
# workflows/meeting-prep.yaml
description: Prepare a brief for a meeting
inputs:
  meeting_id:
    type: ref
    target: meeting
    required: true
context:
  meeting:
    read: "{{inputs.meeting_id}}"
prompt: |
  Prepare me for this meeting.
  {{context.meeting}}
```

### Rules

- A workflow must have either `file:` OR an inline `prompt:` — not both
- If both are present, it's an error

---

## Inputs

Inputs are values provided when running the workflow.

### Input Properties

| Property | Type | Description |
|----------|------|-------------|
| `type` | string | Input type: `string`, `ref`, `date`, `boolean` |
| `required` | boolean | Whether the input must be provided |
| `default` | string | Default value if not provided |
| `description` | string | Human-readable description |
| `target` | string | Target type (only for `ref` type) |

### Input Types

| Type | Description | Example Value |
|------|-------------|---------------|
| `string` | Free-form text | `"How does auth work?"` |
| `ref` | Object reference | `"meetings/team-sync"` |
| `date` | Date value | `"2026-01-15"` |
| `boolean` | True/false | `"true"` |

**Note:** All inputs are handled as strings at render time. Type information is mainly for validation and presentation.

### Examples

```yaml
inputs:
  # Simple required string
  question:
    type: string
    required: true
    description: Question to answer

  # Reference with target type
  meeting_id:
    type: ref
    target: meeting
    required: true
    description: Meeting to prepare for

  # Optional with default
  depth:
    type: string
    default: "medium"
    description: How deep to research (quick, medium, thorough)

  # Date input
  as_of:
    type: date
    default: "today"
    description: Date to check status as of
```

---

## Context Queries

Context queries gather information from the vault before rendering the prompt.

### Query Types

Each context entry must be exactly one of:

| Type | Description | Syntax |
|------|-------------|--------|
| `read` | Read an object's content | `read: "<object-id>"` |
| `query` | Run a Raven query | `query: "<rql>"` |
| `backlinks` | Find objects referencing a target | `backlinks: "<object-id>"` |
| `search` | Full-text search | `search: "<term>"` (optional `limit`) |

### Input Substitution

`{{inputs.*}}` values are substituted **before** the query executes:

```yaml
context:
  # Input substituted before read
  person:
    read: "{{inputs.person_id}}"

  # Input substituted in query
  tasks:
    query: "trait:todo within:[[{{inputs.project_id}}]]"

  # Input substituted in search
  related:
    search: "{{inputs.topic}}"
    limit: 10
```

### Examples

```yaml
context:
  # Read a specific object
  meeting:
    read: "{{inputs.meeting_id}}"

  # Run a query with field filter
  active_projects:
    query: "object:project .status==active"

  # Find what references an object
  mentions:
    backlinks: "{{inputs.person_id}}"

  # Search with limit
  background:
    search: "{{inputs.topic}}"
    limit: 5

  # Complex query with input substitution
  related_tasks:
    query: "trait:todo refs:[[{{inputs.project_id}}]]"
```

### Error Handling

If a context query fails, the workflow still renders. The context slot is filled with an error object:

```json
{
  "error": "Object not found: meetings/nonexistent"
}
```

This allows workflows to gracefully handle missing data.

---

## Template Substitution

The prompt template supports variable substitution for both inputs and context.

### Input Variables

`{{inputs.name}}` is replaced with the input value:

```yaml
prompt: |
  Research this question: {{inputs.question}}
  
  Focus on: {{inputs.topic}}
```

### Context Variables

`{{context.key}}` is replaced with the formatted context result:

```yaml
prompt: |
  ## Meeting Details
  {{context.meeting}}
  
  ## Related Notes
  {{context.mentions}}
```

### Context Subfields

Access specific parts of context results:

| Pattern | Description |
|---------|-------------|
| `{{context.X}}` | Auto-formatted result (readable text) |
| `{{context.X.content}}` | Document content (for `read:` results) |
| `{{context.X.id}}` | Object ID |
| `{{context.X.type}}` | Object type |
| `{{context.X.fields.name}}` | Specific frontmatter field |

### Escaping

Use `\{{` and `\}}` to produce literal `{{` / `}}` in output:

```yaml
prompt: |
  Use this template syntax: \{{variable}}
```

### Unknown Variables

Unknown inputs or context paths are left as-is (not substituted). This makes it easy to spot typos in templates.

---

## Output Format

When you render a workflow, you get back:

### CLI Output (`--json`)

```json
{
  "ok": true,
  "workflow": "meeting-prep",
  "inputs": {
    "meeting_id": "meetings/team-sync"
  },
  "context": {
    "meeting": {
      "id": "meetings/team-sync",
      "type": "meeting",
      "content": "# Team Sync\n\n...",
      "fields": {
        "time": "2026-01-15T10:00",
        "attendees": ["people/freya", "people/thor"]
      }
    },
    "mentions": [
      {"id": "daily/2026-01-14", "type": "date", "snippet": "..."},
      {"id": "projects/website", "type": "project", "snippet": "..."}
    ]
  },
  "prompt": "Prepare me for this meeting.\n\n## Meeting\n# Team Sync\n..."
}
```

### MCP Output

Same structure, returned as the tool result. Agents use the `prompt` field to guide their response.

---

## CLI Commands

```bash
# List all workflows
rvn workflow list

# Show workflow details (inputs, context queries, prompt)
rvn workflow show <name>

# Render a workflow with inputs
rvn workflow render <name> --input key=value [--input key2=value2 ...]
```

### Examples

```bash
# List available workflows
rvn workflow list

# See what a workflow needs
rvn workflow show meeting-prep

# Render with inputs
rvn workflow render meeting-prep --input meeting_id=meetings/team-sync

# Multiple inputs
rvn workflow render research --input question="How does auth work?" --input depth=thorough
```

---

## MCP Tools

Workflows are available via MCP:

| Tool | Description |
|------|-------------|
| `raven_workflow_list` | List available workflows |
| `raven_workflow_show` | Show workflow details |
| `raven_workflow_render` | Render with inputs |

### MCP Usage

```python
# List workflows
raven_workflow_list()

# Show details
raven_workflow_show(name="meeting-prep")

# Render with inputs (pass as object)
raven_workflow_render(name="meeting-prep", input={"meeting_id": "meetings/team-sync"})
```

---

## Complete Examples

### Meeting Preparation

```yaml
workflows:
  meeting-prep:
    description: Prepare a brief for an upcoming meeting
    inputs:
      meeting_id:
        type: ref
        target: meeting
        required: true
        description: Meeting to prepare for
    context:
      meeting:
        read: "{{inputs.meeting_id}}"
      attendees_notes:
        query: "object:person"
      recent_mentions:
        backlinks: "{{inputs.meeting_id}}"
      related_projects:
        query: "object:project refs:[[{{inputs.meeting_id}}]]"
    prompt: |
      Prepare me for this meeting.

      ## Meeting Details
      {{context.meeting}}

      ## Recent Mentions
      These notes recently mentioned this meeting:
      {{context.recent_mentions}}

      ## Related Projects
      {{context.related_projects}}

      Please summarize:
      1. Key topics to discuss
      2. Any action items from previous meetings
      3. Questions to raise
```

### Research Assistant

```yaml
workflows:
  research:
    description: Research a topic across my notes
    inputs:
      question:
        type: string
        required: true
        description: Question to research
      scope:
        type: string
        default: "all"
        description: Scope (all, projects, meetings, people)
    context:
      search_results:
        search: "{{inputs.question}}"
        limit: 20
      related_projects:
        query: "object:project content:\"{{inputs.question}}\""
    prompt: |
      Answer this question based on my notes:
      
      **Question:** {{inputs.question}}
      
      ## Search Results
      {{context.search_results}}
      
      ## Related Projects
      {{context.related_projects}}
      
      Please provide:
      1. A direct answer if possible
      2. Key insights from the notes
      3. Gaps in my knowledge (what I haven't captured)
```

### Weekly Review

```yaml
workflows:
  weekly-review:
    description: Generate a weekly review summary
    inputs:
      week_start:
        type: date
        required: true
        description: Start of the week (Monday)
    context:
      completed:
        query: "trait:todo value==done"
      overdue:
        query: "trait:due value==past"
      upcoming:
        query: "trait:due value==this-week"
    prompt: |
      Generate my weekly review for the week of {{inputs.week_start}}.

      ## Completed Tasks
      {{context.completed}}

      ## Overdue Items
      {{context.overdue}}

      ## Upcoming This Week
      {{context.upcoming}}

      Please summarize:
      1. Key accomplishments
      2. Items needing attention
      3. Priorities for next week
```

### Person Brief

```yaml
workflows:
  person-brief:
    description: Get a quick brief on a person
    inputs:
      person_id:
        type: ref
        target: person
        required: true
    context:
      person:
        read: "{{inputs.person_id}}"
      meetings:
        query: "object:meeting refs:[[{{inputs.person_id}}]]"
      mentions:
        backlinks: "{{inputs.person_id}}"
      tasks:
        query: "trait:todo refs:[[{{inputs.person_id}}]]"
    prompt: |
      Give me a brief on {{inputs.person_id}}.

      ## Profile
      {{context.person}}

      ## Meetings Together
      {{context.meetings}}

      ## Tasks Involving Them
      {{context.tasks}}

      ## Other Mentions
      {{context.mentions}}
```

---

## Best Practices

1. **Use descriptive input names** — `meeting_id` is clearer than `id`

2. **Provide descriptions** — Help users (and agents) understand what each input is for

3. **Set sensible defaults** — Make workflows easier to run for common cases

4. **Structure prompts clearly** — Use headers and sections to organize context

5. **Handle missing context gracefully** — Workflows continue even if a query fails

6. **Keep prompts focused** — One workflow should do one thing well

7. **Use file-backed workflows for complex prompts** — Keeps `raven.yaml` clean
