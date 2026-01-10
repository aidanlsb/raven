<p align="center">
  <img src="raven-logo.svg" width="120" alt="Raven logo">
</p>

<h1 align="center">Raven</h1>

<p align="center">
  <strong>
    Plain markdown notes with schemas, traits, and querying.<br>
    Built around a CLI that also exposes MCP tools for agents.
  </strong>
</p>

> ⚠️ **Experimental:** Raven is early and under active development.

## What it looks like

Write normal markdown (optionally with traits and refs):

```markdown
# Friday, Jan 9, 2026

- @due(today) Send [[clients/midgard]] proposal
- @highlight Buffer time is the key to good estimates
```

Query it:

```bash
rvn query "trait:due value:today"
rvn backlinks clients/midgard
```

## Install

```bash
go install github.com/aidanlsb/raven/cmd/rvn@latest
```

## Quickstart

```bash
rvn init /path/to/notes
rvn reindex
rvn daily
rvn query --list
```

## Documentation

Start here: `docs/README.md`.

## Development

```bash
go test ./...
go build ./cmd/rvn
```

## License

MIT

