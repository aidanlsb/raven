// Package cli implements the command-line interface.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/schema"
)

// getTraitDefault returns the default value for a trait from the schema.
func getTraitDefault(sch *schema.Schema, traitType string) string {
	if traitDef, ok := sch.Traits[traitType]; ok {
		if def, ok := traitDef.Default.(string); ok {
			return def
		}
	}
	return ""
}

// TraitBulkResult represents the result of a bulk trait operation.
type TraitBulkResult struct {
	ID       string `json:"id"`
	FilePath string `json:"file_path"`
	Line     int    `json:"line"`
	Status   string `json:"status"` // "modified", "skipped", "error"
	Reason   string `json:"reason,omitempty"`
	OldValue string `json:"old_value,omitempty"`
	NewValue string `json:"new_value,omitempty"`
}

// TraitBulkPreviewItem represents a preview item for trait bulk operations.
type TraitBulkPreviewItem struct {
	ID        string `json:"id"`
	FilePath  string `json:"file_path"`
	Line      int    `json:"line"`
	TraitType string `json:"trait_type"`
	Content   string `json:"content"`
	OldValue  string `json:"old_value"`
	NewValue  string `json:"new_value"`
}

// TraitBulkPreview represents a preview of trait bulk operations.
type TraitBulkPreview struct {
	Action  string                 `json:"action"`
	Items   []TraitBulkPreviewItem `json:"items"`
	Skipped []TraitBulkResult      `json:"skipped,omitempty"`
	Total   int                    `json:"total"`
}

// TraitBulkSummary represents the summary of completed trait bulk operations.
type TraitBulkSummary struct {
	Action   string            `json:"action"`
	Results  []TraitBulkResult `json:"results"`
	Total    int               `json:"total"`
	Modified int               `json:"modified"`
	Skipped  int               `json:"skipped"`
	Errors   int               `json:"errors"`
}

// applyUpdateTraitFromQuery applies update operation to trait query results.
func applyUpdateTraitFromQuery(vaultPath string, traits []query.PipelineTraitResult, args []string, sch *schema.Schema, vaultCfg *config.VaultConfig, confirm bool) error {
	// Parse value=X argument
	var newValue string
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return handleErrorMsg(ErrInvalidInput,
				fmt.Sprintf("invalid field format: %s", arg),
				"Use format: value=<new_value>")
		}
		if parts[0] != "value" {
			return handleErrorMsg(ErrInvalidInput,
				fmt.Sprintf("unknown field for trait: %s", parts[0]),
				"For trait queries, only 'value' can be updated. Example: --apply \"update value=done\"")
		}
		newValue = parts[1]
	}

	if newValue == "" {
		return handleErrorMsg(ErrMissingArgument, "no value specified", "Usage: --apply \"update value=<new_value>\"")
	}

	if !confirm {
		return previewUpdateTraitBulk(vaultPath, traits, newValue, sch, nil)
	}
	return applyUpdateTraitBulk(vaultPath, traits, newValue, sch, vaultCfg, nil)
}

// previewUpdateTraitBulk shows a preview of trait update operations.
func previewUpdateTraitBulk(vaultPath string, traits []query.PipelineTraitResult, newValue string, sch *schema.Schema, extraSkipped []TraitBulkResult) error {
	var items []TraitBulkPreviewItem
	var skipped []TraitBulkResult

	for _, t := range traits {
		oldValue := ""
		if t.Value != nil {
			oldValue = *t.Value
		} else {
			// For bare traits, get the default value from schema
			oldValue = getTraitDefault(sch, t.TraitType)
		}

		// Skip if value is already the target
		if oldValue == newValue {
			skipped = append(skipped, TraitBulkResult{
				ID:       t.ID,
				FilePath: t.FilePath,
				Line:     t.Line,
				Status:   "skipped",
				Reason:   "already has target value",
			})
			continue
		}

		items = append(items, TraitBulkPreviewItem{
			ID:        t.ID,
			FilePath:  t.FilePath,
			Line:      t.Line,
			TraitType: t.TraitType,
			Content:   t.Content,
			OldValue:  oldValue,
			NewValue:  newValue,
		})
	}
	if len(extraSkipped) > 0 {
		skipped = append(skipped, extraSkipped...)
	}

	preview := TraitBulkPreview{
		Action:  "update-trait",
		Items:   items,
		Skipped: skipped,
		Total:   len(items),
	}

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"preview": true,
			"action":  preview.Action,
			"items":   preview.Items,
			"skipped": preview.Skipped,
			"total":   preview.Total,
		}, &Meta{Count: preview.Total})
		return nil
	}

	printTraitBulkPreview(&preview)
	return nil
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
				// Truncate content for display
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

