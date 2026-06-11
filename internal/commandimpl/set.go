package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/schema"
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

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return commandexec.Failure("SCHEMA_INVALID", "failed to load schema", nil, "Fix schema.yaml and try again")
	}

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
		return runSetBulk(vaultPath, vaultCfg, sch, objectIDs, allUpdates, req.Confirm)
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
		ParseOptions: buildParseOptions(vaultCfg),
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	warnings := appendCommandWarnings(
		warningMessagesToCommandWarnings(serviceResult.WarningMessages, codes.WarnUnknownField),
		autoReindexWarnings(vaultPath, vaultCfg, serviceResult.FilePath),
	)

	data := map[string]interface{}{
		"file":           serviceResult.RelativePath,
		"object_id":      serviceResult.ObjectID,
		"type":           serviceResult.ObjectType,
		"updated_fields": serviceResult.ResolvedUpdates,
	}
	if len(serviceResult.PreviousFields) > 0 {
		data["previous_fields"] = serviceResult.PreviousFields
	}

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

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return commandexec.Failure("SCHEMA_INVALID", "failed to load schema", nil, "Fix schema.yaml and try again")
	}

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
		ParseOptions: buildParseOptions(vaultCfg),
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	var warnings []commandexec.Warning
	if serviceResult.Modified {
		warnings = autoReindexWarnings(vaultPath, vaultCfg, serviceResult.FilePath)
	}

	return commandexec.SuccessWithWarnings(map[string]interface{}{
		"file":            serviceResult.RelativePath,
		"object_id":       serviceResult.ObjectID,
		"type":            serviceResult.ObjectType,
		"removed_fields":  fieldmutation.SerializeFieldValueMap(serviceResult.RemovedFields),
		"missing_fields":  serviceResult.MissingFields,
		"modified":        serviceResult.Modified,
		"previous_fields": serviceResult.PreviousFields,
	}, warnings, nil)
}

func runSetBulk(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, ids []string, updates map[string]schema.FieldValue, confirm bool) commandexec.Result {
	request := objectsvc.SetBulkRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		Schema:       sch,
		ObjectIDs:    ids,
		TypedUpdates: updates,
		ParseOptions: buildParseOptions(vaultCfg),
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
			"warnings": nil,
			"fields":   serializedUpdates,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	var reindexWarnings []commandexec.Warning
	summary, err := objectsvc.ApplySetBulk(request, func(filePath string) {
		reindexWarnings = appendCommandWarnings(reindexWarnings, autoReindexWarnings(vaultPath, vaultCfg, filePath))
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	return commandexec.SuccessWithWarnings(map[string]interface{}{
		"ok":       summary.Errors == 0,
		"action":   summary.Action,
		"results":  canonicalSetResults(summary.Results),
		"total":    summary.Total,
		"skipped":  summary.Skipped,
		"errors":   summary.Errors,
		"modified": summary.Modified,
		"fields":   serializedUpdates,
	}, reindexWarnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}
