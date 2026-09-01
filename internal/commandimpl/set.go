package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func setFieldsJSONHint(caller commandexec.Caller) string {
	if caller == commandexec.CallerMCP {
		return `Provide a JSON object under fields-json, for example {"status":"active"}`
	}
	return `Provide a JSON object, e.g. --fields-json '{"status":"active"}'`
}

func setMissingBulkReferences(caller commandexec.Caller) (string, string) {
	if caller == commandexec.CallerMCP {
		return "no references provided for bulk set", "Provide references for the bulk update and retry"
	}
	return "no references provided via stdin", "Pipe references to stdin, one per line"
}

func setMissingFields(caller commandexec.Caller, bulk bool) string {
	if caller == commandexec.CallerMCP {
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
	if caller == commandexec.CallerMCP {
		return "requires reference", "Provide reference and retry"
	}
	return "requires reference", "Usage: rvn set <reference> field=value..."
}

// HandleSet executes the canonical `set` command.
func HandleSet(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	references := commandIDsArg(req.Args, "references")
	stdinMode := boolArg(req.Args, "stdin") || len(references) > 0

	rt, failure := newRequiredCommandVaultRuntime(vaultPath, false)
	if failure.Error != nil {
		if stdinMode {
			return failure.WithAttemptedIDs("references", references)
		}
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg
	sch := rt.Schema

	updates, err := parseKeyValueArgs(req.Args["field"])
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
		return runSetBulk(rt, references, allUpdates, req.Confirm, req.IndexJournalOperation)
	}

	reference := strings.TrimSpace(stringArg(req.Args, "reference"))
	if reference == "" {
		message, suggestion := setMissingReference(req.Caller)
		return commandexec.Failure("MISSING_ARGUMENT", message, nil, suggestion)
	}
	if len(allUpdates) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no fields to set", nil, setMissingFields(req.Caller, false))
	}

	serviceResult, err := objectsvc.SetByReference(objectsvc.SetByReferenceRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		Schema:       sch,
		Reference:    reference,
		TypedUpdates: allUpdates,
		ParseOptions: rt.ParseOptions,
		Preview:      req.Preview,
		Runtime:      rt,
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	data := commandpayload.SetResult{
		File:          serviceResult.RelativePath,
		ObjectID:      serviceResult.ObjectID,
		Type:          serviceResult.ObjectType,
		UpdatedFields: serviceResult.ResolvedUpdates,
	}
	if len(serviceResult.PreviousFields) > 0 {
		data.PreviousFields = serviceResult.PreviousFields
	}

	if req.Preview {
		data.Preview = true
		return commandexec.SuccessWithWarnings(
			data,
			warningMessagesToCommandWarnings(serviceResult.WarningMessages, codes.WarnUnknownField),
			nil,
		)
	}

	warnings := appendCommandWarnings(
		warningMessagesToCommandWarnings(serviceResult.WarningMessages, codes.WarnUnknownField),
	)

	missingRefs, postWarnings := applyChangeSet(rt, serviceResult.ChangeSet, req.IndexJournalOperation)
	data.MissingReferences = missingRefs
	warnings = appendCommandWarnings(warnings, postWarnings)

	return commandexec.SuccessWithWarnings(
		data,
		warnings,
		nil,
	)
}

// HandleUnset executes the canonical `unset` command.
func HandleUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	rt, failure := newRequiredCommandVaultRuntime(vaultPath, false)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg
	sch := rt.Schema

	reference := strings.TrimSpace(stringArg(req.Args, "reference"))
	if reference == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires reference", nil, "Usage: rvn unset <reference> <field>...")
	}

	fields := stringSliceArg(req.Args["fields"])
	if len(fields) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no fields to unset", nil, "Usage: rvn unset <reference> <field>...")
	}

	serviceResult, err := objectsvc.UnsetByReference(objectsvc.UnsetByReferenceRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		Schema:       sch,
		Reference:    reference,
		Fields:       fields,
		ParseOptions: rt.ParseOptions,
		Runtime:      rt,
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	missingRefs, warnings := applyChangeSet(rt, serviceResult.ChangeSet, req.IndexJournalOperation)

	data := commandpayload.UnsetResult{
		File:              serviceResult.RelativePath,
		ObjectID:          serviceResult.ObjectID,
		Type:              serviceResult.ObjectType,
		RemovedFields:     fieldmutation.SerializeFieldValueMap(serviceResult.RemovedFields),
		MissingFields:     serviceResult.MissingFields,
		Modified:          serviceResult.Modified,
		PreviousFields:    serviceResult.PreviousFields,
		MissingReferences: missingRefs,
	}
	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

func runSetBulk(rt *vaultruntime.Runtime, ids []string, updates map[string]fieldvalue.FieldValue, confirm bool, journalOperation string) commandexec.Result {
	vaultPath := rt.VaultPath
	vaultCfg := rt.VaultCfg
	sch := rt.Schema
	fileIDs, sectionIDs := splitSectionIDs(ids)
	warnings := sectionSkipWarnings(sectionIDs)
	request := objectsvc.SetBulkRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		Schema:       sch,
		ObjectIDs:    fileIDs,
		TypedUpdates: updates,
		ParseOptions: rt.ParseOptions,
		Runtime:      rt,
	}
	serializedUpdates := fieldmutation.SerializeFieldValueMap(updates)

	if !confirm {
		preview, err := objectsvc.PreviewSetBulk(request)
		if err != nil {
			return mapContentMutationError(err).WithAttemptedIDs("references", ids)
		}
		return commandexec.Success(commandpayload.SetBulkPreviewResult{
			BulkPreviewResult: commandpayload.BulkPreviewResult{
				Preview: true,
				Action:  preview.Action,
				Items:   canonicalSetPreviewItems(preview.Items),
				Skipped: canonicalSetResults(preview.Skipped),
				Total:   preview.Total,
			},
			Warnings: warnings,
			Fields:   serializedUpdates,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := objectsvc.ApplySetBulk(request)
	if err != nil {
		return mapContentMutationError(err).WithAttemptedIDs("references", ids)
	}

	missingRefs, postWarnings := applyChangeSet(rt, summary.ChangeSet, journalOperation)
	data := commandpayload.SetBulkResult{
		BulkSummaryResult: commandpayload.BulkSummaryResult{
			OK:                summary.Errors == 0,
			Action:            summary.Action,
			Items:             canonicalSetResults(summary.Results),
			Total:             summary.Total,
			Skipped:           summary.Skipped,
			Errors:            summary.Errors,
			MissingReferences: missingRefs,
		},
		Modified: summary.Modified,
		Fields:   serializedUpdates,
	}
	warnings = appendCommandWarnings(warnings, postWarnings)

	return commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}
