package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/ui"
)

func validateReferenceBrowseFlag(cmd *cobra.Command) (bool, error) {
	browse, _ := cmd.Flags().GetBool("browse")
	if !browse {
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

func browseItemsForReferenceResults(links []model.Reference, displayText func(model.Reference) string) []picker.Item {
	items := make([]picker.Item, 0, len(links))
	for i, link := range links {
		line := referenceLine(link)
		location := referenceLocation(link.FilePath, line)
		content := displayText(link)
		items = append(items, picker.Item{
			ID:       referenceBrowseID(i, link, line),
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
	selected, ok, err := ravenRunPicker(items, picker.Options{
		Title:   title,
		Prompt:  "filter",
		Headers: []string{"#", "content", "location"},
		Columns: ui.BacklinksLayout(),
		Preview: vaultFilePreview(getVaultPath()),
	})
	if err != nil {
		return picker.Item{}, false, handleError(ErrInternal, err, "")
	}
	if !ok {
		return picker.Item{}, false, nil
	}
	if selected.Item.FilePath == "" {
		return picker.Item{}, false, handleErrorMsg(ErrInternal, "selected reference has no file path", "")
	}
	return selected.Item, true, nil
}

func referenceBrowseID(index int, link model.Reference, line int) string {
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
