# Raven Workflows

> ✅ **Status: Implemented** — Phase 1 of this specification is now available. See the [README](../README.md#workflows) for usage examples.

Workflows are reusable prompt templates that enable LLM agents to perform complex, multi-step operations against a Raven vault. They combine pre-gathered context with structured instructions, allowing any agent to invoke sophisticated knowledge management tasks.

## Design Philosophy

Raven workflows follow a clear separation of concerns:

- **Raven's job**: Store workflow definitions, gather context, render prompts
- **Agent's job**: Execute the prompt, reason about the task, call Raven tools

This means Raven doesn't need to know about models, API keys, or LLM providers. It simply provides a well-structured prompt with pre-gathered context to whoever asks for it.

## Workflow Configuration

Workflows are defined in `raven.yaml` under the `workflows` key. Simple workflows can be defined inline, while complex workflows can reference external files within the vault.

```yaml
# raven.yaml

daily_directory: daily
# ... other config ...

workflows:
  # Inline definition for simple workflows
  weekly-review:
    description: Summarize the week and plan ahead
    inputs: {}
    context:
      overdue:
        query: "trait:due value:past"
    prompt: |
      Generate my weekly review based on the context provided.

  # Reference to external file for complex workflows
  meeting-prep:
    file: workflows/meeting-prep.yaml
  
  # Another external file
  project-status:
    file: workflows/project-status.yaml
```

External workflow files contain the full definition without the workflow name (since the name comes from the key in `raven.yaml`):

```yaml
# workflows/meeting-prep.yaml
description: Prepare a brief for an upcoming meeting

inputs:
  meeting_id:
    type: ref
    target: meeting
    required: true

context:
  meeting:
    read: "{{inputs.meeting_id}}"
  
  backlinks:
    backlinks: "{{inputs.meeting_id}}"

prompt: |
  Prepare me for this meeting:
  
  **Meeting:** {{context.meeting.title}}
  
  Use the backlinks and your available Raven tools to gather more context
  about attendees and related projects.
```

The `file:` path is relative to vault root. Users can organize workflow files however they like—`workflows/`, `_config/workflows/`, or alongside related content.

If a workflow entry has both `file:` and inline keys (like `prompt:`), that's a validation error.

## Workflow Definition

Each workflow defines:

- **Description**: A brief summary of what the workflow does
- **Inputs**: Parameters the workflow accepts (optional)
- **Context**: Data to gather before rendering the prompt (optional)
- **Prompt**: The task description with interpolated context

### Basic Structure

```yaml
# Inline in raven.yaml under workflows.<name>, or in an external file

description: A brief description of what this workflow does

inputs:
  some_id:
    type: string
    required: true
    description: An object ID to operate on
  
  optional_param:
    type: string
    required: false
    default: "default value"

context:
  # Data gathered before prompt rendering
  my_object:
    read: "{{inputs.some_id}}"
  
  related_items:
    query: "object:project refs:[[{{inputs.some_id}}]]"

prompt: |
  Here is the task:
  
  **Object:** {{context.my_object.title}}
  
  Your instructions:
  1. Do something with the object
  2. Use raven_query to find additional context
  3. Use raven_new or raven_add to create content
  4. Report what you did
```

## Input Types

Inputs define what parameters a workflow accepts:

```yaml
inputs:
  # Simple string
  question:
    type: string
    required: true
    description: The research question to answer
  
  # Reference to a typed object
  project_id:
    type: ref
    target: project
    required: true
  
  # Optional with default
  format:
    type: string
    required: false
    default: "summary"
  
  # Date
  target_date:
    type: date
    required: false
```

Supported types: `string`, `ref`, `date`, `boolean`

When `type: ref` is used with a `target`, the input is validated to ensure it references an object of that type.

## Context Queries

The `context` block gathers data before the prompt is rendered. This reduces round-trips when the agent executes the workflow.

**Important:** All context queries support `{{inputs.X}}` substitution. Inputs are substituted *before* the query executes, enabling dynamic context gathering based on user-provided inputs.

Each context entry uses one of these query types:

### Read a single object

```yaml
context:
  meeting:
    read: "{{inputs.meeting_id}}"
```

This is equivalent to `rvn read <id>`. Returns the full object with frontmatter and content.

### Run a query

```yaml
context:
  active_projects:
    query: "object:project .status:active"
  
  overdue_tasks:
    query: "trait:due value:past"
  
  # Dynamic query using inputs:
  project_tasks:
    query: "object:task .project:{{inputs.project_id}}"
```

This uses Raven's query language exactly as documented. Returns an array of matching results. Input variables like `{{inputs.project_id}}` are substituted before the query executes.

Note: The query language requires a type constraint (`object:<type>` or `trait:<name>`). For unconstrained full-text search, use `search:` instead.

### Get backlinks

```yaml
context:
  references:
    backlinks: "{{inputs.person_id}}"
```

Returns all objects that reference the given target.

### Full-text search

```yaml
context:
  relevant:
    search: "{{inputs.question}}"
    limit: 10
```

Performs unconstrained full-text search across all content in the vault. The `limit` is optional (defaults to 20).

This differs from using `content:` within a query - `search:` has no type constraint, while `query: "object:project content:\"text\""` only searches within projects.

## Template Syntax

Prompts use simple `{{variable}}` substitution for interpolation.

### Input access

```
{{inputs.project_id}}
{{inputs.question}}
```

### Context access

```
{{context.meeting.title}}
{{context.meeting.type}}
{{context.active_projects}}
```

Context results are included as JSON in the rendered output, so agents can parse structured data.

### Escaping

Use `\{{literal}}` if you need literal `{{` in output.

### No complex templating

Workflows intentionally use simple variable substitution. Complex logic (conditionals, iteration, filtering) should be handled by the agent executing the workflow, not by the template engine.

## CLI Commands

### List available workflows

```bash
rvn workflow list
```

Output:
```
meeting-prep     Prepare a brief for an upcoming meeting
weekly-review    Summarize the week and plan ahead
project-status   Generate a project status report
```

JSON output:
```bash
rvn workflow list --json
```

```json
{
  "ok": true,
  "data": {
    "workflows": [
      {
        "name": "meeting-prep",
        "description": "Prepare a brief for an upcoming meeting",
        "inputs": {
          "meeting_id": {
            "type": "ref",
            "target": "meeting",
            "required": true
          }
        }
      },
      {
        "name": "weekly-review",
        "description": "Summarize the week and plan ahead",
        "inputs": {}
      }
    ]
  }
}
```

### Show workflow details

```bash
rvn workflow show meeting-prep
```

Output:
```yaml
name: meeting-prep
description: Prepare a brief for an upcoming meeting

inputs:
  meeting_id:
    type: ref
    target: meeting
    required: true

context:
  meeting: read
  backlinks: backlinks
```

### Render a workflow prompt

```bash
rvn workflow render meeting-prep --input meeting_id=meetings/alice-1on1
```

This command:
1. Loads the workflow definition
2. Validates inputs
3. Runs all context queries
4. Renders the template
5. Outputs the complete prompt with context

Output:
```
=== PROMPT ===
Prepare me for this meeting:

**Meeting:** 1:1 with Alice

Use the backlinks and your available Raven tools to gather more context
about attendees and related projects.

=== CONTEXT ===
{
  "meeting": {
    "id": "meetings/alice-1on1",
    "type": "meeting",
    "title": "1:1 with Alice",
    "fields": {
      "time": "2026-01-07T14:00",
      "attendees": ["[[people/alice]]"]
    },
    "content": "..."
  },
  "backlinks": [
    {
      "id": "daily/2026-01-02",
      "type": "date",
      "title": "Thursday, January 2, 2026"
    }
  ]
}
```

JSON output includes both prompt and context as structured data:

```bash
rvn workflow render meeting-prep --input meeting_id=meetings/alice-1on1 --json
```

```json
{
  "ok": true,
  "data": {
    "name": "meeting-prep",
    "prompt": "Prepare me for this meeting:\n\n**Meeting:** 1:1 with Alice\n\n...",
    "context": {
      "meeting": {
        "id": "meetings/alice-1on1",
        "type": "meeting",
        "title": "1:1 with Alice",
        "fields": { ... }
      },
      "backlinks": [ ... ]
    }
  }
}
```

## MCP Integration

Workflows are exposed as MCP tools, making them directly invocable by any MCP-compatible agent.

### Tool: `raven_workflow_list`

Lists available workflows with their descriptions and required inputs.

**Parameters:** None

**Response:**
```json
{
  "ok": true,
  "data": {
    "workflows": [
      {
        "name": "meeting-prep",
        "description": "Prepare a brief for an upcoming meeting",
        "inputs": {
          "meeting_id": {
            "type": "ref",
            "target": "meeting",
            "required": true
          }
        }
      },
      {
        "name": "weekly-review",
        "description": "Summarize the week and plan ahead",
        "inputs": {}
      }
    ]
  }
}
```

### Tool: `raven_workflow`

Renders a workflow and returns the prompt with pre-gathered context.

**Parameters:**
```json
{
  "workflow": "meeting-prep",
  "inputs": {
    "meeting_id": "meetings/alice-1on1"
  }
}
```

**Response:**
```json
{
  "ok": true,
  "data": {
    "name": "meeting-prep",
    "prompt": "Prepare me for this meeting:\n\n**Meeting:** 1:1 with Alice\n\n...",
    "context": {
      "meeting": {
        "id": "meetings/alice-1on1",
        "type": "meeting",
        "title": "1:1 with Alice",
        "fields": {
          "time": "2026-01-07T14:00",
          "attendees": ["[[people/alice]]"]
        }
      },
      "backlinks": [
        {
          "id": "daily/2026-01-02",
          "type": "date",
          "title": "Thursday, January 2, 2026"
        }
      ]
    }
  }
}
```

The calling agent then:
1. Reads the `prompt` to understand the task
2. Uses `context` for pre-gathered data (avoiding redundant queries)
3. Executes the task using Raven tools (`raven_query`, `raven_new`, `raven_add`, etc.)

## Example Workflows

### Simple Inline Workflow (in raven.yaml)

```yaml
# raven.yaml
workflows:
  research:
    description: Research a topic using the knowledge base
    inputs:
      question:
        type: string
        required: true
        description: The research question to answer
    context:
      initial_results:
        search: "{{inputs.question}}"
        limit: 20
    prompt: |
      Research this question: **{{inputs.question}}**
      
      I've gathered some initial search results (see context).
      
      Your tasks:
      1. Review the initial results for relevance
      2. Use raven_query to find additional context
      3. Follow references and check backlinks with raven_backlinks
      4. Synthesize findings with [[citations]] to source documents
      5. Save your findings using raven_new or raven_add
```

### External Workflow File

```yaml
# raven.yaml
workflows:
  meeting-prep:
    file: workflows/meeting-prep.yaml
```

```yaml
# workflows/meeting-prep.yaml
description: Prepare a brief for an upcoming meeting

inputs:
  meeting_id:
    type: ref
    target: meeting
    required: true
    description: The meeting to prepare for

context:
  meeting:
    read: "{{inputs.meeting_id}}"
  
  meeting_refs:
    backlinks: "{{inputs.meeting_id}}"

prompt: |
  Prepare me for this meeting:
  
  **Meeting:** {{context.meeting.title}}
  **Type:** {{context.meeting.type}}
  
  I've gathered the meeting details and any documents that reference it
  (see context).
  
  Your tasks:
  1. Read the meeting object to understand the purpose and attendees
  2. For each attendee, use raven_backlinks to find recent interactions
  3. Check for open tasks or notes related to this meeting
  4. Identify potential discussion topics based on recent activity
  5. Create a prep document using raven_new with type "note" containing:
     - Meeting context and purpose
     - Per-attendee summary
     - Suggested agenda items
     - Open questions to address
```

### Weekly Review

```yaml
# workflows/weekly-review.yaml
description: Summarize the week and plan ahead

inputs: {}

context:
  overdue:
    query: "trait:due value:past"
  
  upcoming:
    query: "trait:due value:next-week"
  
  recent_highlights:
    query: "trait:highlight"

prompt: |
  Generate my weekly review.
  
  I've gathered overdue items, upcoming items, and recent highlights
  (see context).
  
  Your tasks:
  1. Summarize what was accomplished this week (check daily notes)
  2. Review overdue items and suggest what to reschedule or drop
  3. Preview upcoming items for next week
  4. Note any highlights worth celebrating
  5. Create a review document using raven_new with a summary and action items
```

### Project Status

```yaml
# workflows/project-status.yaml
description: Generate a status report for a project

inputs:
  project_id:
    type: ref
    target: project
    required: true

context:
  project:
    read: "{{inputs.project_id}}"
  
  project_refs:
    backlinks: "{{inputs.project_id}}"
  
  project_tasks:
    query: "trait:due refs:[[{{inputs.project_id}}]]"

prompt: |
  Generate a status report for this project.
  
  **Project:** {{context.project.title}}
  
  I've gathered the project details, all documents referencing it,
  and tasks linked to it (see context).
  
  Your tasks:
  1. Review the project object for current status
  2. Analyze linked tasks - what's done, what's pending, what's overdue
  3. Check recent notes and meetings related to the project
  4. Create a status report using raven_add to the project file, or
     raven_new to create a separate status document, containing:
     - One-paragraph executive summary
     - Key accomplishments
     - Current blockers or risks
     - Next milestones
     - Open questions
```

## File Organization

Workflows are registered in `raven.yaml`. External workflow files can live anywhere in the vault:

```
my-vault/
  raven.yaml              # workflows: section registers all workflows
  schema.yaml
  workflows/              # optional directory for external workflow files
    meeting-prep.yaml
    weekly-review.yaml
    project-status.yaml
  ...
```

Or organize them however makes sense for your vault:

```
my-vault/
  raven.yaml
  schema.yaml
  _config/
    workflows/
      meeting-prep.yaml
  projects/
    project-status.yaml   # workflow alongside related content
  ...
```

## Implementation Notes

### Error Handling

- Invalid workflow name → return error with available workflows
- Missing required input → return error listing missing inputs with their types
- Context query fails → include error in response, continue with other context
- Template render fails → return error with details
- Workflow file not found → return error indicating the missing file path

### Validation

On workflow load:
- Validate YAML syntax
- Check that context query types are valid (`read`, `query`, `backlinks`, `search`)
- Verify input types are supported (`string`, `ref`, `date`, `boolean`)
- For `ref` inputs with `target`, validate the target type exists in schema

### Context Query Execution

Context queries run sequentially. Each query type maps to existing Raven functionality:

| Context Type | Raven Command | Notes |
|-------------|---------------|-------|
| `read: <id>` | `rvn read <id> --json` | Returns single object |
| `query: "<query>"` | `rvn query "<query>" --json` | Returns array of results (type-constrained) |
| `backlinks: <id>` | `rvn backlinks <id> --json` | Returns array of referencing objects |
| `search: "<term>"` | `rvn search "<term>" --json` | Returns array of search results (unconstrained) |

### Variable Substitution

Template variables are substituted in two passes:
1. **Input substitution**: `{{inputs.X}}` replaced with input values (in context queries and prompt)
2. **Context substitution**: `{{context.X}}` replaced with gathered context (in prompt only)

Context values are included as-is for simple values, or as JSON for objects/arrays.
