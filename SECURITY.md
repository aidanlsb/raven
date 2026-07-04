# Security Policy

## Reporting a Vulnerability

Please report suspected vulnerabilities privately through
[GitHub's private vulnerability reporting](https://github.com/aidanlsb/raven/security/advisories/new)
(the "Report a vulnerability" button on the repository's Security tab).

Do not open a public issue for security problems. Public issues are fine for
ordinary bugs, crashes, and data-integrity problems that have no security
impact.

When reporting, include:

- The Raven version (`rvn version`) and operating system
- Reproduction steps or a proof of concept
- The impact you believe the issue has (what an attacker could do)

You should receive an acknowledgement within a few days. Confirmed
vulnerabilities are fixed in the next release, and the changelog notes the fix
under a `Security` heading. There is no bug bounty program.

## Supported Versions

Raven is pre-1.0. Only the latest tagged release receives fixes; there are no
maintenance branches for older versions. If you find a vulnerability in an
older release, verify it still exists in the latest release before reporting.

## Trust Model

Understanding what Raven does and does not protect against helps you decide
whether an issue is a vulnerability.

### Local-first, no telemetry

Raven is a local CLI. It sends no telemetry or analytics. The only network
access is user-initiated:

- `rvn init` and `rvn docs fetch` download the documentation archive from
  GitHub (`codeload.github.com`, overridable with `--source`)
- installs and upgrades via Homebrew or `go install`

### Vault scoping

All vault operations are confined to the vault directory:

- Paths are validated before use, and symlinks are resolved so that a link
  pointing outside the vault cannot be read or written through Raven
- `.raven/`, `.trash/`, `.git/`, `raven.yaml`, and `schema.yaml` are protected
  from content-mutation commands, along with any configured
  `protected_prefixes`
- Paths matched by `exclude` patterns are outside Raven's managed content
  model entirely

Escapes from these boundaries (reading or writing outside the vault via a
crafted path, reference, or symlink) are security bugs. Please report them.

### MCP server and AI agents

`rvn serve` speaks JSON-RPC over stdin/stdout. There is no network listener
and no authentication layer: **any MCP client you connect has the same power
over the vault as you do running `rvn` yourself**, including creating,
modifying, and deleting notes.

Treat connecting an agent to your vault as granting it full read/write access
to that vault's contents. Raven's safety measures for agents (preview-first
bulk operations, confirm flags, protected paths) reduce the blast radius of
mistakes, but they are not a security boundary against a malicious or
compromised agent.

### Local configuration is trusted

Raven trusts local configuration by design. For example, the `editor` setting
in `config.toml` is executed to open files. Anyone who can edit your Raven
configuration or your vault files already has your local privileges; issues
requiring that level of access are generally not treated as vulnerabilities.
