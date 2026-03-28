# Installation

This guide covers installing Raven and verifying that the CLI is available before you create a vault.

## Choose an install method

### Homebrew

Recommended if you want a normal macOS CLI install path and easy upgrades.

```bash
brew tap aidanlsb/tap
brew install aidanlsb/tap/rvn
rvn version
```

### Go install

Recommended if you already use Go tooling or want the latest tagged build via `go install`.

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
rvn version
```

## Verify the binary is available

These should work after installation:

```bash
rvn version
rvn help
```

If `rvn` is not found, your install succeeded but the binary directory is not on your shell `PATH`.

## Common `PATH` fix for Go installs

The Go binary usually lands in one of these:

- `$(go env GOPATH)/bin`
- `$(go env GOBIN)` if you set `GOBIN`

Check where Go expects binaries:

```bash
go env GOPATH
go env GOBIN
```

Typical shell profile fix:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

Then restart your shell and rerun:

```bash
rvn version
```

## First validation pass

Before creating a vault, make sure the CLI responds normally:

```bash
rvn version
rvn help
rvn docs
```

If those work, continue to `getting-started/first-vault.md`.

## Upgrading

Homebrew:

```bash
brew update
brew upgrade aidanlsb/tap/rvn
```

Go:

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
```

## Next step

Continue with `getting-started/first-vault.md` to initialize your first vault and inspect the files Raven creates.