// applyUpdateTraitBulk applies update operations to traits.
func applyUpdateTraitBulk(vaultPath string, traits []query.PipelineTraitResult, newValue string, sch *schema.Schema, vaultCfg *config.VaultConfig, extraSkipped []TraitBulkResult) error {
	// Group traits by file for efficient file I/O
	traitsByFile := make(map[string][]query.PipelineTraitResult)
	for _, t := range traits {
		traitsByFile[t.FilePath] = append(traitsByFile[t.FilePath], t)
	}

	results := make([]TraitBulkResult, 0, len(traits)+len(extraSkipped))
	modified := 0
	skipped := 0
	errors := 0

	if len(extraSkipped) > 0 {
		results = append(results, extraSkipped...)
		skipped += len(extraSkipped)
	}

	for filePath, fileTraits := range traitsByFile {
		fullPath := filePath
		if !strings.HasPrefix(filePath, vaultPath) {
			fullPath = vaultPath + "/" + filePath
		}

		// Read the file
		content, err := os.ReadFile(fullPath)
		if err != nil {
			for _, t := range fileTraits {
				results = append(results, TraitBulkResult{
					ID:       t.ID,
					FilePath: t.FilePath,
					Line:     t.Line,
					Status:   "error",
					Reason:   fmt.Sprintf("failed to read file: %v", err),
				})
				errors++
			}
			continue
		}

		lines := strings.Split(string(content), "\n")
		fileModified := false

		for _, t := range fileTraits {
			// Lines are 1-indexed
			lineIdx := t.Line - 1
			if lineIdx < 0 || lineIdx >= len(lines) {
				results = append(results, TraitBulkResult{
					ID:       t.ID,
					FilePath: t.FilePath,
					Line:     t.Line,
					Status:   "error",
					Reason:   "line number out of range",
				})
				errors++
				continue
			}

			oldValue := ""
			if t.Value != nil {
				oldValue = *t.Value
			} else {
				oldValue = getTraitDefault(sch, t.TraitType)
			}

			// Skip if already target value
			if oldValue == newValue {
				results = append(results, TraitBulkResult{
					ID:       t.ID,
					FilePath: t.FilePath,
					Line:     t.Line,
					Status:   "skipped",
					Reason:   "already has target value",
				})
				skipped++
				continue
			}

			// Rewrite the trait on this line
			oldLine := lines[lineIdx]
			newLine, ok := rewriteTraitValue(oldLine, t.TraitType, newValue)
			if !ok {
				results = append(results, TraitBulkResult{
					ID:       t.ID,
					FilePath: t.FilePath,
					Line:     t.Line,
					Status:   "error",
					Reason:   "trait not found on line",
				})
				errors++
				continue
			}

			lines[lineIdx] = newLine
			fileModified = true
			results = append(results, TraitBulkResult{
				ID:       t.ID,
				FilePath: t.FilePath,
				Line:     t.Line,
				Status:   "modified",
				OldValue: oldValue,
				NewValue: newValue,
			})
			modified++
		}

		// Write the file back if modified
		if fileModified {
			newContent := strings.Join(lines, "\n")
			if err := atomicfile.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
				// Mark all modified traits in this file as errors
				for i, r := range results {
					if r.FilePath == filePath && r.Status == "modified" {
						results[i].Status = "error"
						results[i].Reason = fmt.Sprintf("failed to write file: %v", err)
						modified--
						errors++
					}
				}
				continue
			}
			maybeReindex(vaultPath, fullPath, vaultCfg)
		}
	}

	summary := TraitBulkSummary{
		Action:   "update-trait",
		Results:  results,
		Total:    len(traits) + len(extraSkipped),
		Modified: modified,
		Skipped:  skipped,
		Errors:   errors,
	}

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"action":   summary.Action,
			"results":  summary.Results,
			"total":    summary.Total,
			"modified": summary.Modified,
			"skipped":  summary.Skipped,
			"errors":   summary.Errors,
		}, &Meta{Count: summary.Modified})
		return nil
	}

	printTraitBulkSummary(&summary)
	return nil
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

