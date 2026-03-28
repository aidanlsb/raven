# Raven Core

Use this skill for day-to-day Raven operations when the user needs safe create, read, update, move, or delete flows.

## Operating rules

- Prefer `rvn` CLI with `--json` for deterministic machine-readable output.
- When operating through Raven MCP, use the equivalent Raven MCP tools instead of shelling out redundantly.
- Prefer Raven commands over direct file writes or shell text manipulation.
- Choose the smallest mutation primitive that matches the user's intent.
- Read with `rvn read --raw` before constructing `rvn edit` replacements.
- For bulk or destructive operations, preview first, then apply with confirmation.

## Choose the right command

- New object identity: `rvn new`
- Append log or capture text: `rvn add`
- Idempotent generated output: `rvn upsert`
- Frontmatter-only update: `rvn set`
- Exact body replacement: `rvn edit`
- Trait value update: `rvn update`
- Type correction or conversion: `rvn reclassify`
- Safe move or rename: `rvn move`
- Safe removal: `rvn delete`

## Typical flow

1. Inspect context: `rvn schema`, `rvn resolve`, `rvn read --raw`.
2. Choose a write primitive:
- Create a brand-new object: `rvn new`
- Append history that should accumulate on reruns: `rvn add`
- Write canonical output that should converge on reruns: `rvn upsert`
- Update structured metadata without changing body text: `rvn set`
- Replace a unique exact string in the body: `rvn edit`
- Change trait state after finding trait IDs with `rvn query --ids`: `rvn update`
- Change the object's type and let Raven handle defaults and field drops: `rvn reclassify`
3. Retrieve and analyze: `rvn query`, `rvn search`, `rvn backlinks`.
4. Move or delete with Raven commands rather than shell tools.

## Load references as needed

- Command chooser and CLI snippets: `references/command-map.md`
- Use `raven-query-advanced` when the user needs RQL syntax help, complex predicate composition, saved query authoring, or query troubleshooting.

## Safety

- Avoid shell-level `rm`, `mv`, `sed`, or `awk` for vault objects when Raven commands exist.
- Keep path operations vault-relative where possible.
- If `reclassify` reports dropped fields or missing required fields, stop and resolve that explicitly instead of guessing.
