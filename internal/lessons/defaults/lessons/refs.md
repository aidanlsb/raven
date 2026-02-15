---
title: Linking with Refs
prereqs:
  - objects
---

# Linking with Refs

Use `[[wikilinks]]` to connect notes and typed objects. Raven indexes these
references so you can inspect backlinks and query relationships.

## Example

```md
---
type: meeting
with: "[[people/alice]]"
---

# Weekly Sync

Discussed API rollout with Alice.
```

## Try It

After creating `people/alice`, create a meeting that links to that person and
run:

```bash
rvn backlinks people/alice
```
