package commandimpl

import (
	"context"
	"errors"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
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

	traitIDs := commandIDsArg(req.Args, "trait_ids")
	stdinMode := boolArg(req.Args, "stdin") || len(traitIDs) > 0
	newValue := strings.TrimSpace(stringArg(req.Args, "value"))
	if newValue == "" {
		failure := commandexec.Failure("MISSING_ARGUMENT", "no value specified", nil, "Usage: rvn update <trait_id> <new_value>")
		if stdinMode {
			return failure.WithAttemptedIDs("trait_ids", traitIDs)
		}
		return failure
	}

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

	rt, failure := newRequiredCommandVaultRuntime(vaultPath, true)
	if failure.Error != nil {
		if stdinMode {
			return failure.WithAttemptedIDs("trait_ids", traitIDs)
		}
		return failure
	}
	defer rt.Close()

	result, err := traitsvc.Update(rt, traitIDs, newValue, confirm, stdinMode)
	if err != nil {
		failure := mapTraitMutationError(err)
		if stdinMode {
			return failure.WithAttemptedIDs("trait_ids", traitIDs)
		}
		return failure
	}

	if !confirm {
		preview := result.Preview
		return commandexec.Success(commandpayload.TraitUpdatePreviewResult{
			Preview: true,
			Action:  preview.Action,
			Items:   preview.Items,
			Skipped: preview.Skipped,
			Total:   preview.Total,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary := result.Summary

	missingRefs, postWarnings := applyChangeSet(rt, summary.ChangeSet, req.IndexJournalOperation)
	data := commandpayload.TraitUpdateResult{
		Action:            summary.Action,
		Items:             summary.Results,
		Total:             summary.Total,
		Modified:          summary.Modified,
		Skipped:           summary.Skipped,
		Errors:            summary.Errors,
		MissingReferences: missingRefs,
	}
	return commandexec.SuccessWithWarnings(data, postWarnings, &commandexec.Meta{Count: summary.Modified})
}

func mapTraitMutationError(err error) commandexec.Result {
	var validationErr *traitsvc.ValueValidationError
	if errors.As(err, &validationErr) {
		return commandexec.Failure("VALIDATION_FAILED", validationErr.Error(), nil, validationErr.Suggestion())
	}
	return commandexec.FromServiceError(err)
}
