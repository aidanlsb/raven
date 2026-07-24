package commandimpl

import (
	"context"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/objectsvc"
)

// HandleReclassify executes the canonical `reclassify` command.
func HandleReclassify(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	rt, failure := newConfigCommandVaultRuntime(vaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg
	if rt.SchemaLoadErr != nil {
		return commandexec.Failure("SCHEMA_NOT_FOUND", "failed to load schema", nil, "Run 'rvn init' to create a schema")
	}
	sch := rt.Schema

	fieldValues, err := parseKeyValueArgs(req.Args["field"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "Use --field key=value")
	}
	typedFieldValues, err := parseTypedFieldValues(req.Args["fields-json"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", "invalid --fields-json payload", nil, "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
	}
	allFieldValues := mergeFieldInputs(fieldValues, typedFieldValues)

	result, err := objectsvc.ReclassifyByReference(objectsvc.ReclassifyByReferenceRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		Schema:       sch,
		Reference:    strings.TrimSpace(stringArg(req.Args, "object")),
		NewTypeName:  strings.TrimSpace(stringArg(req.Args, "new-type")),
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

	data := map[string]interface{}{
		"object_id":      result.ObjectID,
		"old_type":       result.OldType,
		"new_type":       result.NewType,
		"file":           result.File,
		"moved":          result.Moved,
		"old_path":       result.OldPath,
		"new_path":       result.NewPath,
		"updated_refs":   result.UpdatedRefs,
		"added_fields":   result.AddedFields,
		"dropped_fields": result.DroppedFields,
		"needs_confirm":  result.NeedsConfirm,
		"reason":         result.Reason,
	}

	warnings := make([]commandexec.Warning, 0, len(result.WarningMessages))
	for _, warning := range result.WarningMessages {
		warnings = append(warnings, commandexec.Warning{
			Code:    indexUpdateFailedWarningCode,
			Message: warning,
		})
	}
	if !result.NeedsConfirm {
		postData, postWarnings := applyChangeSet(rt, result.ChangeSet)
		data = mergeDataFields(data, postData)
		warnings = appendCommandWarnings(warnings, postWarnings)
	}

	res := commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	if result.NeedsConfirm {
		// Field-dropping reclassify is blocked pending --force; nothing was
		// written, so report a preview phase.
		res = res.WithMutationPhase(commandexec.MutationPhasePreview)
	}
	return res
}
