# Raven Keep Admin

Use this skill for keep setup, active/default keep selection, and global Raven config.

## Operating rules

- Use explicit keep naming and avoid guessing which keep should be active.
- Prefer `rvn keep ...` and `rvn config ...` over manual file edits in machine config.
- Use `--json` for deterministic automation output.

## Typical flow

1. Bootstrap or register keeps (`rvn init`, `rvn keep add`).
2. Set routing defaults (`rvn keep use`, optional `rvn keep pin`).
3. Confirm current resolution (`rvn keep current`, `rvn keep path`, `rvn path`).
4. Manage global settings (`rvn config show`, `rvn config set`, `rvn config unset`).

## Safety

- On `rvn keep remove`, respect guard flags when removing active/default entries.
- Keep `default_keep` and `active_keep` coherent to avoid unexpected fallback behavior.

## Reference

- End-to-end command sequences and gotchas: `references/keep-lifecycle.md`
