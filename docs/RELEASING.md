# Releasing Raven

This repository uses a tag-driven release flow.

1. Cut an annotated semver tag from `main`.
2. Push the tag.
3. GitHub Actions runs preflight checks and publishes with GoReleaser.

## Requirements

- Clean working tree
- On `main`
- `origin` remote configured
- GitHub secrets configured:
  - `GITHUB_TOKEN` (provided automatically by Actions)
  - `HOMEBREW_TAP_TOKEN` (optional, for Homebrew formula publishing)

## Local Preflight

Run the same checks as release CI:

```bash
make release-preflight
```

## Cut and Publish (One Command)

```bash
make release VERSION=v0.2.0
```

This command:

1. Validates semver tag format.
2. Verifies clean tree and `main` branch.
3. Runs `make release-preflight`.
4. Creates an annotated tag.
5. Pushes the tag to `origin`.

Pushing the tag triggers [`.github/workflows/release.yml`](../.github/workflows/release.yml), which:

1. Re-validates tag format.
2. Verifies tag is annotated.
3. Runs `make release-preflight` again.
4. Runs GoReleaser to publish binaries/checksums and GitHub Release notes.

## Codex Project Skill

This repo includes a project-local Codex skill at:

- `.codex/skills/raven-release/SKILL.md`

Use it when you want Codex to drive the release workflow with the same guardrails:

- Run release preflight only
- Create a release tag without pushing
- Cut and publish a full release via `make release`

## Notes on Changelog

- `CHANGELOG.md` is for curated human-written notes.
- GitHub release notes are generated from commits by GoReleaser.
- Prefer conventional commit prefixes (`feat:`, `fix:`, etc.) so release notes group cleanly.
