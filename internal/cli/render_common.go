package cli

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

// renderObjectCreated renders a simple "Created <path>" message with optional ID
func renderObjectCreated(file, id string) {
	fmt.Println(ui.Checkf("Created %s", ui.FilePath(file)))
	if id != "" {
		fmt.Println(ui.LinkAs(id))
	}
}

// renderObjectUpdated renders a simple "Updated <path>" message
func renderObjectUpdated(file string) {
	fmt.Println(ui.Checkf("Updated %s", ui.FilePath(file)))
}

// renderObjectDeleted renders a simple "Deleted <path>" message
func renderObjectDeleted(file string) {
	fmt.Println(ui.Checkf("Deleted %s", ui.FilePath(file)))
}

// renderWithWarningsAndPrompt renders warnings and prompts for missing refs
func renderWithWarningsAndPrompt(vaultPath string, result commandexec.Result) {
	for _, warning := range result.Warnings {
		fmt.Printf("  %s\n", ui.Warningf("%s: %s", warning.Code, warning.Message))
		if warning.CreateCommand != "" {
			fmt.Printf("    %s\n", ui.Hint("→ "+warning.CreateCommand))
		}
	}
	promptCreateMissingRefsFromResult(vaultPath, result)
}
