package vault

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/shellquote"
)

// OpenInEditor opens a file in the user's configured editor.
// Returns true if the editor was launched, false otherwise.
// GUI editors are started in the background (non-blocking).
// Terminal editors run in the foreground to keep TTY attached.
//
// Note: If your vault path contains spaces (e.g., iCloud paths like
// "Mobile Documents"), some editors may have issues. Use a symlink
// to a path without spaces as a workaround.
func OpenInEditor(cfg *config.Config, filePath string) bool {
	return OpenInEditorAtLine(cfg, filePath, 0)
}

// OpenInEditorAtLine opens a file in the user's configured editor, optionally
// positioning the cursor at a 1-indexed line when the editor supports it.
func OpenInEditorAtLine(cfg *config.Config, filePath string, line int) bool {
	if cfg == nil {
		return false
	}

	editor := cfg.GetEditor()
	if editor == "" {
		return false
	}

	var cmd *exec.Cmd
	args := editorOpenArgs(editor, filePath, line)

	// If editor contains spaces, it's a compound command like "open -a Cursor"
	// Execute via shell to handle this correctly
	if strings.Contains(editor, " ") {
		cmd = exec.Command("sh", "-c", editor+" "+quoteShellArgs(args))
	} else {
		cmd = exec.Command(editor, args...)
	}

	if shouldRunEditorInTerminal(cfg, editor) {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Warning: failed to open editor '%s': %v\n", editor, err)
			return false
		}
		return true
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("Warning: failed to open editor '%s': %v\n", editor, err)
		return false
	}
	return true
}

func editorOpenArgs(editor, filePath string, line int) []string {
	if line <= 0 {
		return []string{filePath}
	}

	lineTarget := fmt.Sprintf("%s:%d", filePath, line)
	switch editorCommandName(editor) {
	case "code", "code-insiders", "codium", "cursor":
		return []string{"-g", lineTarget}
	case "vi", "vim", "vimdiff", "nvim", "nvimdiff", "nano", "micro":
		return []string{fmt.Sprintf("+%d", line), filePath}
	case "hx", "helix", "kak", "kakoune":
		return []string{lineTarget}
	default:
		return []string{filePath}
	}
}

func quoteShellArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellquote.Quote(arg)
	}
	return strings.Join(quoted, " ")
}

// OpenFilesInEditor opens multiple files in the user's configured editor.
// Returns true if the editor was launched, false otherwise.
// GUI editors are started in the background (non-blocking).
// Terminal editors run in the foreground to keep TTY attached.
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
			quotedPaths[i] = shellquote.Quote(p)
		}
		cmd = exec.Command("sh", "-c", editor+" "+strings.Join(quotedPaths, " "))
	} else {
		cmd = exec.Command(editor, filePaths...)
	}

	if shouldRunEditorInTerminal(cfg, editor) {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Warning: failed to open editor '%s': %v\n", editor, err)
			return false
		}
		return true
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

type editorMode int

const (
	editorModeAuto editorMode = iota
	editorModeTerminal
	editorModeGUI
)

func shouldRunEditorInTerminal(cfg *config.Config, editor string) bool {
	switch parseEditorMode(cfg) {
	case editorModeTerminal:
		return true
	case editorModeGUI:
		return false
	default:
		return isTerminalEditor(editor)
	}
}

func parseEditorMode(cfg *config.Config) editorMode {
	if cfg == nil {
		return editorModeAuto
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.EditorMode))
	switch mode {
	case "", "auto":
		return editorModeAuto
	case "terminal", "tty", "tui", "cli":
		return editorModeTerminal
	case "gui", "background", "nonblocking", "non-blocking":
		return editorModeGUI
	default:
		return editorModeAuto
	}
}

func isTerminalEditor(editor string) bool {
	name := editorCommandName(editor)
	switch name {
	case "vi", "vim", "vimdiff",
		"nvim", "nvimdiff",
		"hx", "helix",
		"nano",
		"emacs", "emacsclient",
		"micro",
		"kak", "kakoune":
		return true
	default:
		return false
	}
}

func editorCommandName(editor string) string {
	trimmed := strings.TrimSpace(editor)
	if trimmed == "" {
		return ""
	}

	var cmd string
	switch trimmed[0] {
	case '"', '\'':
		quote := trimmed[0]
		if end := strings.IndexByte(trimmed[1:], quote); end >= 0 {
			cmd = trimmed[1 : 1+end]
		} else {
			cmd = trimmed[1:]
		}
	default:
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			return ""
		}
		cmd = fields[0]
	}

	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}

	base := filepath.Base(cmd)
	base = strings.TrimSuffix(base, ".exe")
	return strings.ToLower(base)
}
