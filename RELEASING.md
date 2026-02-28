# Releasing Raven

Maintainer runbook for shipping a tagged release.

Raven uses a tag-driven release flow:
1. Create an annotated semver tag from `main`.
2. Push the tag.
3. GitHub Actions runs preflight and publishes with GoReleaser.

## Preconditions

- Working tree is clean.
- Current branch is `main`.
- `origin` points to `aidanlsb/raven`.
- GitHub Actions secrets:
  - `GITHUB_TOKEN` (provided automatically by Actions)
  - `HOMEBREW_TAP_TOKEN` (required only for Homebrew formula publishing)

## Homebrew One-Time Setup

Raven publishes formulas to `aidanlsb/homebrew-tap` (`aidanlsb/tap` in brew commands).

Before the first Homebrew-enabled release, confirm:
1. `aidanlsb/homebrew-tap` exists and is writable by the token owner.
2. The tap repo contains a `Formula/` directory.
3. `HOMEBREW_TAP_TOKEN` is set in `aidanlsb/raven` repository secrets.

If `HOMEBREW_TAP_TOKEN` is missing, release binaries still publish, but Homebrew formula updates are skipped.

## Local Preflight

```bash
make release-preflight
```

This runs formatting checks, lint, unit tests, and integration tests.

## Release Commands

Pick one approach:

```bash
make release VERSION=v0.2.0
```

Or compute and release the next version automatically:

```bash
make release-auto BUMP=patch
```

Preview only (no tag created):

```bash
make release-next BUMP=patch
```

Supported bump values: `patch`, `minor`, `major`.

## What CI Does On Tag Push

`.github/workflows/release.yml` runs when a `v*.*.*` tag is pushed and:
1. Validates semver tag format.
2. Verifies the pushed tag is annotated.
3. Runs `make release-preflight`.
4. Runs GoReleaser to publish release artifacts and GitHub release notes.
5. Updates `aidanlsb/homebrew-tap/Formula/raven.rb` when `HOMEBREW_TAP_TOKEN` is available.

## Post-Release Verification

1. Confirm the GitHub Release has binaries plus `checksums.txt`.
2. Confirm `aidanlsb/homebrew-tap` has a new formula commit.
3. Verify Homebrew install:

```bash
brew tap aidanlsb/tap
brew install raven
rvn version
```
