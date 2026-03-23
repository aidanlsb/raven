package commandimpl

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/schema"
)

// HandleReclassify executes the canonical `reclassify` command.
func HandleReclassify(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
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
		return commandexec.Failure("SCHEMA_NOT_FOUND", "failed to load schema", nil, "Run 'rvn init' to create a schema")
	}

	fieldValues, err := parseKeyValueArgs(req.Args["field"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "Use --field key=value")
	}

	result, err := objectsvc.ReclassifyByReference(objectsvc.ReclassifyByReferenceRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		Schema:       sch,
		Reference:    strings.TrimSpace(stringArg(req.Args, "object")),
		NewTypeName:  strings.TrimSpace(stringArg(req.Args, "new-type")),
		FieldValues:  fieldValues,
		NoMove:       boolArg(req.Args, "no-move"),
		UpdateRefs:   boolArgDefault(req.Args, "update-refs", true),
		Force:        boolArg(req.Args, "force"),
		ParseOptions: buildParseOptions(vaultCfg),
	})
	if err != nil {
		return mapReclassifyFailure(err)
	}

	if result.ChangedFilePath != "" {
		autoReindexFile(vaultPath, result.ChangedFilePath, vaultCfg)
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
			Code:    "INDEX_UPDATE_FAILED",
			Message: warning,
		})
	}

	return commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

func mapReclassifyFailure(err error) commandexec.Result {
	var svcErr *objectsvc.Error
	if !errors.As(err, &svcErr) {
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
	}
	return commandexec.Failure(mapServiceCode(svcErr.Code), svcErr.Message, svcErr.Details, svcErr.Suggestion)
}
