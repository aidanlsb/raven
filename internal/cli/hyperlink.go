package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/ui"
)

// hyperlinkEnabled caches whether we should emit hyperlinks.
// Hyperlinks are only emitted to TTY terminals, not JSON output or pipes.
var hyperlinkEnabled *bool

// shouldEmitHyperlinks returns true if we should emit OSC 8 hyperlinks.
func shouldEmitHyperlinks() bool {
	if hyperlinkEnabled != nil {
		return *hyperlinkEnabled
	}

	// Don't emit hyperlinks for JSON output or non-TTY
	enabled := !jsonOutput && isatty.IsTerminal(os.Stdout.Fd())
	hyperlinkEnabled = &enabled
	return enabled
}

// formatLocationLink formats a file:line location as a clickable hyperlink.
// If hyperlinks are not supported, returns the plain location string.
// The output is styled with the accent color.
//
// The link URL is determined by the configured editor:
//   - cursor, code (VS Code): cursor://file/path:line or vscode://file/path:line
//   - subl (Sublime Text): subl://open?url=file:///path&line=N
//   - idea, goland, etc: idea://open?file=/path&line=N
//   - Others: file:///path (line number not supported)
func formatLocationLink(cfg *config.Config, vaultPath, relPath string, line int) string {
	location := fmt.Sprintf("%s:%d", relPath, line)

	if !shouldEmitHyperlinks() {
		return ui.Accent.Render(location)
	}

	// Build absolute path for URL
	absPath := filepath.Join(vaultPath, relPath)

	// Get the URL based on editor
	url := buildEditorURL(cfg, absPath, line)

	// OSC 8 hyperlink format: \x1b]8;;URL\x1b\\TEXT\x1b]8;;\x1b\\
	// Using \x07 (BEL) as terminator for broader compatibility
	// Wrap with accent color styling
	return ui.Accent.Render(fmt.Sprintf("\x1b]8;;%s\x07%s\x1b]8;;\x07", url, location))
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

// formatLocationLinkSimple formats a location using only the relative path (no vault context).
// This is useful when vault path is not easily available.
// The output is styled with the accent color.
func formatLocationLinkSimple(relPath string, line int) string {
	location := fmt.Sprintf("%s:%d", relPath, line)

	if !shouldEmitHyperlinks() {
		return ui.Accent.Render(location)
	}

	// Get vault path and config
	vaultPath := getVaultPath()
	cfg := getConfig()

	if vaultPath == "" {
		return ui.Accent.Render(location)
	}

	absPath := filepath.Join(vaultPath, relPath)
	url := buildEditorURL(cfg, absPath, line)

	// Wrap with accent color styling
	return ui.Accent.Render(fmt.Sprintf("\x1b]8;;%s\x07%s\x1b]8;;\x07", url, location))
}
