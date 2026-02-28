# Hooks Removal Decision

Date: 2026-02-28

## Summary

Raven hooks were removed from core scope for now.

The feature enabled event-driven automation, but it also added security and maintenance cost because vault config could execute arbitrary shell commands. Current expected usage appears niche compared to the complexity it introduces into core command execution and configuration.

## Decision

- Remove built-in hook runtime, hook config keys, and `rvn hook`.
- Keep automation as explicit orchestration using wrappers, workflows, file watchers, and CI.
- Revisit only if concrete, repeated use cases show that command-semantic lifecycle triggers must be first-class in Raven.

## Recommended Alternatives

- Wrapper scripts/functions for command-semantic automation (for example, `rvn edit` then `rvn check` then git sync).
- Raven workflows for explicit multi-step automations.
- File watchers (`watchexec`, `entr`, `fswatch`) for background reactions.
- Git hooks / CI for validation and sync policies.

## Why This Direction

- Improves trust boundaries in core Raven.
- Keeps automation explicit and auditable.
- Preserves power-user flexibility without expanding implicit runtime behavior.

## Project Link

- [Raven on GitHub](https://github.com/aidanlsb/raven)
