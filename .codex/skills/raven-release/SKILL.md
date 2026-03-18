---
name: raven-release
description: Cut, validate, and publish Raven releases with the repository's guarded flow. Use when asked to prepare a release, tag a version, publish artifacts, verify release readiness, or automate the Raven release process end-to-end.
---

# Raven Release

Use the repository release primitives as the source of truth:

- `make release-preflight`
- `make release-next BUMP=patch|minor|major`
- `make release-auto BUMP=patch|minor|major`
- `make release-tag VERSION=vX.Y.Z`
- `make release VERSION=vX.Y.Z`

Do not bypass them with ad-hoc tagging or direct GoReleaser invocation unless the user explicitly asks.

Read `RELEASING.md` before cutting a release. Treat it as the maintainer runbook for changelog requirements, CI behavior, and Homebrew publishing expectations.

## Inputs

Require a semver tag in `vX.Y.Z` format (or prerelease like `vX.Y.Z-rc.1`).

If no version is provided, ask for it. Do not infer bump level silently.

## Default Workflow

1. Confirm branch and tree state:
   - Ensure on `main`.
   - Ensure working tree is clean.
   - Ensure `CHANGELOG.md` contains the exact `## [<tag>]` heading for the release.
2. Run preflight:
   - `make release-preflight`
3. Publish:
   - `make release VERSION=<tag>`
4. Report:
   - Tag pushed
   - Release workflow triggered
   - Whether GitHub Release artifacts and Homebrew publishing still need verification
   - Any follow-up actions if checks fail

## Safe Variants

- Validation-only: run `make release-preflight` and stop.
- Compute-only: run `make release-next BUMP=<patch|minor|major>` and stop.
- Auto-bump publish: run `make release-auto BUMP=<patch|minor|major>`.
- Tag-only (no push): run `make release-tag VERSION=<tag>` and stop.
- Full publish: run `make release VERSION=<tag>`.

## Failure Handling

- If preflight fails, stop and summarize actionable failures.
- If tag already exists (local or origin), stop and ask for a new version.
- If push succeeds but release workflow fails, surface failing step names and logs.
- If the GitHub release succeeds but Homebrew publishing is skipped, call out the missing `HOMEBREW_TAP_TOKEN` or tap-side follow-up explicitly.

## Optional Monitoring

If `gh` is available and authenticated, monitor release run status:

```bash
gh run list --workflow Release --limit 5
gh run view <run-id> --log-failed
```

## Post-Release Checks

- Confirm the GitHub Release contains binaries and `checksums.txt`.
- Confirm the `aidanlsb/homebrew-tap` formula update landed when Homebrew publishing is enabled.
- Report any verification you did not perform yet instead of implying the release is fully validated.
