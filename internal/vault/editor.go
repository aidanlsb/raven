package vault

import (
	"os/exec"

	"github.com/ravenscroftj/raven/internal/config"
)

// OpenInEditor opens a file in the user's configured editor.
// If no editor is configured, this does nothing.
// The process is started in the background (non-blocking).
func OpenInEditor(cfg *config.Config, filePath string) {
	if cfg == nil {
		return
	}

	editor := cfg.GetEditor()
	if editor == "" {
		return
	}

	cmd := exec.Command(editor, filePath)
	cmd.Start() // Non-blocking
}
