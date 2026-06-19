package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/ui"
)

func validateReferenceBrowseFlag(cmd *cobra.Command) (bool, error) {
	browse, _ := cmd.Flags().GetBool("browse")
	return validateInteractiveBrowse(browse)
}

func validateInteractiveBrowse(enabled bool) (bool, error) {
	if !enabled {
		return false, nil
	}
	if isJSONOutput() {
		err := handleErrorMsg(ErrInvalidInput, "--browse cannot be used with --json", "Remove --browse or --json")
		return err == nil, err
	}
	if !canUseInteractiveTerminal() {
		err := handleErrorMsg(ErrInvalidInput, "interactive browse requires an interactive terminal", "Run without --browse in non-interactive contexts")
		return err == nil, err
	}
	return false, nil
}

type browsePickerOptions struct {
	Title                  string
	Items                  []picker.Item
	Headers                []string
	Columns                []ui.ColumnDef
	MissingFilePathMessage string
}

func browseItemsForReferenceResults(links []model.Reference, displayText func(model.Reference) string) []picker.Item {
	items := make([]picker.Item, 0, len(links))
	for i, link := range links {
		line := referenceLine(link)
		location := referenceLocation(link.FilePath, line)
		content := displayText(link)
		items = append(items, picker.Item{
			ID:       referenceBrowseSelectionKey(i, link, line),
			Label:    content,
			Detail:   link.TargetRaw,
			Location: location,
			FilePath: link.FilePath,
			Line:     line,
			Columns:  []string{content, location},
			SearchText: browseSearchText(
				link.SourceID,
				link.SourceType,
				link.TargetRaw,
				stringPtrValue(link.DisplayText),
				link.FilePath,
				location,
			),
		})
	}
	return items
}

func browseReferences(title string, items []picker.Item) (picker.Item, bool, error) {
	return runBrowsePicker(browsePickerOptions{
		Title:                  title,
		Items:                  items,
		Headers:                []string{"#", "content", "location"},
		Columns:                ui.BacklinksLayout(),
		MissingFilePathMessage: "selected reference has no file path",
	})
}

func browseAndOpenReferences(title string, items []picker.Item) error {
	return browseAndOpenPickerSelection(browsePickerOptions{
		Title:                  title,
		Items:                  items,
		Headers:                []string{"#", "content", "location"},
		Columns:                ui.BacklinksLayout(),
		MissingFilePathMessage: "selected reference has no file path",
	})
}

func browseAndOpenPickerSelection(opts browsePickerOptions) error {
	item, ok, err := runBrowsePicker(opts)
	if err != nil || !ok {
		return err
	}
	openPickerItemInEditor(item)
	return nil
}

func runBrowsePicker(opts browsePickerOptions) (picker.Item, bool, error) {
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

func openPickerItemInEditor(item picker.Item) {
	openFileInEditorAtLine(filepath.Join(getVaultPath(), item.FilePath), item.FilePath, item.Line, false)
}

func referenceBrowseSelectionKey(index int, link model.Reference, line int) string {
	parts := []string{
		link.SourceID,
		link.TargetRaw,
		link.FilePath,
		fmt.Sprintf("%d", line),
		fmt.Sprintf("%d", index+1),
	}
	return strings.Join(parts, ":")
}

func referenceLocation(filePath string, line int) string {
	if line > 0 {
		return fmt.Sprintf("%s:%d", filePath, line)
	}
	return filePath
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
