package commandimpl

import (
	"context"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/schema"
)

// HandleMove executes the canonical `move` command.
func HandleMove(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		sch = schema.New()
	}

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
		ParseOptions:   buildParseOptions(vaultCfg),
		FailOnIndexErr: true,
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	if serviceResult.NeedsConfirm && serviceResult.TypeMismatch != nil {
		mismatch := serviceResult.TypeMismatch
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
		}}, nil)
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
	warnings := sectionSkipWarnings(sectionIDs)
	request := objectsvc.MoveBulkRequest{
		VaultPath:      vaultPath,
		VaultConfig:    vaultCfg,
		Schema:         sch,
		ObjectIDs:      fileIDs,
		DestinationDir: destination,
		UpdateRefs:     updateRefs,
		ParseOptions:   buildParseOptions(vaultCfg),
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
			"warnings":    warnings,
			"destination": preview.Destination,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := objectsvc.ApplyMoveBulk(request)
	if err != nil {
		return mapContentMutationError(err)
	}

	allWarnings := append([]commandexec.Warning{}, warnings...)
	allWarnings = append(allWarnings, warningMessagesToCommandWarnings(summary.WarningMessages, indexUpdateFailedWarningCode)...)
	return commandexec.SuccessWithWarnings(map[string]interface{}{
		"ok":          summary.Errors == 0,
		"action":      summary.Action,
		"results":     canonicalMoveResults(summary.Results),
		"total":       summary.Total,
		"skipped":     summary.Skipped,
		"errors":      summary.Errors,
		"moved":       summary.Moved,
		"destination": summary.Destination,
	}, allWarnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}
