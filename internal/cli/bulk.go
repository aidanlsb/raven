// Package cli implements the command-line interface.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/ui"
)

// BulkOperation represents the type of bulk operation.
type BulkOperation string

const (
	BulkOpSet    BulkOperation = "set"
	BulkOpDelete BulkOperation = "delete"
	BulkOpAdd    BulkOperation = "add"
	BulkOpMove   BulkOperation = "move"
)

// BulkResult represents the result of a single bulk operation on one object.
type BulkResult struct {
	ID      string `json:"id"`
	Status  string `json:"status"` // "modified", "deleted", "added", "moved", "skipped", "error"
	Reason  string `json:"reason,omitempty"`
	Details string `json:"details,omitempty"`
}

// BulkPreviewItem represents a single item in a bulk operation preview.
type BulkPreviewItem struct {
	ID      string            `json:"id"`
	Changes map[string]string `json:"changes,omitempty"` // field -> "new_value (was: old_value)"
	Action  string            `json:"action"`            // "set", "delete", "add", "move"
	Details string            `json:"details,omitempty"` // Additional info like destination for move
}

// BulkPreview represents a preview of bulk operations before confirmation.
type BulkPreview struct {
	Action   string            `json:"action"`
	Items    []BulkPreviewItem `json:"items"`
	Skipped  []BulkResult      `json:"skipped,omitempty"`
	Total    int               `json:"total"`
	Warnings []Warning         `json:"warnings,omitempty"`
}

// BulkSummary represents the summary of a completed bulk operation.
type BulkSummary struct {
	Action   string       `json:"action"`
	Results  []BulkResult `json:"results"`
	Total    int          `json:"total"`
	Modified int          `json:"modified,omitempty"`
	Deleted  int          `json:"deleted,omitempty"`
	Added    int          `json:"added,omitempty"`
	Moved    int          `json:"moved,omitempty"`
	Skipped  int          `json:"skipped,omitempty"`
	Errors   int          `json:"errors,omitempty"`
}

// ReadIDsFromStdin reads object/trait IDs from stdin, one per line.
// Returns the IDs and any embedded IDs that were filtered out.
func ReadIDsFromStdin() (ids []string, embedded []string, err error) {
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

		// Check for embedded object IDs (contain #)
		if strings.Contains(id, "#") {
			embedded = append(embedded, id)
			continue
		}

		ids = append(ids, id)
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("error reading from stdin: %w", err)
	}

	return ids, embedded, nil
}

// extractIDFromPipeLine extracts an ID from pipe-friendly list output.
// Expected format: num<TAB>id<TAB>content<TAB>location
// Falls back to the full line if no tabs are present.
func extractIDFromPipeLine(line string) string {
	if strings.Contains(line, "\t") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) >= 2 {
			id := strings.TrimSpace(parts[1])
			if id != "" {
				return id
			}
		}
	}
	return line
}

// IsEmbeddedID checks if an ID is an embedded object ID (contains #).
func IsEmbeddedID(id string) bool {
	_, _, ok := paths.ParseEmbeddedID(id)
	return ok
}

// PrintBulkPreview prints a human-readable preview of bulk operations.
func PrintBulkPreview(preview *BulkPreview) {
	fmt.Printf("\n%s\n\n", ui.SectionHeader(fmt.Sprintf("Preview: %d objects will be %s", len(preview.Items), getActionVerb(preview.Action))))

	for _, item := range preview.Items {
		fmt.Println(ui.Bullet(item.ID))
		if item.Details != "" {
			fmt.Println(ui.Indent(2, ui.Hint("→ "+item.Details)))
		}
		for field, change := range item.Changes {
			fmt.Println(ui.Indent(2, fmt.Sprintf("%s: %s", field, change)))
		}
	}

	if len(preview.Skipped) > 0 {
		fmt.Printf("\n%s\n", ui.Starf("Skipped %d items.", len(preview.Skipped)))
		for _, skip := range preview.Skipped {
			fmt.Println(ui.Bullet(fmt.Sprintf("%s: %s", skip.ID, skip.Reason)))
		}
	}

	for _, w := range preview.Warnings {
		fmt.Printf("\n%s\n", ui.Warning(w.Message))
	}

	fmt.Printf("\n%s\n", ui.Hint("Run with --confirm to apply changes."))
}

// PrintBulkSummary prints a human-readable summary of completed bulk operations.
func PrintBulkSummary(summary *BulkSummary) {
	switch summary.Action {
	case "set":
		fmt.Println(ui.Checkf("Updated %d objects", summary.Modified))
	case "delete":
		fmt.Println(ui.Checkf("Deleted %d objects", summary.Deleted))
	case "add":
		fmt.Println(ui.Checkf("Added content to %d objects", summary.Added))
	case "move":
		fmt.Println(ui.Checkf("Moved %d objects", summary.Moved))
	}

	if summary.Skipped > 0 {
		fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Skipped: %d", summary.Skipped)))
	}
	if summary.Errors > 0 {
		fmt.Printf("  %s\n", ui.Errorf("%d errors", summary.Errors))
	}
}

// getActionVerb returns the past tense verb for an action.
func getActionVerb(action string) string {
	switch action {
	case "set":
		return "modified"
	case "delete":
		return "deleted"
	case "add":
		return "updated"
	case "move":
		return "moved"
	default:
		return "processed"
	}
}

// BuildEmbeddedSkipWarning creates a warning for skipped embedded objects.
func BuildEmbeddedSkipWarning(embedded []string) *Warning {
	if len(embedded) == 0 {
		return nil
	}
	return &Warning{
		Code:    WarnEmbeddedSkipped,
		Message: fmt.Sprintf("Skipped %d embedded object(s) - bulk operations only support file-level objects", len(embedded)),
		Ref:     strings.Join(embedded, ", "),
	}
}

// Warning codes for bulk operations
const (
	WarnObjectNotFound   = "object_not_found"
	WarnFieldNotInSchema = "field_not_in_schema"
)
