# Installation

This guide covers installing Raven and verifying that the CLI is available before you create a vault.

## Choose an install method

### Homebrew (macOS and Linux)

Recommended if you use Homebrew and want easy upgrades.

```bash
brew tap aidanlsb/tap
brew install aidanlsb/tap/rvn
rvn version
```

### Release binary (all platforms)

Every release publishes prebuilt binaries for Linux, macOS, and Windows (amd64 and arm64) on the [GitHub releases page](https://github.com/aidanlsb/raven/releases/latest).

Download the archive for your platform, verify it against `checksums.txt` if you like, and place the `rvn` binary somewhere on your `PATH`. For example, on Linux (x86_64), substituting the current version number:

```bash
VERSION=0.0.32   # See the releases page for the latest version
curl -LO "https://github.com/aidanlsb/raven/releases/download/v${VERSION}/raven_${VERSION}_linux_x86_64.tar.gz"
tar -xzf "raven_${VERSION}_linux_x86_64.tar.gz" rvn
sudo install rvn /usr/local/bin/rvn
rvn version
```

Archives are named `raven_<version>_<os>_<arch>.tar.gz` with OS `linux`, `darwin`, or `windows` and architecture `x86_64` or `arm64`. Windows archives are `.zip` files; extract `rvn.exe` and add its directory to your `PATH`.

On macOS, binaries are not currently signed or notarized, so Gatekeeper may quarantine a manually downloaded binary. Either install via Homebrew (which avoids this) or remove the quarantine attribute after verifying the checksum:

```bash
xattr -d com.apple.quarantine ./rvn
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
```

If those work, continue to `getting-started/first-vault.md`. `rvn init` attempts
to fetch the global docs cache; before initialization, run `rvn docs fetch`
explicitly if you want `rvn docs` available.

## Upgrading

Homebrew:

```bash
brew update
brew upgrade aidanlsb/tap/rvn
```

Release binary: download the new archive from the releases page and replace the installed binary.

Go:

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
```

After upgrading, the next docs read (`rvn docs`, `rvn docs list`, or
`rvn docs search`) checks an existing global docs cache and lazily refreshes it
from the installed Raven version tag. If the refresh cannot reach the network,
Raven warns and continues serving the existing cache.

Use `docs fetch` to force a refresh or pin another ref:

```bash
rvn docs fetch                       # Force-refresh from the default ref
rvn docs fetch --ref v0.0.32         # Pin a specific tag
```

A missing cache is not fetched implicitly; run `rvn docs fetch` to create it.

## Next step

Continue with `getting-started/first-vault.md` to initialize your first vault and inspect the files Raven creates.
