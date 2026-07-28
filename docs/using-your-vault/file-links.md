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

These forms render correctly when Markdown is converted to HTML, so they remain
portable across viewers. Do not use a wikilink for a file: `[[...]]` is
reserved for Raven object and section references and does not render as a file
link or image.

Use `[[...]]` object or section references for Markdown notes inside the vault.
`rvn check` flags standard Markdown links or images targeting in-vault `.md`
notes as `markdown_link_to_vault_note` because Raven cannot track them as
references for backlinks or rewrites.

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

The `.normalized_key` field is intentionally conservative. For URLs, Raven
lowercases the host and strips only default ports (`:80` for HTTP and `:443`
for HTTPS); path case, the query string, trailing slash, and fragment are
preserved. File targets inside the vault use a vault-relative POSIX key;
absolute targets outside the vault remain absolute.

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
