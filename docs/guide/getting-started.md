# Getting started

## Install

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
```

## Create a vault

```bash
rvn init /path/to/notes
```

This creates:
- `schema.yaml` — types + traits
- `raven.yaml` — vault config (saved queries, workflows, etc)
- `.raven/` — local index + metadata (gitignored)

## Configure your default vault (optional)

Raven looks for a global config at `~/.config/raven/config.toml` (or your OS config dir).

Simple (legacy) single-vault config:

```toml
vault = "/path/to/notes"
editor = "code"
```

Named vaults:

```toml
default_vault = "work"
editor = "code"

[vaults]
work = "/path/to/notes"
personal = "/path/to/personal-notes"
```

## First commands

```bash
rvn daily              # open/create today’s daily note
rvn add "Quick note"   # append to capture destination (default: today)
rvn reindex            # build/update the index
rvn query --list       # list saved queries from raven.yaml
```

## Next steps

- Read `core-concepts.md` to understand types, traits, and references
- See `cli.md` for common patterns and workflows
- Keep `reference/cli.md` open for the complete command reference
- Explore `reference/query-language.md` for powerful querying

