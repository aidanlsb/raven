package commandimpl

import (
	"context"
	"errors"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/traitsvc"
)

func updateMissingBulkTraitIDs(caller commandexec.Caller) (string, string) {
	if caller == commandexec.CallerMCP {
		return "no trait_ids provided for bulk update", "Provide trait_ids for the bulk update and retry"
	}
	return "no trait IDs provided via stdin", "Pipe trait IDs to stdin, one per line"
}

// HandleUpdate executes the canonical `update` command.
func HandleUpdate(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	newValue := strings.TrimSpace(stringArg(req.Args, "value"))
	if newValue == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "no value specified", nil, "Usage: rvn update <trait_id> <new_value>")
	}

	traitIDs := commandIDsArg(req.Args, "trait_ids")
	stdinMode := boolArg(req.Args, "stdin") || len(traitIDs) > 0
	confirm := req.Confirm
	if !stdinMode {
		singleID := strings.TrimSpace(stringArg(req.Args, "trait_id"))
		if singleID == "" {
			return commandexec.Failure("MISSING_ARGUMENT", "requires trait-id and new value arguments", nil, "Usage: rvn update <trait_id> <new_value>")
		}
		if !strings.Contains(singleID, ":trait:") {
			return commandexec.Failure("INVALID_INPUT", "invalid trait ID format", nil, "Trait IDs look like: path/file.md:trait:N")
		}
		traitIDs = []string{singleID}
		// Single-object updates apply immediately unless the caller requested a
		// dry run, which normalizes to req.Preview.
		confirm = !req.Preview
	}
	if len(traitIDs) == 0 {
		message, suggestion := updateMissingBulkTraitIDs(req.Caller)
		return commandexec.Failure("MISSING_ARGUMENT", message, nil, suggestion)
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return commandexec.Failure("SCHEMA_INVALID", "failed to load schema", nil, "Fix schema.yaml and try again")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		return commandexec.Failure("DATABASE_ERROR", err.Error(), nil, "Run 'rvn reindex' to rebuild the database")
	}
	defer db.Close()
	db.SetDailyDirectory(vaultCfg.GetDailyDirectory())

	traits, skipped, err := traitsvc.ResolveTraitIDs(db, traitIDs)
	if err != nil {
		return mapTraitMutationError(err)
	}
	filteredTraits := make([]model.Trait, 0, len(traits))
	for _, trait := range traits {
		if err := objectsvc.ValidateContentMutationFilePath(vaultPath, vaultCfg, trait.FilePath); err != nil {
			if !stdinMode {
				return mapContentMutationError(err)
			}
			skipped = append(skipped, traitsvc.BulkResult{
				ID:       trait.ID,
				FilePath: trait.FilePath,
				Line:     trait.Line,
				Status:   "skipped",
				Reason:   err.Error(),
			})
			continue
		}
		filteredTraits = append(filteredTraits, trait)
	}
	traits = filteredTraits

	if !confirm {
		preview, err := traitsvc.BuildPreview(traits, newValue, sch, skipped)
		if err != nil {
			return mapTraitMutationError(err)
		}
		return commandexec.Success(map[string]interface{}{
			"preview": true,
			"action":  preview.Action,
			"items":   preview.Items,
			"skipped": preview.Skipped,
			"total":   preview.Total,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := traitsvc.ApplyUpdates(vaultPath, traits, newValue, sch, skipped)
	if err != nil {
		return mapTraitMutationError(err)
	}

	warnings := autoReindexWarnings(vaultPath, vaultCfg, summary.ChangedFilePaths...)

	return commandexec.SuccessWithWarnings(map[string]interface{}{
		"action":   summary.Action,
		"results":  summary.Results,
		"total":    summary.Total,
		"modified": summary.Modified,
		"skipped":  summary.Skipped,
		"errors":   summary.Errors,
	}, warnings, &commandexec.Meta{Count: summary.Modified})
}

func mapTraitMutationError(err error) commandexec.Result {
	var validationErr *traitsvc.ValueValidationError
	if errors.As(err, &validationErr) {
		return commandexec.Failure("VALIDATION_FAILED", validationErr.Error(), nil, validationErr.Suggestion())
	}
	if svcErr, ok := traitsvc.AsError(err); ok {
		return commandexec.Failure(svcErr.Code, svcErr.Message, svcErr.Details, svcErr.Suggestion)
	}
	return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
}
