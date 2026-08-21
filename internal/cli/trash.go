package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var trashCmd = buildRegistrySubtree(registrySubtreeSpec{
	Prefix:    []string{"trash"},
	VaultPath: getVaultPath,
	Root: registryGroup{
		Short: "Inspect recoverable deleted files",
	},
	Renders: map[string]func(*cobra.Command, commandexec.Result) error{
		"trash_list": renderTrashList,
	},
})

var restoreCmd = newCanonicalLeafCommand("restore", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderRestore,
})

func renderTrashList(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	var entries []objectsvc.TrashEntry
	if err := decodeResultData(data["items"], &entries); err != nil {
		return handleError(ErrInternal, err, "")
	}
	if len(entries) == 0 {
		fmt.Println(ui.Hint("Trash is empty."))
		return nil
	}

	fmt.Printf("%s %s\n", ui.SectionHeader("Trash"), ui.Hint(stringValue(data["trash_dir"])+"/"))
	for _, entry := range entries {
		fmt.Printf("%s %s\n", ui.Bold.Render(entry.Reference), ui.Hint("("+entry.Kind+")"))
		fmt.Printf("  %s → %s\n", ui.FilePath(entry.TrashPath), ui.FilePath(entry.RestorePath))
	}
	return nil
}

func renderRestore(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	trashPath := stringValue(data["trash_path"])
	restorePath := stringValue(data["restore_path"])
	if boolValue(data["preview"]) {
		fmt.Printf("%s\n", ui.SectionHeader("Restore preview"))
		fmt.Printf("  %s → %s\n", ui.FilePath(trashPath), ui.FilePath(restorePath))
		fmt.Println(ui.Hint("Re-run with --confirm to restore."))
		return nil
	}

	fmt.Println(ui.Checkf("Restored %s", ui.FilePath(restorePath)))
	return nil
}

func init() {
	rootCmd.AddCommand(trashCmd)
	rootCmd.AddCommand(restoreCmd)
}