// rewriteTraitValue rewrites a trait's value on a line.
// It handles both bare traits (@todo) and valued traits (@todo(doing)).
// Returns the modified line and true if successful, or the original line and false if the trait wasn't found.
func rewriteTraitValue(line, traitType, newValue string) (string, bool) {
	// Pattern to match the specific trait, with or without value
	// Matches: @traitType or @traitType(value) or @traitType (value)
	pattern := regexp.MustCompile(`@` + regexp.QuoteMeta(traitType) + `(?:\s*\([^)]*\))?`)

	if !pattern.MatchString(line) {
		return line, false
	}

	// Replace with new value
	newTrait := fmt.Sprintf("@%s(%s)", traitType, newValue)
	newLine := pattern.ReplaceAllString(line, newTrait)

	return newLine, true
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

		// Trait IDs contain ":trait:" in them
		if !strings.Contains(id, ":trait:") {
			continue // Skip non-trait IDs
		}
		ids = append(ids, id)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading from stdin: %w", err)
	}

	return ids, nil
}

// applyUpdateTraitsByID updates traits identified by IDs, with preview/confirm behavior.
func applyUpdateTraitsByID(vaultPath string, traitIDs []string, newValue string, confirm bool, vaultCfg *config.VaultConfig) error {
	// Load schema for defaults (optional)
	sch, err := schema.Load(vaultPath)
	if err != nil {
		sch = nil
	}

	db, err := openDatabaseWithConfig(vaultPath, vaultCfg)
	if err != nil {
		return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
	}
	defer db.Close()

	traits, skipped, err := resolveTraitIDs(db, traitIDs)
	if err != nil {
		return handleError(ErrDatabaseError, err, "")
	}

	if !confirm {
		return previewUpdateTraitBulk(vaultPath, traits, newValue, sch, skipped)
	}
	return applyUpdateTraitBulk(vaultPath, traits, newValue, sch, vaultCfg, skipped)
}

// resolveTraitIDs resolves trait IDs to concrete trait results using the index.
func resolveTraitIDs(db *index.Database, ids []string) ([]query.PipelineTraitResult, []TraitBulkResult, error) {
	results := make([]query.PipelineTraitResult, 0, len(ids))
	var skipped []TraitBulkResult

	for _, id := range ids {
		if !strings.Contains(id, ":trait:") {
			skipped = append(skipped, TraitBulkResult{
				ID:     id,
				Status: "skipped",
				Reason: "invalid trait ID format",
			})
			continue
		}

		trait, err := db.GetTrait(id)
		if err != nil {
			return nil, nil, err
		}
		if trait == nil {
			filePath := strings.SplitN(id, ":trait:", 2)[0]
			skipped = append(skipped, TraitBulkResult{
				ID:       id,
				FilePath: filePath,
				Status:   "skipped",
				Reason:   "trait not found in index",
			})
			continue
		}

		results = append(results, query.PipelineTraitResult{
			Trait:    *trait,
			Computed: make(map[string]interface{}),
		})
	}

	return results, skipped, nil
}
