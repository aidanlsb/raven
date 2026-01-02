package vault

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/ravenscroftj/raven/internal/config"
)

// OpenInEditor opens a file in the user's configured editor.
// Returns true if the editor was launched, false otherwise.
// The process is started in the background (non-blocking).
//
// On macOS, if the editor contains spaces (e.g., "open -a Cursor"),
// it will be executed via shell to handle the arguments correctly.
func OpenInEditor(cfg *config.Config, filePath string) bool {
	if cfg == nil {
		return false
	}

	editor := cfg.GetEditor()
	if editor == "" {
		return false
	}

	var cmd *exec.Cmd

	// If editor contains spaces, it might be a compound command like "open -a Cursor"
	// Execute via shell to handle this correctly
	if strings.Contains(editor, " ") {
		if runtime.GOOS == "darwin" {
			// macOS: use sh -c
			cmd = exec.Command("sh", "-c", editor+" "+shellQuote(filePath))
		} else {
			// Linux/other: use sh -c
			cmd = exec.Command("sh", "-c", editor+" "+shellQuote(filePath))
		}
	} else {
		cmd = exec.Command(editor, filePath)
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("Warning: failed to open editor '%s': %v\n", editor, err)
		return false
	}
	return true
}

// shellQuote quotes a string for safe use in shell commands.
func shellQuote(s string) string {
	// Use single quotes and escape any single quotes in the string
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// OpenInEditorOrPrintPath opens a file in the editor, or prints the path if no editor is configured.
func OpenInEditorOrPrintPath(cfg *config.Config, filePath string) {
	if !OpenInEditor(cfg, filePath) {
		fmt.Printf("Open: %s\n", filePath)
		fmt.Println("(Set 'editor' in ~/.config/raven/config.toml or $EDITOR to open automatically)")
	}
}
