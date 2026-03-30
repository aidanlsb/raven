// Package cli implements the command-line interface.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/traitsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

type TraitBulkResult = traitsvc.BulkResult

type TraitBulkPreviewItem = traitsvc.BulkPreviewItem

type TraitBulkPreview = traitsvc.BulkPreview

type TraitBulkSummary = traitsvc.BulkSummary

func parseTraitUpdateValueArgs(args []string, usageHint string) (string, error) {
	value := strings.TrimSpace(strings.Join(args, " "))
	if value == "" {
		return "", handleErrorMsg(ErrMissingArgument, "no value specified", usageHint)
	}
	return value, nil
}

// printTraitBulkPreview prints a human-readable preview of trait bulk operations.
func printTraitBulkPreview(preview *TraitBulkPreview) {
	if len(preview.Items) == 0 {
		fmt.Println(ui.Star("No traits to update."))
	} else {
		fmt.Printf("\n%s\n\n", ui.SectionHeader(fmt.Sprintf("Preview: %d trait(s) will be updated", len(preview.Items))))
	}

	if len(preview.Items) > 0 {
		for _, item := range preview.Items {
			fmt.Println(ui.Bullet(fmt.Sprintf("%s:%d", ui.FilePath(item.FilePath), item.Line)))
			fmt.Println(ui.Indent(2, fmt.Sprintf("@%s: %s → %s", item.TraitType, item.OldValue, item.NewValue)))
			if item.Content != "" {
				content := item.Content
				if len(content) > 50 {
					content = content[:47] + "..."
				}
				fmt.Println(ui.Indent(2, ui.Hint("content: "+content)))
			}
		}
	}

	if len(preview.Skipped) > 0 {
		fmt.Printf("\n%s\n", ui.Starf("Skipped %d trait(s).", len(preview.Skipped)))
		for _, skip := range preview.Skipped {
			path := skip.FilePath
			if path == "" {
				path = skip.ID
			}
			fmt.Println(ui.Bullet(fmt.Sprintf("%s:%d - %s", path, skip.Line, skip.Reason)))
		}
	}

	fmt.Printf("\n%s\n", ui.Hint("Run with --confirm to apply changes."))
}

// printTraitBulkSummary prints a human-readable summary of trait bulk operations.
func printTraitBulkSummary(summary *TraitBulkSummary) {
	fmt.Println(ui.Checkf("Updated %d trait(s)", summary.Modified))
	if summary.Skipped > 0 {
		fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Skipped: %d", summary.Skipped)))
	}
	if summary.Errors > 0 {
		fmt.Printf("  %s\n", ui.Errorf("%d errors", summary.Errors))
	}
}

// ReadTraitIDsFromStdin reads trait IDs from stdin for bulk operations.
func ReadTraitIDsFromStdin() (ids []string, err error) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		id := extractIDFromPipeLine(line)
		if id == "" {
			continue
		}

		if !strings.Contains(id, ":trait:") {
			continue
		}
		ids = append(ids, id)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading from stdin: %w", err)
	}

	return ids, nil
}
