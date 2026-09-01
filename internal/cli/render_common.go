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

// renderObjectUpdatedWithID renders "Updated <path>" with an optional ID
func renderObjectUpdatedWithID(file, id string) {
	renderObjectUpdated(file)
	if id != "" {
		fmt.Println(ui.LinkAs(id))
	}
}

// renderObjectAdded renders a simple "Added to <path>" message
func renderObjectAdded(file string) {
	fmt.Println(ui.Checkf("Added to %s", ui.FilePath(file)))
}

// renderObjectDeleted renders a simple "Deleted <path>" message
func renderObjectDeleted(file string) {
	fmt.Println(ui.Checkf("Deleted %s", ui.FilePath(file)))
}

// renderObjectMoved renders "Moved to <path>" message
func renderObjectMoved(destination string) {
	fmt.Println(ui.Checkf("Moved to %s", ui.FilePath(destination)))
}

// renderSectionCreated renders a simple "Created section <path>" message
func renderSectionCreated(section string) {
	fmt.Println(ui.Checkf("Created section %s", ui.FilePath(section)))
}

// renderSectionDeleted renders a simple "Deleted section <path>" message with file context
func renderSectionDeleted(section, file string, lineStart, lineEnd int) {
	fmt.Println(ui.Checkf(
		"Deleted section %s from %s (lines %d-%d)",
		ui.FilePath(section),
		ui.FilePath(file),
		lineStart,
		lineEnd,
	))
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
