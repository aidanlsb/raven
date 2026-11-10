package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
	"github.com/aidanlsb/raven/internal/vaultsession"
)

func setFieldsJSONHint(caller commandexec.Caller) string {
	if caller.IsMachine() {
		return `Provide a JSON object under fields-json, for example {"status":"active"}`
	}
	return `Provide a JSON object, e.g. --fields-json '{"status":"active"}'`
}

func setMissingBulkReferences(caller commandexec.Caller) (string, string) {
	if caller.IsMachine() {
		return "no references provided for bulk set", "Provide references for the bulk update and retry"
	}
	return "no references provided via stdin", "Pipe references to stdin, one per line"
}

func setMissingFields(caller commandexec.Caller, bulk bool) string {
	if caller.IsMachine() {
		if bulk {
			return "Provide fields or fields-json in args"
		}
		return "Provide reference plus fields or fields-json in args"
	}
	if bulk {
		return "Usage: rvn set --stdin field=value... or --fields-json '{...}'"
	}
	return "Usage: rvn set <reference> field=value... or --fields-json '{...}'"
}

func setMissingReference(caller commandexec.Caller) (string, string) {
	if caller.IsMachine() {
		return "requires reference", "Provide reference and retry"
	}
	return "requires reference", "Usage: rvn set <reference> field=value..."
}

// HandleSet executes the canonical `set` command.
func HandleSet(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	references := commandIDsArg(req.Args, "references")
	stdinMode := boolArg(req.Args, "stdin") || len(references) > 0

	sess, failure := openSession(vaultPath, vaultsession.Required())
	if failure.Error != nil {
		if stdinMode {
			return failure.WithAttemptedIDs("references", references)
		}
		return failure
	}
	defer sess.Close()

	// Accept both "field" (new canonical) and "fields" (legacy MCP) for backward compat
	fieldArg := req.Args["field"]
	if fieldArg == nil {
		fieldArg = req.Args["fields"]
	}
	updates, err := parseKeyValueArgs(fieldArg)
	if err != nil {
		failure := commandexec.Failure("INVALID_INPUT", "invalid field payload", nil, err.Error())
		if stdinMode {
			return failure.WithAttemptedIDs("references", references)
		}
		return failure
	}

	typedUpdates, err := parseTypedFieldValues(req.Args["fields-json"])
	if err != nil {
		failure := commandexec.Failure("INVALID_INPUT", "invalid fields-json payload", nil, setFieldsJSONHint(req.Caller))
		if stdinMode {
			return failure.WithAttemptedIDs("references", references)
		}
		return failure
	}
	allUpdates := mergeFieldInputs(updates, typedUpdates)

	if stdinMode {
		if len(references) == 0 {
			message, suggestion := setMissingBulkReferences(req.Caller)
			return commandexec.Failure("MISSING_ARGUMENT", message, nil, suggestion)
		}
		if len(allUpdates) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no fields to set", nil, setMissingFields(req.Caller, true)).
				WithAttemptedIDs("references", references)
		}
		return runSetBulk(sess, references, allUpdates, req.Confirm)
	}

	reference := strings.TrimSpace(stringArg(req.Args, "reference"))
	if reference == "" {
		message, suggestion := setMissingReference(req.Caller)
		return commandexec.Failure("MISSING_ARGUMENT", message, nil, suggestion)
	}
	if len(allUpdates) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no fields to set", nil, setMissingFields(req.Caller, false))
	}

	setReq := objectsvc.SetByReferenceRequest{
		Reference:    reference,
		TypedUpdates: allUpdates,
		Preview:      req.Preview,
	}
	if req.Preview {
		serviceResult, err := objectsvc.SetByReference(sess.Runtime(), setReq)
		if err != nil {
			return mapContentMutationError(err)
		}
		data := commandpayload.SetResult{
			File:          serviceResult.RelativePath,
			ObjectID:      serviceResult.ObjectID,
			Type:          serviceResult.ObjectType,
			UpdatedFields: serviceResult.ResolvedUpdates,
			Preview:       true,
		}
		if len(serviceResult.PreviousFields) > 0 {
			data.PreviousFields = serviceResult.PreviousFields
		}
		return commandexec.SuccessWithWarnings(
			data,
			warningMessagesToCommandWarnings(serviceResult.WarningMessages, codes.WarnUnknownField),
			nil,
		)
	}

	setReq.Preview = false
	serviceResult, proj, failure := mutateResult(sess, func(rt *vaultruntime.Runtime) (*objectsvc.SetByReferenceResult, mutation.ChangeSet, error) {
		result, err := objectsvc.SetByReference(rt, setReq)
		if err != nil {
			return nil, mutation.ChangeSet{}, err
		}
		return result, result.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure
	}

	data := commandpayload.SetResult{
		File:              serviceResult.RelativePath,
		ObjectID:          serviceResult.ObjectID,
		Type:              serviceResult.ObjectType,
		UpdatedFields:     serviceResult.ResolvedUpdates,
		MissingReferences: missingRefsOf(proj),
	}
	if len(serviceResult.PreviousFields) > 0 {
		data.PreviousFields = serviceResult.PreviousFields
	}
	warnings := appendCommandWarnings(
		warningMessagesToCommandWarnings(serviceResult.WarningMessages, codes.WarnUnknownField),
		proj.Warnings,
	)

	return commandexec.SuccessWithWarnings(
		data,
		warnings,
		nil,
	)
}

