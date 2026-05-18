# Assets

Assets are vault-local non-Markdown files such as images, PDFs, audio, videos, and datasets.

Assets are first-class graph resources, but they are not Raven object types. Object types in `schema.yaml` still describe Markdown-backed objects and sections. Asset kinds are separate organization and validation rules.

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

## Asset Kinds

Configure asset behavior in `raven.yaml`:

```yaml
assets:
  root: assets/
  kinds:
    photo:
      extensions: [jpg, jpeg, png, gif, webp, heic]
      media_types: [image/]
      default_path: photos/
    pdf:
      extensions: [pdf]
      media_types: [application/pdf]
      default_path: pdfs/
```

Kinds classify assets and define preferred placement. They do not define metadata fields, create schema types, or make assets queryable by frontmatter. If you need authored metadata, create a Markdown object such as `paper`, `source`, or `photo_set` and link it to the asset.

## Checks And Moves

`rvn check` reports:

- `missing_asset` when a Markdown asset link points to a file Raven cannot find.
- `orphaned_asset` when an indexed asset has no incoming references.
- `non_canonical_asset` when an asset is outside its kind's `default_path`.

Use `rvn move` instead of shell `mv` for assets. It preserves vault safety checks, updates Markdown links/images that point to the moved asset, and refreshes the index.

```bash
rvn move assets/downloads/paper.pdf assets/pdfs/paper.pdf
rvn backlinks assets/pdfs/paper.pdf
rvn resolve paper
```
