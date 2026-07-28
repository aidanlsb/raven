package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/ui"
)

// Test seams for the underlying interactive picker. Every CLI selection flow
// runs through these, so tests can stub selection without a real terminal.
var (
	ravenRunPicker      = picker.Run
	ravenRunPickerMulti = picker.RunMulti
)

// Terminal-detection seams (overridable in tests).
var (
	interactiveStdinIsTerminal  = func() bool { return term.IsTerminal(os.Stdin.Fd()) }
	interactiveStdoutIsTerminal = func() bool { return term.IsTerminal(os.Stdout.Fd()) }
)

// selector is the CLI-side adapter over internal/picker. It owns picker option
// construction (titles, prompts, headers, columns, shortcuts, insert mode),
// terminal gating, preview defaults, and cancel semantics, so UX tweaks stay in
// one place while internal/picker remains a generic, presentation-only
// component.
//
// Each exported method is a named selection flow. Callers build the domain
// items (see the item helpers in interactive_picker.go, query_browse.go, and
// reference_browse.go) and hand them to the adapter; the adapter never
// interprets item meaning beyond the generic picker contract.
type selector struct{}

// cliSelector is the process-wide selection adapter used by CLI commands.
var cliSelector = selector{}

// canInteract reports whether Raven's interactive picker may be shown for the
// current invocation (human output plus TTY stdin and stdout).
func canUseRavenInteractive() bool {
	if isJSONOutput() {
		return false
	}
	return canUseInteractiveTerminal()
}

// canUseInteractiveTerminal reports whether stdin and stdout are both TTYs.
func canUseInteractiveTerminal() bool {
	return interactiveStdinIsTerminal() && interactiveStdoutIsTerminal()
}

// cleanPickerPrompt normalizes a caller prompt (which may include a trailing
// "> ") into the bare prompt label the picker expects.
func cleanPickerPrompt(prompt string) string {
	return strings.TrimSuffix(prompt, "> ")
}

// vaultFile opens a picker over indexed vault file paths and returns the
// selected vault-relative path. ok is false when the user cancels.
func (selector) vaultFile(vaultPath string, vaultCfg *config.VaultConfig, prompt, title string) (string, bool, error) {
	paths, err := indexedVaultFilePaths(vaultPath, vaultCfg)
	if err != nil {
		return "", false, err
	}
	if len(paths) == 0 {
		return "", false, fmt.Errorf("no indexed files available (run 'rvn reindex')")
	}

	items := make([]picker.Item, 0, len(paths))
	for _, relPath := range paths {
		items = append(items, fileSelectionItem(relPath))
	}

	selected, ok, err := ravenRunPicker(items, picker.Options{
		Title:             title,
		Prompt:            cleanPickerPrompt(prompt),
		StartInInsertMode: true,
	})
	if err != nil || !ok {
		return "", ok, err
	}
	return strings.TrimSpace(selected.Item.ID), true, nil
}

// referenceCandidate opens a picker over indexed objects and sections and
// returns the selected reference. ok is false when the user cancels.
func (selector) referenceCandidate(vaultPath string, vaultCfg *config.VaultConfig, prompt, title string) (string, bool, error) {
	items, err := indexedReferenceTargetItems(vaultPath, vaultCfg)
	if err != nil {
		return "", false, err
	}
	if len(items) == 0 {
		return "", false, fmt.Errorf("no indexed references available (run 'rvn reindex')")
	}

	selected, ok, err := ravenRunPicker(items, picker.Options{
		Title:             title,
		Prompt:            cleanPickerPrompt(prompt),
		Headers:           []string{"#", "reference", "kind", "location"},
		Columns:           ui.SearchLayout(),
		StartInInsertMode: true,
		Preview:           vaultFilePreview(vaultPath),
	})
	if err != nil || !ok {
		return "", ok, err
	}
	return strings.TrimSpace(selected.Item.ID), true, nil
}

// ambiguousReference opens a picker over the candidates of an ambiguous
// reference and returns the chosen target. ok is false when there are no
// candidates or the user cancels.
func (selector) ambiguousReference(reference string, matches []string, matchSources map[string]string, prompt string) (string, bool, error) {
	items := make([]picker.Item, 0, len(matches))
	for _, match := range matches {
		match = strings.TrimSpace(match)
		if match == "" {
			continue
		}
		items = append(items, ambiguousReferenceItem(match, strings.TrimSpace(matchSources[match])))
	}
	if len(items) == 0 {
		return "", false, nil
	}

	selected, ok, err := ravenRunPicker(items, picker.Options{
		Title:   fmt.Sprintf("Reference %q is ambiguous", reference),
		Prompt:  cleanPickerPrompt(prompt),
		Headers: []string{"#", "target", "matched via"},
		Columns: ui.BacklinksLayout(),
	})
	if err != nil || !ok {
		return "", ok, err
	}

	target := strings.TrimSpace(selected.Item.ID)
	if target == "" {
		return "", false, nil
	}
	return target, true, nil
}