// HandleUnset executes the canonical `unset` command.
func HandleUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	sess, failure := openSession(vaultPath, vaultsession.Required())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()

	reference := strings.TrimSpace(stringArg(req.Args, "reference"))
	if reference == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires reference", nil, "Usage: rvn unset <reference> <field>...")
	}

	fields := stringSliceArg(req.Args["fields"])
	if len(fields) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no fields to unset", nil, "Usage: rvn unset <reference> <field>...")
	}

	serviceResult, proj, failure := mutateResult(sess, func(rt *vaultruntime.Runtime) (*objectsvc.UnsetByReferenceResult, mutation.ChangeSet, error) {
		result, err := objectsvc.UnsetByReference(rt, objectsvc.UnsetByReferenceRequest{
			Reference: reference,
			Fields:    fields,
		})
		if err != nil {
			return nil, mutation.ChangeSet{}, err
		}
		return result, result.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure
	}

	data := commandpayload.UnsetResult{
		File:              serviceResult.RelativePath,
		ObjectID:          serviceResult.ObjectID,
		Type:              serviceResult.ObjectType,
		RemovedFields:     fieldmutation.SerializeFieldValueMap(serviceResult.RemovedFields),
		MissingFields:     serviceResult.MissingFields,
		Modified:          serviceResult.Modified,
		PreviousFields:    serviceResult.PreviousFields,
		MissingReferences: missingRefsOf(proj),
	}
	return commandexec.SuccessWithWarnings(data, proj.Warnings, nil)
}

func runSetBulk(sess *vaultsession.Session, ids []string, updates map[string]fieldvalue.FieldValue, confirm bool) commandexec.Result {
	fileIDs, sectionIDs := splitSectionIDs(ids)
	warnings := sectionSkipWarnings(sectionIDs)
	request := objectsvc.SetBulkRequest{
		ObjectIDs:    fileIDs,
		TypedUpdates: updates,
	}
	serializedUpdates := fieldmutation.SerializeFieldValueMap(updates)

	if !confirm {
		preview, err := objectsvc.PreviewSetBulk(sess.Runtime(), request)
		if err != nil {
			return mapContentMutationError(err).WithAttemptedIDs("references", ids)
		}
		return commandexec.Success(commandpayload.SetBulkPreviewResult{
			BulkPreviewResult: commandpayload.BulkPreviewResult{
				Preview: true,
				Action:  preview.Action,
				Items:   canonicalBulkPreviewItems(preview.Items),
				Skipped: canonicalBulkResults(preview.Skipped),
				Total:   preview.Total,
			},
			Warnings: warnings,
			Fields:   serializedUpdates,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, proj, failure := mutateResult(sess, func(rt *vaultruntime.Runtime) (*objectsvc.SetBulkSummary, mutation.ChangeSet, error) {
		summary, err := objectsvc.ApplySetBulk(rt, request)
		if err != nil {
			return nil, mutation.ChangeSet{}, err
		}
		return summary, summary.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure.WithAttemptedIDs("references", ids)
	}
	data := commandpayload.SetBulkResult{
		BulkSummaryResult: commandpayload.BulkSummaryResult{
			OK:                summary.Errors == 0,
			Action:            summary.Action,
			Items:             canonicalBulkResults(summary.Results),
			Total:             summary.Total,
			Skipped:           summary.Skipped,
			Errors:            summary.Errors,
			MissingReferences: missingRefsOf(proj),
		},
		Modified: summary.Modified,
		Fields:   serializedUpdates,
	}
	warnings = appendCommandWarnings(warnings, proj.Warnings)

	return commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}
