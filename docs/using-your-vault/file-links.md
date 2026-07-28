# File Links

Non-Markdown files such as images, PDFs, audio, videos, and datasets can live
anywhere inside a Raven vault. They are ordinary files, not Raven entities.

Copy a file into the vault with normal filesystem tools, then reindex:

```bash
cp ~/Downloads/paper.pdf ~/notes/files/paper.pdf
rvn reindex --json
```

Use standard Markdown links and images:

```markdown
[Original paper](../files/paper.pdf)
![System diagram](../files/system.png)
```

Do not use a wikilink for a file. `[[...]]` resolves only Raven objects and
sections.

## Querying Links

Raven indexes outgoing Markdown links and images as edges. Query the edges
directly with the `link` root:

```bash
rvn query 'link .ext==pdf' --json
rvn query 'link .is_image==true within(type:project)' --json
```

Or filter objects, sections, and traits by their outgoing links:

```bash
rvn query 'type:project links(.ext==pdf)' --json
rvn query 'section links(.scheme==url)' --json
```

## Checking And Moving Files

`rvn check` reports a `broken_file_link` when a local file target does not
exist. URL targets are never fetched.

Use `rvn move` for a file already inside the vault. Raven rewrites inbound
Markdown file links matched by normalized target path:

```bash
rvn move files/paper.pdf files/archive/paper.pdf --json
```

Copying a new external file does not require a Raven command. Run `rvn reindex`
after the copy so any changed Markdown link edges are current.
