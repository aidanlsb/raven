package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"

	"github.com/aidanlsb/raven/internal/config"
)

// hyperlinkEnabled caches whether we should emit hyperlinks.
// Hyperlinks are only emitted to TTY terminals, not JSON output or pipes.
var hyperlinkEnabled *bool

// hyperlinksDisabled forces hyperlinks off for the current run (e.g. --no-links).
var hyperlinksDisabled bool

func setHyperlinksDisabled(disabled bool) {
	hyperlinksDisabled = disabled
	// Reset cached decision so changes take effect immediately.
	hyperlinkEnabled = nil
}

// shouldEmitHyperlinks returns true if we should emit OSC 8 hyperlinks.
func shouldEmitHyperlinks() bool {
	if hyperlinkEnabled != nil {
		return *hyperlinkEnabled
	}

	// Don't emit hyperlinks for JSON output or non-TTY
	enabled := !jsonOutput && isatty.IsTerminal(os.Stdout.Fd()) && !hyperlinksDisabled
	hyperlinkEnabled = &enabled
	return enabled
}

// buildEditorURL builds the appropriate URL for the configured editor.
func buildEditorURL(cfg *config.Config, absPath string, line int) string {
	editor := ""
	if cfg != nil {
		editor = cfg.GetEditor()
	}

	// Normalize editor name (handle "open -a Cursor" style commands)
	editorLower := strings.ToLower(editor)

	switch {
	case strings.Contains(editorLower, "cursor"):
		// Cursor: cursor://file/path:line:column
		return fmt.Sprintf("cursor://file%s:%d:1", absPath, line)

	case strings.Contains(editorLower, "code") || strings.Contains(editorLower, "vscode"):
		// VS Code: vscode://file/path:line:column
		return fmt.Sprintf("vscode://file%s:%d:1", absPath, line)

	case strings.Contains(editorLower, "subl") || strings.Contains(editorLower, "sublime"):
		// Sublime Text: subl://open?url=file:///path&line=N
		return fmt.Sprintf("subl://open?url=file://%s&line=%d", absPath, line)

	case strings.Contains(editorLower, "idea") ||
		strings.Contains(editorLower, "goland") ||
		strings.Contains(editorLower, "webstorm") ||
		strings.Contains(editorLower, "pycharm") ||
		strings.Contains(editorLower, "phpstorm") ||
		strings.Contains(editorLower, "rider") ||
		strings.Contains(editorLower, "rubymine") ||
		strings.Contains(editorLower, "clion"):
		// JetBrains IDEs: idea://open?file=/path&line=N
		return fmt.Sprintf("idea://open?file=%s&line=%d", absPath, line)

	case strings.Contains(editorLower, "zed"):
		// Zed: zed://file/path:line
		return fmt.Sprintf("zed://file%s:%d", absPath, line)

	case strings.Contains(editorLower, "nvim") || strings.Contains(editorLower, "vim"):
		// Neovim/Vim: Use file:// - no standard URL scheme for line numbers
		return fmt.Sprintf("file://%s", absPath)

	case strings.Contains(editorLower, "emacs"):
		// Emacs: Use emacs:// scheme if available, otherwise file://
		return fmt.Sprintf("file://%s", absPath)

	default:
		// Default: file:// URL (most terminals will open with default app)
		return fmt.Sprintf("file://%s", absPath)
	}
}

func formatLocationLinkSimpleStyled(relPath string, line int, render func(...string) string) string {
	location := fmt.Sprintf("%s:%d", relPath, line)
	if render == nil {
		render = func(strs ...string) string {
			if len(strs) == 0 {
				return ""
			}
			return strs[0]
		}
	}

	if !shouldEmitHyperlinks() {
		return render(location)
	}

	// Get vault path and config
	vaultPath := getVaultPath()
	cfg := getConfig()

	if vaultPath == "" {
		return render(location)
	}

	absPath := filepath.Join(vaultPath, relPath)
	url := buildEditorURL(cfg, absPath, line)

	return render(fmt.Sprintf("\x1b]8;;%s\x07%s\x1b]8;;\x07", url, location))
}
