// Package cli implements the command-line interface.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/traitsvc"
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
		fmt.Println("No traits to update.")
	} else {
		fmt.Printf("\nPreview: %d trait(s) will be updated\n\n", len(preview.Items))
	}

	if len(preview.Items) > 0 {
		for _, item := range preview.Items {
			fmt.Printf("  %s:%d\n", item.FilePath, item.Line)
			fmt.Printf("    @%s: %s → %s\n", item.TraitType, item.OldValue, item.NewValue)
			if item.Content != "" {
				content := item.Content
				if len(content) > 50 {
					content = content[:47] + "..."
				}
				fmt.Printf("    content: %s\n", content)
			}
		}
	}

	if len(preview.Skipped) > 0 {
		fmt.Printf("\nSkipped %d trait(s):\n", len(preview.Skipped))
		for _, skip := range preview.Skipped {
			path := skip.FilePath
			if path == "" {
				path = skip.ID
			}
			fmt.Printf("  %s:%d - %s\n", path, skip.Line, skip.Reason)
		}
	}

	fmt.Printf("\nRun with --confirm to apply changes.\n")
}

// printTraitBulkSummary prints a human-readable summary of trait bulk operations.
func printTraitBulkSummary(summary *TraitBulkSummary) {
	fmt.Printf("✓ Updated %d trait(s)\n", summary.Modified)
	if summary.Skipped > 0 {
		fmt.Printf("  Skipped: %d\n", summary.Skipped)
	}
	if summary.Errors > 0 {
		fmt.Printf("  Errors: %d\n", summary.Errors)
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
