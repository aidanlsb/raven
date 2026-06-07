# Assets

Assets are vault-local non-Markdown files such as images, PDFs, audio, videos, and datasets.

Assets are first-class graph resources, but they are not Raven object types. Object types in `schema.yaml` still describe Markdown-backed objects and sections. Asset metadata is derived from the filesystem and index, not authored in `raven.yaml`.

## Linking Assets

Use normal Markdown links and images with vault-relative paths when you want portable Markdown rendering:

```markdown
![System diagram](assets/photos/system.png)
[Original paper](assets/pdfs/paper.pdf)
```

After `rvn reindex`, Raven records those links in the reference graph. That means commands such as `rvn backlinks assets/pdfs/paper.pdf --json`, `rvn outlinks notes/design --json`, and RQL `refs(...)` predicates can see notes that reference an asset.

You can also use Raven wikilinks for semantic references:

```markdown
See [[assets/pdfs/paper.pdf]] for the original source.
See [[paper]] for the original source.
```

Short asset references resolve when they are unambiguous. `[[paper]]` can resolve to `assets/pdfs/paper.pdf` if no object or other asset claims the same short name. If both `paper.pdf` and `paper.png` exist, use the full path or extension-bearing filename.

Raven only indexes vault-local asset references. External URLs, anchors, mail links, and Markdown links to `.md` files are not asset references.

## Asset Directory

Configure asset behavior in `raven.yaml`:

```yaml
directories:
  assets: assets/
```

`directories.assets` is the vault-relative directory scanned for non-Markdown files. Raven derives each indexed asset's path, filename, extension, media type, size, and modification time. If you need authored metadata, create a Markdown object such as `paper`, `source`, or `photo_set` and link it to the asset.

Prefer the structured config commands when changing the asset directory:

```bash
rvn vault config directories get --json
rvn vault config directories set --assets assets --json
rvn reindex --json
```

Run `rvn reindex --json` after changing `directories.assets` so cached asset metadata is refreshed.

## Checks And Moves

`rvn check` reports:

- `missing_asset` when a Markdown asset link points to a file Raven cannot find.
- `orphaned_asset` when an indexed asset has no incoming references.

Use `rvn move` instead of shell `mv` for assets. It preserves vault safety checks, updates Markdown links/images that point to the moved asset, and refreshes the index.

```bash
rvn move assets/downloads/paper.pdf assets/pdfs/paper.pdf
rvn backlinks assets/pdfs/paper.pdf
rvn resolve paper
```
