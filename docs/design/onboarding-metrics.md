# Onboarding Metrics Plan

This note defines lightweight metrics for evaluating onboarding docs, especially the first-session funnel:

install -> init -> note -> structure -> query -> agent setup

## Goals

1. Reduce "installed, now what?" drop-off.
2. Increase first-session activation (note -> structure -> query success).
3. Improve progression from activation to agent setup.

## Core metrics

Track these metrics consistently before/after docs changes:

### 1) First-loop completion rate

Definition:
- percent of new users who complete the first loop:
  - create vault
  - add structured note
  - run a query with expected result

Why it matters:
- this is the activation definition for onboarding quality.

### 2) Time to first-loop completion

Definition:
- median time from first install step to successful query result.

Why it matters:
- measures onboarding friction and cognitive load.

### 3) Step drop-off rate

Definition:
- percent of users who stall at each step:
  - install
  - init
  - first structured write
  - first query
  - agent setup

Why it matters:
- localizes where docs need tightening.

### 4) Agent-next-step conversion

Definition:
- percent of first-loop completers who proceed to MCP setup.

Why it matters:
- validates agent-assisted onboarding for Raven's primary persona.

### 5) Reference dependency before activation

Definition:
- percent of users who need reference docs before first-loop completion.

Why it matters:
- high values indicate guides are missing critical explanations.

## Instrumentation points

### Immediate (no product telemetry required)

### A) Structured usability sessions

Run short onboarding sessions with 5-10 participants:
- observe completion/failure per step
- record timestamps and confusion moments
- capture quotes for qualitative context

### B) Onboarding feedback issue template

Create a GitHub issue template that asks:
- where did you hear about Raven?
- where did you get stuck? (install/init/write/query/agent setup)
- did you complete the first loop? (yes/no)
- what command or doc section was confusing?

### C) PR-level docs evaluation checklist

For onboarding doc PRs, require:
- expected funnel step affected
- hypothesized metric movement
- rollback criteria if confusion rises

### D) GitHub traffic trend checks

Review trend changes for:
- `README.md`
- `docs/guide/getting-started.md`
- `docs/guide/configuration.md`
- `docs/guide/schema-intro.md`

Use directional changes to detect gross regressions in discoverability.

## Optional future instrumentation (if product telemetry is introduced)

- `onboarding_step_completed` event with `step_name`
- `first_loop_completed` event
- `agent_setup_started` / `agent_setup_completed` events
- `reference_opened_pre_activation` event

Keep telemetry opt-in and privacy-conscious.

## Suggested review cadence

- Weekly: quick trend check for first-loop completion and drop-off hotspots
- Monthly: deeper analysis and docs IA adjustments
- Quarterly: revisit activation definition and persona assumptions

## Success thresholds (initial)

- First-loop completion rate: >= 70%
- Median time to first-loop completion: <= 10 minutes
- Agent-next-step conversion: >= 40%
- Reference dependency pre-activation: <= 25%

Thresholds should be updated once baseline data exists.

