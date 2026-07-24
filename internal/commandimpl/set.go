package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
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

func setMissingBulkObjectIDs(caller commandexec.Caller) (string, string) {
	if caller == commandexec.CallerMCP {
		return "no object_ids provided for bulk set", "Provide object_ids for the bulk update and retry"
	}
	return "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line"
}

func setMissingFields(caller commandexec.Caller, bulk bool) string {
	if caller == commandexec.CallerMCP {
		if bulk {
			return "Provide fields or fields-json in args"
		}
		return "Provide object_id plus fields or fields-json in args"
	}
	if bulk {
		return "Usage: rvn set --stdin field=value... or --fields-json '{...}'"
	}
	return "Usage: rvn set <object-id> field=value... or --fields-json '{...}'"
}

func setMissingObjectID(caller commandexec.Caller) (string, string) {
	if caller == commandexec.CallerMCP {
		return "requires object_id", "Provide object_id and retry"
	}
	return "requires object-id", "Usage: rvn set <object-id> field=value..."
}

// HandleSet executes the canonical `set` command.
func HandleSet(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	rt, failure := newRequiredCommandVaultRuntime(vaultPath, false)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg
	sch := rt.Schema

	updates, err := parseKeyValueArgs(req.Args["fields"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", "invalid fields payload", nil, err.Error())
	}

	typedUpdates, err := parseTypedFieldValues(req.Args["fields-json"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", "invalid fields-json payload", nil, setFieldsJSONHint(req.Caller))
	}
	allUpdates := mergeFieldInputs(updates, typedUpdates)

	objectIDs := commandIDsArg(req.Args, "object_ids")
	stdinMode := boolArg(req.Args, "stdin") || len(objectIDs) > 0
	if stdinMode {
		if len(objectIDs) == 0 {
			message, suggestion := setMissingBulkObjectIDs(req.Caller)
			return commandexec.Failure("MISSING_ARGUMENT", message, nil, suggestion)
		}
		if len(allUpdates) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no fields to set", nil, setMissingFields(req.Caller, true))
		}
		return runSetBulk(rt, objectIDs, allUpdates, req.Confirm)
	}

	reference := strings.TrimSpace(stringArg(req.Args, "object_id"))
	if reference == "" {
		message, suggestion := setMissingObjectID(req.Caller)
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

	data := map[string]interface{}{
		"file":           serviceResult.RelativePath,
		"object_id":      serviceResult.ObjectID,
		"type":           serviceResult.ObjectType,
		"updated_fields": serviceResult.ResolvedUpdates,
	}
	if len(serviceResult.PreviousFields) > 0 {
		data["previous_fields"] = serviceResult.PreviousFields
	}

	if req.Preview {
		data["preview"] = true
		return commandexec.SuccessWithWarnings(
			data,
			warningMessagesToCommandWarnings(serviceResult.WarningMessages, codes.WarnUnknownField),
			nil,
		)
	}

	warnings := appendCommandWarnings(
		warningMessagesToCommandWarnings(serviceResult.WarningMessages, codes.WarnUnknownField),
	)

	postData, postWarnings := applyChangeSet(rt, serviceResult.ChangeSet)
	data = mergeDataFields(data, postData)
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
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	rt, failure := newRequiredCommandVaultRuntime(vaultPath, false)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg
	sch := rt.Schema

	reference := strings.TrimSpace(stringArg(req.Args, "object_id"))
	if reference == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires object-id", nil, "Usage: rvn unset <object-id> <field>...")
	}

	fields := stringSliceArg(req.Args["fields"])
	if len(fields) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no fields to unset", nil, "Usage: rvn unset <object-id> <field>...")
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

	postData, warnings := applyChangeSet(rt, serviceResult.ChangeSet)

	data := map[string]interface{}{
		"file":            serviceResult.RelativePath,
		"object_id":       serviceResult.ObjectID,
		"type":            serviceResult.ObjectType,
		"removed_fields":  fieldmutation.SerializeFieldValueMap(serviceResult.RemovedFields),
		"missing_fields":  serviceResult.MissingFields,
		"modified":        serviceResult.Modified,
		"previous_fields": serviceResult.PreviousFields,
	}
	data = mergeDataFields(data, postData)
	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

func runSetBulk(rt *vaultruntime.Runtime, ids []string, updates map[string]fieldvalue.FieldValue, confirm bool) commandexec.Result {
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
			return mapContentMutationError(err)
		}
		return commandexec.Success(map[string]interface{}{
			"preview":  true,
			"action":   preview.Action,
			"items":    canonicalSetPreviewItems(preview.Items),
			"skipped":  canonicalSetResults(preview.Skipped),
			"total":    preview.Total,
			"warnings": warnings,
			"fields":   serializedUpdates,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := objectsvc.ApplySetBulk(request)
	if err != nil {
		return mapContentMutationError(err)
	}

	data := map[string]interface{}{
		"ok":       summary.Errors == 0,
		"action":   summary.Action,
		"items":    canonicalSetResults(summary.Results),
		"total":    summary.Total,
		"skipped":  summary.Skipped,
		"errors":   summary.Errors,
		"modified": summary.Modified,
		"fields":   serializedUpdates,
	}
	postData, postWarnings := applyChangeSet(rt, summary.ChangeSet)
	data = mergeDataFields(data, postData)
	warnings = appendCommandWarnings(warnings, postWarnings)

	return commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}