// browsePickerOptions configures a post-result browse picker. The concrete
// items, headers, and columns are supplied by the calling command; the adapter
// owns the shared prompt, preview, and missing-file-path handling.
type browsePickerOptions struct {
	Title                  string
	Items                  []picker.Item
	Headers                []string
	Columns                []ui.ColumnDef
	MissingFilePathMessage string
}

// browse opens a browse picker over result rows and returns the selected item.
// It requires the selected item to carry a file path so callers can open it.
func (selector) browse(opts browsePickerOptions) (picker.Item, bool, error) {
	selected, ok, err := ravenRunPicker(opts.Items, picker.Options{
		Title:   opts.Title,
		Prompt:  "filter",
		Headers: opts.Headers,
		Columns: opts.Columns,
		Preview: vaultFilePreview(getVaultPath()),
	})
	if err != nil {
		return picker.Item{}, false, handleError(ErrInternal, err, "")
	}
	if !ok {
		return picker.Item{}, false, nil
	}
	if selected.Item.FilePath == "" {
		message := strings.TrimSpace(opts.MissingFilePathMessage)
		if message == "" {
			message = "selected item has no file path"
		}
		return picker.Item{}, false, handleErrorMsg(ErrInternal, message, "")
	}
	return selected.Item, true, nil
}

// browseAndOpen browses result rows and opens the selection in the editor.
func (s selector) browseAndOpen(opts browsePickerOptions) error {
	item, ok, err := s.browse(opts)
	if err != nil || !ok {
		return err
	}
	openPickerItemInEditor(item)
	return nil
}

// pipeItems opens the picker used by 'rvn pick' on the supplied controlling
// terminal and returns the user's selections. Single-select results are
// normalized into a one-element slice so callers handle both modes uniformly.
func (selector) pipeItems(items []picker.Item, multi bool, tty *os.File) ([]picker.Selection, bool, error) {
	opts := picker.Options{
		Title:   "Pick items",
		Prompt:  "filter",
		Headers: []string{"#", "content", "id", "location"},
		Columns: ui.SearchLayout(),
		Preview: vaultFilePreview(getVaultPath()),
		Input:   tty,
		Output:  tty,
	}
	if multi {
		return ravenRunPickerMulti(items, opts)
	}
	selection, ok, err := ravenRunPicker(items, opts)
	if err != nil || !ok {
		return nil, ok, err
	}
	return []picker.Selection{selection}, true, nil
}

// docsSection opens the docs section navigator. Selection may carry a forward
// action; the caller maps the returned item ID back to a section.
func (selector) docsSection(items []picker.Item) (picker.Selection, bool, error) {
	return ravenRunPicker(items, picker.Options{
		Title:        "Select a docs section",
		Prompt:       "docs/section",
		Headers:      []string{"#", "section", "title", "topics"},
		Columns:      ui.SearchLayout(),
		AllowForward: true,
		Shortcuts: []picker.ShortcutTip{
			{Key: "j/k", Description: "move"},
			{Key: "l", Description: "topics"},
			{Key: "enter", Description: "topics"},
			{Key: "/ or i", Description: "filter"},
			{Key: "q", Description: "cancel"},
		},
	})
}

// docsTopic opens the docs topic navigator for a section. Selection may carry a
// forward (open) or back (return to sections) action.
func (selector) docsTopic(items []picker.Item, title, prompt string) (picker.Selection, bool, error) {
	return ravenRunPicker(items, picker.Options{
		Title:        title,
		Prompt:       prompt,
		Headers:      []string{"#", "topic", "title", "path"},
		Columns:      ui.SearchLayout(),
		AllowForward: true,
		AllowBack:    true,
		Shortcuts: []picker.ShortcutTip{
			{Key: "j/k", Description: "move"},
			{Key: "h", Description: "sections"},
			{Key: "l", Description: "open"},
			{Key: "enter", Description: "open"},
			{Key: "/ or i", Description: "filter"},
			{Key: "q", Description: "cancel"},
		},
	})
}
