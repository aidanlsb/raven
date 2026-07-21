package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

var sectionCmd = buildRegistrySubtree(registrySubtreeSpec{
	Prefix:    []string{"section"},
	VaultPath: getVaultPath,
	Root: registryGroup{
		Short:      "Manage Markdown sections",
		Long:       "Manage Markdown-derived sections without editing headings or references by hand.",
		ParentOnly: true,
	},
	Renders: map[string]func(*cobra.Command, commandexec.Result) error{
		"section_rename": renderSectionRenameResult,
	},
})

func init() {
	rootCmd.AddCommand(sectionCmd)
}

func renderSectionRenameResult(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	source := stringValue(data["source"])
	destination := stringValue(data["destination"])

	if boolValue(data["preview"]) {
		fmt.Println(ui.Star(fmt.Sprintf("Would rename section %s → %s", ui.FilePath(source), ui.FilePath(destination))))
		if updatedRefs := stringSliceFromAny(data["updated_refs"]); len(updatedRefs) > 0 {
			fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Would update %d references", len(updatedRefs))))
		}
		fmt.Println(ui.Hint("Dry run: re-run without --dry-run to apply"))
		return nil
	}

	fmt.Println(ui.Checkf("Renamed section %s → %s", ui.FilePath(source), ui.FilePath(destination)))
	if updatedRefs := stringSliceFromAny(data["updated_refs"]); len(updatedRefs) > 0 {
		fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Updated %d references", len(updatedRefs))))
	}
	return nil
}
