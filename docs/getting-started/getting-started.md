# Getting Started

This guide is for your first session only.

Goal: complete one full Raven loop with confidence:
1. create a vault
2. add structured information
3. query and verify the result

Out of scope here:
- deep configuration (`configuration.md`)
- deep schema modeling (`types-and-traits/schema-intro.md` and `types-and-traits/schema.md`)
- advanced command workflows (use `rvn help <command>`)

## 1) Install and verify

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
rvn version
```

Success check: `rvn version` prints version/build metadata.

## 2) Create a vault

```bash
rvn init ~/notes
cd ~/notes
```

Success check: you have:
- `schema.yaml`
- `raven.yaml`
- `.raven/`

## 3) Complete your first loop

### Step A: Create a typed object

```bash
rvn new project "Onboarding"
```

Success check: a file exists at `projects/onboarding.md`.

### Step B: Add a structured note

```bash
rvn add "Planning [[projects/onboarding]] @highlight"
```

This appends to today's daily note and includes:
- a reference (`[[projects/onboarding]]`)
- a trait (`@highlight`)

### Step C: Query it

```bash
rvn query 'trait:highlight refs([[projects/onboarding]])'
```

Success check: at least one result appears from today's note.

If no results appear:
- run `rvn reindex`
- run the same query again

## 4) What to do next

Follow this sequence:
1. `getting-started/configuration.md` - set up `config.toml` and `raven.yaml`
2. `types-and-traits/templates.md` - set up reusable note structure
3. `types-and-traits/schema-intro.md` - learn practical `schema.yaml` basics

## Agent next step (after activation)

If you are using Cursor or Claude, connect Raven through MCP now:
- see `agents/mcp.md` for setup

Suggested first prompt once connected:

```text
"Summarize my onboarding project and list actionable notes that reference it."
```

