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
		"section_create": renderSectionCreateResult,
		"section_move":   renderSectionMoveResult,
		"section_rename": renderSectionRenameResult,
	},
})

func init() {
	rootCmd.AddCommand(sectionCmd)
}

func renderSectionCreateResult(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	sectionID := stringValue(data["section"])

	if boolValue(data["preview"]) {
		fmt.Println(ui.Star(fmt.Sprintf("Would create section %s", ui.FilePath(sectionID))))
		fmt.Println(ui.Hint("Dry run: re-run without --dry-run to apply"))
		return nil
	}

	fmt.Println(ui.Checkf("Created section %s", ui.FilePath(sectionID)))
	return nil
}

func renderSectionMoveResult(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	sectionID := stringValue(data["section"])
	placement := stringValue(data["placement"])
	anchor := stringValue(data["anchor"])
	description := placement
	if anchor != "" {
		description += " " + anchor
	}

	if boolValue(data["preview"]) {
		fmt.Println(ui.Star(fmt.Sprintf("Would move section %s %s", ui.FilePath(sectionID), description)))
		fmt.Println(ui.Hint("Dry run: re-run without --dry-run to apply"))
		return nil
	}

	fmt.Println(ui.Checkf("Moved section %s %s", ui.FilePath(sectionID), description))
	return nil
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
