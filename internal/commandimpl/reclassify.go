package commandimpl

import (
	"context"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// HandleReclassify executes the canonical `reclassify` command.
func HandleReclassify(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	vaultPath := strings.TrimSpace(req.VaultPath)

	references := commandIDsArg(req.Args, "references")
	stdinMode := boolArg(req.Args, "stdin") || len(references) > 0

	rt, failure := newConfigCommandVaultRuntime(vaultPath)
	if failure.Error != nil {
		if stdinMode {
			return failure.WithAttemptedIDs("references", references)
		}
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg
	if rt.SchemaLoadErr != nil {
		failure := commandexec.Failure("SCHEMA_NOT_FOUND", "failed to load schema", nil, "Run 'rvn init' to create a schema")
		if stdinMode {
			return failure.WithAttemptedIDs("references", references)
		}
		return failure
	}
	sch := rt.Schema

	fieldValues, err := parseKeyValueArgs(req.Args["field"])
	if err != nil {
		failure := commandexec.Failure("INVALID_INPUT", err.Error(), nil, "Use --field key=value")
		if stdinMode {
			return failure.WithAttemptedIDs("references", references)
		}
		return failure
	}
	typedFieldValues, err := parseTypedFieldValues(req.Args["fields-json"])
	if err != nil {
		failure := commandexec.Failure("INVALID_INPUT", "invalid --fields-json payload", nil, "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
		if stdinMode {
			return failure.WithAttemptedIDs("references", references)
		}
		return failure
	}
	allFieldValues := mergeFieldInputs(fieldValues, typedFieldValues)
	newTypeName := strings.TrimSpace(stringArg(req.Args, "new-type"))

	if stdinMode {
		if len(references) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no references provided via stdin", nil, "Pipe references to stdin, one per line")
		}
		if newTypeName == "" {
			return commandexec.Failure("MISSING_ARGUMENT", "no target type provided", nil, "Usage: rvn reclassify <new-type> --stdin").
				WithAttemptedIDs("references", references)
		}
		return runReclassifyBulk(rt, references, newTypeName, allFieldValues, req)
	}

	result, err := objectsvc.ReclassifyByReference(objectsvc.ReclassifyByReferenceRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		Schema:       sch,
		Reference:    strings.TrimSpace(stringArg(req.Args, "reference")),
		NewTypeName:  newTypeName,
		FieldValues:  allFieldValues,
		NoMove:       boolArg(req.Args, "no-move"),
		UpdateRefs:   boolArgDefault(req.Args, "update-refs", true),
		Force:        boolArg(req.Args, "force"),
		ParseOptions: rt.ParseOptions,
		Runtime:      rt,
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	warnings := make([]commandexec.Warning, 0, len(result.WarningMessages))
	for _, warning := range result.WarningMessages {
		warnings = append(warnings, commandexec.Warning{
			Code:    indexUpdateFailedWarningCode,
			Message: warning,
		})
	}
	missingRefs, postWarnings := applyChangeSet(rt, result.ChangeSet, req.IndexJournalOperation)
	warnings = appendCommandWarnings(warnings, postWarnings)
	data := commandpayload.ReclassifyResult{
		ObjectID:          result.ObjectID,
		OldType:           result.OldType,
		NewType:           result.NewType,
		File:              result.File,
		Moved:             result.Moved,
		OldPath:           result.OldPath,
		NewPath:           result.NewPath,
		UpdatedRefs:       result.UpdatedRefs,
		AddedFields:       result.AddedFields,
		DroppedFields:     result.DroppedFields,
		NeedsConfirm:      result.NeedsConfirm,
		Reason:            result.Reason,
		MissingReferences: missingRefs,
	}

	res := commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	if result.NeedsConfirm {
		// Field-dropping reclassify is blocked pending --force; nothing was
		// written, so report a preview phase.
		res = res.WithMutationPhase(commandexec.MutationPhasePreview)
	}
	return res
}

func runReclassifyBulk(
	rt *vaultruntime.Runtime,
	ids []string,
	newTypeName string,
	fieldValues map[string]fieldvalue.FieldValue,
	req commandexec.Request,
) commandexec.Result {
	request := objectsvc.ReclassifyBulkRequest{
		VaultPath:    rt.VaultPath,
		VaultConfig:  rt.VaultCfg,
		Schema:       rt.Schema,
		ObjectIDs:    ids,
		NewTypeName:  newTypeName,
		FieldValues:  fieldValues,
		NoMove:       boolArg(req.Args, "no-move"),
		UpdateRefs:   boolArgDefault(req.Args, "update-refs", true),
		Force:        boolArg(req.Args, "force"),
		ParseOptions: rt.ParseOptions,
		Runtime:      rt,
	}

	if !req.Confirm {
		preview, err := objectsvc.PreviewReclassifyBulk(request)
		if err != nil {
			return mapContentMutationError(err).WithAttemptedIDs("references", ids)
		}
		warnings := warningMessagesToCommandWarnings(preview.WarningMessages, indexUpdateFailedWarningCode)
		return commandexec.SuccessWithWarnings(commandpayload.ReclassifyBulkPreviewResult{
			Preview: true,
			Action:  preview.Action,
			NewType: preview.NewType,
			Items:   preview.Items,
			Skipped: preview.Skipped,
			Total:   preview.Total,
		}, warnings, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := objectsvc.ApplyReclassifyBulk(request, nil)
	if err != nil {
		return mapContentMutationError(err).WithAttemptedIDs("references", ids)
	}

	missingRefs, postWarnings := applyChangeSet(rt, summary.ChangeSet, req.IndexJournalOperation)
	data := commandpayload.ReclassifyBulkResult{
		OK:                summary.Errors == 0,
		Action:            summary.Action,
		NewType:           summary.NewType,
		Items:             summary.Results,
		Total:             summary.Total,
		Skipped:           summary.Skipped,
		Errors:            summary.Errors,
		Reclassified:      summary.Reclassified,
		MissingReferences: missingRefs,
	}
	warnings := warningMessagesToCommandWarnings(summary.WarningMessages, indexUpdateFailedWarningCode)
	warnings = appendCommandWarnings(warnings, postWarnings)

	result := commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{Count: summary.Reclassified})
	if summary.Reclassified == 0 {
		return result.WithMutationPhase(commandexec.MutationPhasePreview)
	}
	return result
}
