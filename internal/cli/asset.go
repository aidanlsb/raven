package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

var assetCmd = buildRegistrySubtree(registrySubtreeSpec{
	Prefix:    []string{"asset"},
	VaultPath: getVaultPath,
	Root: registryGroup{
		Short:      "Manage vault assets",
		Long:       "Import and manage non-Markdown files under the configured vault asset root.",
		ParentOnly: true,
	},
	Renders: map[string]func(*cobra.Command, commandexec.Result) error{
		"asset_import": renderAssetImportResult,
	},
})

func init() {
	rootCmd.AddCommand(assetCmd)
}

func renderAssetImportResult(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	path := stringValue(data["path"])
	mode := stringValue(data["mode"])

	if boolValue(data["preview"]) {
		fmt.Println(ui.Star(fmt.Sprintf("Would %s asset to %s", mode, ui.FilePath(path))))
		fmt.Println(ui.Hint("Dry run: re-run without --dry-run to apply"))
		return nil
	}

	fmt.Println(ui.Checkf("Imported asset to %s (%s)", ui.FilePath(path), mode))
	return nil
}
