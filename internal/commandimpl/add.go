package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
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
	request := objectsvc.AddBulkRequest{
		ObjectIDs: ids,
		Line:      text,
		Preview:   !confirm,
	}

	if request.Preview {
		preview, _, err := objectsvc.AddBulk(sess.Runtime(), request)
		if err != nil {
			return mapContentMutationError(err).WithAttemptedIDs("object_ids", ids)
		}
		return commandexec.Success(preview.Payload(text, nil), &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, proj, failure := mutateResult(sess, func(rt *vaultruntime.Runtime) (*objectsvc.AddBulkSummary, mutation.ChangeSet, error) {
		_, summary, err := objectsvc.AddBulk(rt, request)
		if err != nil {
			return nil, mutation.ChangeSet{}, err
		}
		return summary, summary.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure.WithAttemptedIDs("object_ids", ids)
	}

	return commandexec.SuccessWithWarnings(
		summary.Payload(text, missingRefsOf(proj)),
		proj.Warnings,
		&commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors},
	)
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
	result.MissingReferences = missingRefsOf(proj)
	return commandexec.SuccessWithWarnings(*result, proj.Warnings, nil)
}

func removedAddHeadingFailure() commandexec.Result {
	return commandexec.Failure(
		"INVALID_INPUT",
		"rvn add only appends body content; it does not accept or create headings",
		nil,
		`Create the heading with 'rvn section create <file> "<title>" --level N', then append content with 'rvn add <text> --to <file#section>'`,
	)
}
