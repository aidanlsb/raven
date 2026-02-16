package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mattn/go-isatty"

	"github.com/aidanlsb/raven/internal/config"
)

var (
	fzfLookPath         = exec.LookPath
	fzfStdinIsTerminal  = func() bool { return isatty.IsTerminal(os.Stdin.Fd()) }
	fzfStdoutIsTerminal = func() bool { return isatty.IsTerminal(os.Stdout.Fd()) }
)

type fzfPickerOptions struct {
	Prompt    string
	Header    string
	Delimiter string
	WithNth   string
}

func hasFZFInstalled() bool {
	_, err := fzfLookPath("fzf")
	return err == nil
}

func canUseFZFInteractive() bool {
	if isJSONOutput() {
		return false
	}
	if !fzfStdinIsTerminal() || !fzfStdoutIsTerminal() {
		return false
	}
	return hasFZFInstalled()
}

func runFZFPicker(lines []string, opts fzfPickerOptions) (string, bool, error) {
	if len(lines) == 0 {
		return "", false, nil
	}

	args := []string{
		"--layout=reverse",
		"--height=80%",
		"--border",
		"--select-1",
		"--exit-0",
	}
	if strings.TrimSpace(opts.Prompt) != "" {
		args = append(args, "--prompt", opts.Prompt)
	}
	if strings.TrimSpace(opts.Header) != "" {
		args = append(args, "--header", opts.Header)
	}
	if strings.TrimSpace(opts.Delimiter) != "" {
		args = append(args, "--delimiter", opts.Delimiter)
	}
	if strings.TrimSpace(opts.WithNth) != "" {
		args = append(args, "--with-nth", opts.WithNth)
	}

	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if code := exitErr.ExitCode(); code == 1 || code == 130 {
				return "", false, nil
			}
		}
		return "", false, fmt.Errorf("run fzf selector: %w", err)
	}

	selection := strings.TrimSpace(stdout.String())
	if selection == "" {
		return "", false, nil
	}
	return selection, true, nil
}

func pickVaultFileWithFZF(vaultPath string, vaultCfg *config.VaultConfig, prompt, header string) (string, bool, error) {
	paths, err := indexedVaultFilePaths(vaultPath, vaultCfg)
	if err != nil {
		return "", false, err
	}
	if len(paths) == 0 {
		return "", false, fmt.Errorf("no indexed files available (run 'rvn reindex')")
	}

	selectedLine, selected, err := runFZFPicker(paths, fzfPickerOptions{
		Prompt: prompt,
		Header: header,
	})
	if err != nil || !selected {
		return "", selected, err
	}
	return strings.TrimSpace(selectedLine), true, nil
}

func indexedVaultFilePaths(vaultPath string, vaultCfg *config.VaultConfig) ([]string, error) {
	db, err := openDatabaseWithConfig(vaultPath, vaultCfg)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	paths, err := db.AllIndexedFilePaths()
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	out := make([]string, 0, len(paths))
	for _, relPath := range paths {
		if _, err := os.Stat(filepath.Join(vaultPath, relPath)); err == nil {
			out = append(out, relPath)
		}
	}
	return out, nil
}

func interactivePickerMissingArgSuggestion(commandName, usage string) string {
	if hasFZFInstalled() {
		return fmt.Sprintf("Run '%s'", usage)
	}
	return fmt.Sprintf("Install fzf to enable interactive selection for bare 'rvn %s', or run '%s'", commandName, usage)
}
