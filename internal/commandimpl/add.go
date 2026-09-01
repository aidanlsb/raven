package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// HandleAdd executes the canonical `add` command.
func HandleAdd(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	objectIDs := commandIDsArg(req.Args, "object_ids")
	stdinMode := boolArg(req.Args, "stdin") || len(objectIDs) > 0

	text := strings.TrimSpace(stringArg(req.Args, "text"))
	if text == "" {
		failure := commandexec.Failure("MISSING_ARGUMENT", "requires text argument", nil, "Usage: rvn add <text>")
		if stdinMode {
			return failure.WithAttemptedIDs("object_ids", objectIDs)
		}
		return failure
	}
	if err := objectsvc.ValidateAddContent(text); err != nil {
		failure := removedAddHeadingFailure()
		if stdinMode {
			return failure.WithAttemptedIDs("object_ids", objectIDs)
		}
		return failure
	}
	if _, hasHeading := req.Args["heading"]; hasHeading {
		failure := removedAddHeadingFailure()
		if stdinMode {
			return failure.WithAttemptedIDs("object_ids", objectIDs)
		}
		return failure
	}
	if _, hasCreateHeading := req.Args["create-heading"]; hasCreateHeading {
		failure := removedAddHeadingFailure()
		if stdinMode {
			return failure.WithAttemptedIDs("object_ids", objectIDs)
		}
		return failure
	}

	rt, failure := newRequiredCommandVaultRuntime(vaultPath, false)
	if failure.Error != nil {
		if stdinMode {
			return failure.WithAttemptedIDs("object_ids", objectIDs)
		}
		return failure
	}
	defer rt.Close()

	if !stdinMode {
		return runAddSingle(rt, text, strings.TrimSpace(stringArg(req.Args, "to")), req.IndexJournalOperation)
	}
	if len(objectIDs) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no object IDs provided via stdin", nil, "Pipe object IDs to stdin, one per line")
	}

	return runAddBulk(rt, objectIDs, text, req.Confirm, req.IndexJournalOperation)
}

func runAddBulk(rt *vaultruntime.Runtime, ids []string, text string, confirm bool, journalOperation string) commandexec.Result {
	vaultPath := rt.VaultPath
	vaultCfg := rt.VaultCfg
	// Section IDs (file#slug) are passed through: bulk add appends within the
	// targeted section instead of at the end of the file.
	var warnings []commandexec.Warning
	request := objectsvc.AddBulkRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		ObjectIDs:    ids,
		Line:         text,
		ParseOptions: rt.ParseOptions,
		Runtime:      rt,
	}

	if !confirm {
		preview, err := objectsvc.PreviewAddBulk(request)
		if err != nil {
			return mapContentMutationError(err).WithAttemptedIDs("object_ids", ids)
		}
		return commandexec.Success(commandpayload.AddBulkPreviewResult{
			BulkPreviewResult: commandpayload.BulkPreviewResult{
				Preview: true,
				Action:  "add",
				Items:   canonicalBulkPreviewItems(preview.Items),
				Skipped: canonicalBulkResults(preview.Skipped),
				Total:   preview.Total,
			},
			Warnings: warnings,
			Content:  text,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := objectsvc.ApplyAddBulk(request)
	if err != nil {
		return mapContentMutationError(err).WithAttemptedIDs("object_ids", ids)
	}

	missingRefs, postWarnings := applyChangeSet(rt, summary.ChangeSet, journalOperation)
	data := commandpayload.AddBulkResult{
		BulkSummaryResult: commandpayload.BulkSummaryResult{
			OK:                summary.Errors == 0,
			Action:            summary.Action,
			Items:             canonicalBulkResults(summary.Results),
			Total:             summary.Total,
			Skipped:           summary.Skipped,
			Errors:            summary.Errors,
			MissingReferences: missingRefs,
		},
		Added:   summary.Added,
		Content: text,
	}
	warnings = appendCommandWarnings(warnings, postWarnings)
	return commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}

func runAddSingle(rt *vaultruntime.Runtime, text, toRef, journalOperation string) commandexec.Result {
	result, err := objectsvc.Add(rt, objectsvc.AddRequest{Text: text, ToReference: toRef})
	if err != nil {
		return mapContentMutationError(err)
	}
	missingRefs, postWarnings := applyChangeSet(rt, result.ChangeSet, journalOperation)
	data := commandpayload.AddResult{
		File:              result.File,
		Line:              result.Line,
		Content:           text,
		MissingReferences: missingRefs,
	}
	return commandexec.SuccessWithWarnings(data, postWarnings, nil)
}

func removedAddHeadingFailure() commandexec.Result {
	return commandexec.Failure(
		"INVALID_INPUT",
		"rvn add only appends body content; it does not accept or create headings",
		nil,
		`Create the heading with 'rvn section create <file> "<title>" --level N', then append content with 'rvn add <text> --to <file#section>'`,
	)
}
