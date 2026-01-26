package vault

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
)

// OpenInEditor opens a file in the user's configured editor.
// Returns true if the editor was launched, false otherwise.
// The process is started in the background (non-blocking).
//
// Note: If your vault path contains spaces (e.g., iCloud paths like
// "Mobile Documents"), some editors may have issues. Use a symlink
// to a path without spaces as a workaround.
func OpenInEditor(cfg *config.Config, filePath string) bool {
	if cfg == nil {
		return false
	}

	editor := cfg.GetEditor()
	if editor == "" {
		return false
	}

	var cmd *exec.Cmd

	// If editor contains spaces, it's a compound command like "open -a Cursor"
	// Execute via shell to handle this correctly
	if strings.Contains(editor, " ") {
		cmd = exec.Command("sh", "-c", editor+" "+shellQuote(filePath))
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
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// OpenFilesInEditor opens multiple files in the user's configured editor.
// Returns true if the editor was launched, false otherwise.
// The process is started in the background (non-blocking).
func OpenFilesInEditor(cfg *config.Config, filePaths []string) bool {
	if cfg == nil || len(filePaths) == 0 {
		return false
	}

	editor := cfg.GetEditor()
	if editor == "" {
		return false
	}

	var cmd *exec.Cmd

	// If editor contains spaces, it's a compound command like "open -a Cursor"
	// Execute via shell to handle this correctly
	if strings.Contains(editor, " ") {
		quotedPaths := make([]string, len(filePaths))
		for i, p := range filePaths {
			quotedPaths[i] = shellQuote(p)
		}
		cmd = exec.Command("sh", "-c", editor+" "+strings.Join(quotedPaths, " "))
	} else {
		cmd = exec.Command(editor, filePaths...)
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("Warning: failed to open editor '%s': %v\n", editor, err)
		return false
	}
	return true
}

// OpenInEditorOrPrintPath opens a file in the editor, or prints the path if no editor is configured.
func OpenInEditorOrPrintPath(cfg *config.Config, filePath string) {
	if !OpenInEditor(cfg, filePath) {
		fmt.Printf("Open: %s\n", filePath)
		fmt.Println("(Set 'editor' in ~/.config/raven/config.toml or $EDITOR to open automatically)")
	}
}
