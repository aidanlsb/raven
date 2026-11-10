package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
	"github.com/aidanlsb/raven/internal/vaultsession"
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

	sess, failure := openSession(vaultPath, vaultsession.Required())
	if failure.Error != nil {
		if stdinMode {
			return failure.WithAttemptedIDs("object_ids", objectIDs)
		}
		return failure
	}
	defer sess.Close()

	if !stdinMode {
		return runAddSingle(sess, text, strings.TrimSpace(stringArg(req.Args, "to")))
	}
	if len(objectIDs) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no object IDs provided via stdin", nil, "Pipe object IDs to stdin, one per line")
	}

	return runAddBulk(sess, objectIDs, text, req.Confirm)
}

func runAddBulk(sess *vaultsession.Session, ids []string, text string, confirm bool) commandexec.Result {
	// Section IDs (file#slug) are passed through: bulk add appends within the
	// targeted section instead of at the end of the file.
	var warnings []commandexec.Warning
	request := objectsvc.AddBulkRequest{
		ObjectIDs: ids,
		Line:      text,
	}

	if !confirm {
		preview, err := objectsvc.PreviewAddBulk(sess.Runtime(), request)
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

	summary, proj, failure := mutateResult(sess, func(rt *vaultruntime.Runtime) (*objectsvc.AddBulkSummary, mutation.ChangeSet, error) {
		summary, err := objectsvc.ApplyAddBulk(rt, request)
		if err != nil {
			return nil, mutation.ChangeSet{}, err
		}
		return summary, summary.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure.WithAttemptedIDs("object_ids", ids)
	}

	data := commandpayload.AddBulkResult{
		BulkSummaryResult: commandpayload.BulkSummaryResult{
			OK:                summary.Errors == 0,
			Action:            summary.Action,
			Items:             canonicalBulkResults(summary.Results),
			Total:             summary.Total,
			Skipped:           summary.Skipped,
			Errors:            summary.Errors,
			MissingReferences: missingRefsOf(proj),
		},
		Added:   summary.Added,
		Content: text,
	}
	warnings = appendCommandWarnings(warnings, proj.Warnings)
	return commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}

func runAddSingle(sess *vaultsession.Session, text, toRef string) commandexec.Result {
	result, proj, failure := mutateResult(sess, func(rt *vaultruntime.Runtime) (*objectsvc.AddResult, mutation.ChangeSet, error) {
		result, err := objectsvc.Add(rt, objectsvc.AddRequest{Text: text, ToReference: toRef})
		if err != nil {
			return nil, mutation.ChangeSet{}, err
		}
		return result, result.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure
	}
	data := commandpayload.AddResult{
		File:              result.File,
		Line:              result.Line,
		Content:           text,
		MissingReferences: missingRefsOf(proj),
	}
	return commandexec.SuccessWithWarnings(data, proj.Warnings, nil)
}

func removedAddHeadingFailure() commandexec.Result {
	return commandexec.Failure(
		"INVALID_INPUT",
		"rvn add only appends body content; it does not accept or create headings",
		nil,
		`Create the heading with 'rvn section create <file> "<title>" --level N', then append content with 'rvn add <text> --to <file#section>'`,
	)
}
