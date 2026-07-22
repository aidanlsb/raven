package commandimpl

import (
	"context"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/parseopts"
	"github.com/aidanlsb/raven/internal/schema"
)

// HandleMove executes the canonical `move` command.
func HandleMove(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	// Move resolves references and rewrites backlinks using the schema, so
	// require a valid schema on this safety-sensitive path.
	rt, failure := newRequiredCommandVaultRuntime(vaultPath, false)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg
	sch := rt.Schema

	objectIDs := commandIDsArg(req.Args, "object_ids")
	stdinMode := boolArg(req.Args, "stdin") || len(objectIDs) > 0
	if stdinMode {
		destination := strings.TrimSpace(stringArg(req.Args, "destination"))
		if destination == "" {
			destination = strings.TrimSpace(stringArg(req.Args, "source"))
		}
		if len(objectIDs) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no object IDs provided via stdin", nil, "Provide object IDs when using bulk move")
		}
		return runMoveBulk(vaultPath, vaultCfg, sch, objectIDs, destination, boolArgDefault(req.Args, "update-refs", true), req.Confirm)
	}

	source := strings.TrimSpace(stringArg(req.Args, "source"))
	destination := strings.TrimSpace(stringArg(req.Args, "destination"))
	if source == "" || destination == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires source and destination arguments", nil, "Usage: rvn move <source> <destination>")
	}

	serviceResult, err := objectsvc.MoveByReference(objectsvc.MoveByReferenceRequest{
		VaultPath:      vaultPath,
		VaultConfig:    vaultCfg,
		Schema:         sch,
		Reference:      source,
		Destination:    destination,
		UpdateRefs:     boolArgDefault(req.Args, "update-refs", true),
		SkipTypeCheck:  boolArg(req.Args, "skip-type-check"),
		Preview:        req.Preview,
		ParseOptions:   parseopts.FromVaultConfig(vaultCfg),
		FailOnIndexErr: true,
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	if serviceResult.NeedsConfirm && serviceResult.TypeMismatch != nil {
		mismatch := serviceResult.TypeMismatch
		// Nothing was written: the move is blocked pending confirmation, so
		// report a preview phase regardless of the normalized apply resolution.
		return commandexec.SuccessWithWarnings(map[string]interface{}{
			"source":        serviceResult.SourceID,
			"destination":   serviceResult.DestinationID,
			"preview":       req.Preview,
			"needs_confirm": true,
			"reason":        serviceResult.Reason,
		}, []commandexec.Warning{{
			Code: codes.WarnTypeMismatch,
			Message: fmt.Sprintf("Moving to '%s/' which is the default directory for type '%s', but file has type '%s'",
				mismatch.DestinationDir, mismatch.ExpectedType, mismatch.ActualType),
			Ref: fmt.Sprintf("Use --skip-type-check to proceed, or change the file's type to '%s'", mismatch.ExpectedType),
		}}, nil).WithMutationPhase(commandexec.MutationPhasePreview)
	}

	warnings := warningMessagesToCommandWarnings(serviceResult.WarningMessages, indexUpdateFailedWarningCode)
	data := map[string]interface{}{
		"source":      serviceResult.SourceID,
		"destination": serviceResult.DestinationID,
	}
	if req.Preview {
		data["preview"] = true
		data["status"] = "preview"
	}
	if len(serviceResult.UpdatedRefs) > 0 {
		data["updated_refs"] = serviceResult.UpdatedRefs
	}

	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

func runMoveBulk(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, ids []string, destination string, updateRefs bool, confirm bool) commandexec.Result {
	if strings.TrimSpace(destination) == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "no destination provided", nil, "Usage: rvn move --stdin <destination-directory/>")
	}
	if !strings.HasSuffix(destination, "/") {
		return commandexec.Failure("INVALID_INPUT", "destination must be a directory (end with /)", nil, "Example: rvn move --stdin archive/projects/")
	}

	fileIDs, sectionIDs := splitSectionIDs(ids)
	if len(sectionIDs) > 0 {
		return moveSectionSourceFailure(sectionIDs)
	}
	request := objectsvc.MoveBulkRequest{
		VaultPath:      vaultPath,
		VaultConfig:    vaultCfg,
		Schema:         sch,
		ObjectIDs:      fileIDs,
		DestinationDir: destination,
		UpdateRefs:     updateRefs,
		ParseOptions:   parseopts.FromVaultConfig(vaultCfg),
	}

	if !confirm {
		preview, err := objectsvc.PreviewMoveBulk(request)
		if err != nil {
			return mapContentMutationError(err)
		}
		return commandexec.Success(map[string]interface{}{
			"preview":     true,
			"action":      preview.Action,
			"items":       canonicalMovePreviewItems(preview.Items),
			"skipped":     canonicalMoveResults(preview.Skipped),
			"total":       preview.Total,
			"destination": preview.Destination,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := objectsvc.ApplyMoveBulk(request)
	if err != nil {
		return mapContentMutationError(err)
	}

	allWarnings := warningMessagesToCommandWarnings(summary.WarningMessages, indexUpdateFailedWarningCode)
	return commandexec.SuccessWithWarnings(map[string]interface{}{
		"ok":          summary.Errors == 0,
		"action":      summary.Action,
		"items":       canonicalMoveResults(summary.Results),
		"total":       summary.Total,
		"skipped":     summary.Skipped,
		"errors":      summary.Errors,
		"moved":       summary.Moved,
		"destination": summary.Destination,
	}, allWarnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}

func moveSectionSourceFailure(sectionIDs []string) commandexec.Result {
	return commandexec.Failure(
		codes.ErrInvalidInput,
		"rvn move does not accept section sources",
		map[string]interface{}{"section_ids": sectionIDs},
		`Use 'rvn section move <file#section>' to reorder/reparent, or 'rvn section rename <file#section> "<new heading text>"' to change heading identity`,
	)
}
