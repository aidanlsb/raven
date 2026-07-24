package commands

var assetRegistry = map[string]Meta{
	"asset_import": {
		Name:        "asset import",
		CLIPath:     []string{"asset", "import"},
		Use:         "import <source> <destination>",
		Description: "Import an external file into the vault asset directory",
		Category:    CategoryContent,
		Access:      AccessWrite,
		Risk:        RiskDestructive,
		VaultScope:  VaultScopeRequired,
		LongDesc: `Import a host filesystem file into the configured vault asset root.

The source must be an absolute or ~-relative host path to a regular non-Markdown
file outside the vault. It is not a Raven reference. The destination is a
vault-relative path under directories.assets. Pass a full destination filename,
or a directory ending with "/" (or an existing directory) to preserve the
source basename. The final destination must include a file extension.

Imports copy by default and create parent directories as needed. Pass --move to
remove the source only after the destination write and index handoff succeed.
If the destination exists, the command fails unless --force is supplied; Raven
never generates an automatic suffix. Pass --dry-run to validate and preview the
resolved destination without writing or removing anything.

Use 'rvn move' instead when the source is already inside the vault so references
and both sides of the index mutation remain consistent.`,
		Args: []ArgMeta{
			{Name: "source", Description: "Absolute or ~-relative host filesystem path to a regular non-Markdown file", Required: true, Examples: []string{"/tmp/paper.pdf", "~/Downloads/photo.png"}},
			{Name: "destination", Description: "Vault-relative file or directory path under directories.assets", Required: true, Examples: []string{"assets/pdfs/paper.pdf", "assets/photos/"}},
		},
		Flags: []FlagMeta{
			{Name: "move", Description: "Remove the source after the imported asset is written and handed to the index", Type: FlagTypeBool},
			{Name: "force", Description: "Overwrite an existing destination file", Type: FlagTypeBool},
			{Name: "dry-run", Description: "Preview the resolved import without writing or removing files", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn asset import ~/Downloads/paper.pdf assets/pdfs/ --json",
			"rvn asset import /tmp/photo.png assets/photos/hero.png --move --json",
			"rvn asset import /tmp/report.pdf assets/pdfs/report.pdf --force --json",
			"rvn asset import /tmp/data.csv assets/data/ --dry-run --json",
		},
		UseCases: []string{
			"Copy an external image, PDF, audio file, video, or dataset into the vault",
			"Move a downloaded non-Markdown file into the configured asset directory",
			"Preview and validate an asset destination before importing",
		},
	},
}
