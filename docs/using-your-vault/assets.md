# Assets

Assets are vault-local non-Markdown files such as images, PDFs, audio, videos, and datasets.

Assets are first-class graph resources, but they are not Raven object types. Object types in `schema.yaml` still describe Markdown-backed objects and sections. Asset kinds are separate organization and validation rules.

## Linking Assets

Use normal Markdown links and images with vault-relative paths:

```markdown
![System diagram](assets/photos/system.png)
[Original paper](assets/pdfs/paper.pdf)
```

After `rvn reindex`, Raven records those links in the reference graph. That means commands such as `rvn backlinks assets/pdfs/paper.pdf --json`, `rvn outlinks notes/design --json`, and RQL `refs(...)` predicates can see notes that reference an asset.

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

Kinds classify assets and define preferred placement. They do not define metadata fields. If you need authored metadata, create a Markdown object such as `paper`, `source`, or `photo_set` and link it to the asset.

## Checks And Moves

`rvn check` reports:

- `missing_asset` when a Markdown asset link points to a file Raven cannot find.
- `orphaned_asset` when an indexed asset has no incoming references.
- `non_canonical_asset` when an asset is outside its kind's `default_path`.

Use `rvn move` instead of shell `mv` for assets. It preserves vault safety checks, updates Markdown links/images that point to the moved asset, and refreshes the index.
