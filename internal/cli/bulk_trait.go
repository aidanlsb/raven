// Package cli implements the command-line interface.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/traitsvc"
)

type TraitBulkResult = traitsvc.BulkResult

type TraitBulkPreviewItem = traitsvc.BulkPreviewItem

type TraitBulkPreview = traitsvc.BulkPreview

type TraitBulkSummary = traitsvc.BulkSummary

type traitValueValidationError = traitsvc.ValueValidationError

func parseTraitUpdateValueArgs(args []string, usageHint string) (string, error) {
	value := strings.TrimSpace(strings.Join(args, " "))
	if value == "" {
		return "", handleErrorMsg(ErrMissingArgument, "no value specified", usageHint)
	}
	return value, nil
}

func ensureTraitSchema(vaultPath string, sch *schema.Schema) (*schema.Schema, error) {
	if sch != nil {
		return sch, nil
	}
	loaded, err := schema.Load(vaultPath)
	if err != nil {
		return nil, handleError(ErrSchemaInvalid, err, "Fix schema.yaml and try again")
	}
	return loaded, nil
}

// applyUpdateTraitFromQuery applies update operation to trait query results.
func applyUpdateTraitFromQuery(vaultPath string, traits []model.Trait, args []string, sch *schema.Schema, vaultCfg *config.VaultConfig, confirm bool) error {
	newValue, err := parseTraitUpdateValueArgs(args, "Usage: --apply \"update <new_value>\"")
	if err != nil {
		return err
	}

	sch, err = ensureTraitSchema(vaultPath, sch)
	if err != nil {
		return err
	}

	if !confirm {
		if err := previewUpdateTraitBulk(vaultPath, traits, newValue, sch, nil); err != nil {
			return err
		}
		if promptForConfirm("Apply changes?") {
			return applyUpdateTraitBulk(vaultPath, traits, newValue, sch, vaultCfg, nil)
		}
		return nil
	}
	return applyUpdateTraitBulk(vaultPath, traits, newValue, sch, vaultCfg, nil)
}

// previewUpdateTraitBulk shows a preview of trait update operations.
func previewUpdateTraitBulk(vaultPath string, traits []model.Trait, newValue string, sch *schema.Schema, extraSkipped []TraitBulkResult) error {
	preview, err := traitsvc.BuildPreview(traits, newValue, sch, extraSkipped)
	if err != nil {
		return err
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

	printTraitBulkPreview(preview)
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
func applyUpdateTraitBulk(vaultPath string, traits []model.Trait, newValue string, sch *schema.Schema, vaultCfg *config.VaultConfig, extraSkipped []TraitBulkResult) error {
	summary, err := traitsvc.ApplyUpdates(vaultPath, traits, newValue, sch, extraSkipped)
	if err != nil {
		return err
	}

	reindexed := make(map[string]struct{}, len(summary.ChangedFilePaths))
	for _, filePath := range summary.ChangedFilePaths {
		if filePath == "" {
			continue
		}
		if _, seen := reindexed[filePath]; seen {
			continue
		}
		reindexed[filePath] = struct{}{}
		maybeReindex(vaultPath, filePath, vaultCfg)
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

	printTraitBulkSummary(summary)
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

// applyUpdateTraitsByID updates traits identified by IDs, with preview/confirm behavior.
func applyUpdateTraitsByID(vaultPath string, traitIDs []string, newValue string, confirm bool, prompt bool, vaultCfg *config.VaultConfig) error {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaInvalid, err, "Fix schema.yaml and try again")
	}

	db, err := openDatabaseWithConfig(vaultPath, vaultCfg)
	if err != nil {
		return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
	}
	defer db.Close()

	traits, skipped, err := resolveTraitIDs(db, traitIDs)
	if err != nil {
		return err
	}

	if !confirm {
		if err := previewUpdateTraitBulk(vaultPath, traits, newValue, sch, skipped); err != nil {
			return err
		}
		if prompt && promptForConfirm("Apply changes?") {
			return applyUpdateTraitBulk(vaultPath, traits, newValue, sch, vaultCfg, skipped)
		}
		return nil
	}
	return applyUpdateTraitBulk(vaultPath, traits, newValue, sch, vaultCfg, skipped)
}

// resolveTraitIDs resolves trait IDs to concrete trait results using the index.
func resolveTraitIDs(db *index.Database, ids []string) ([]model.Trait, []TraitBulkResult, error) {
	traits, skipped, err := traitsvc.ResolveTraitIDs(db, ids)
	if err != nil {
		if svcErr, ok := traitsvc.AsError(err); ok {
			return nil, nil, handleErrorMsg(ErrDatabaseError, svcErr.Message, svcErr.Suggestion)
		}
		return nil, nil, handleError(ErrDatabaseError, err, "")
	}
	return traits, skipped, nil
}
