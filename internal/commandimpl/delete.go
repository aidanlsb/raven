package commandimpl

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/schema"
)

// HandleDelete executes the canonical `delete` command.
func HandleDelete(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	objectIDs := commandIDsArg(req.Args, "object_ids")
	stdinMode := boolArg(req.Args, "stdin") || len(objectIDs) > 0
	if stdinMode {
		if len(objectIDs) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no object IDs provided via stdin", nil, "Pipe object IDs to stdin, one per line")
		}
		return runDeleteBulk(vaultPath, vaultCfg, objectIDs, req.Confirm)
	}

	reference := strings.TrimSpace(stringArg(req.Args, "object_id"))
	if reference == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires object-id argument", nil, "Usage: rvn delete <object-id>")
	}

	sch, _ := schema.Load(vaultPath)
	deletionCfg := vaultCfg.GetDeletionConfig()
	if req.Preview {
		preview, err := objectsvc.PreviewDeleteByReference(objectsvc.DeleteByReferenceRequest{
			VaultPath:   vaultPath,
			VaultConfig: vaultCfg,
			Schema:      sch,
			Reference:   reference,
			Behavior:    deletionCfg.Behavior,
			TrashDir:    deletionCfg.TrashDir,
		})
		if err != nil {
			return mapContentMutationError(err)
		}

		warnings := deleteBacklinkCommandWarnings(preview.Backlinks)
		return commandexec.SuccessWithWarnings(map[string]interface{}{
			"preview":   true,
			"object_id": preview.ObjectID,
			"behavior":  preview.Behavior,
			"trash_dir": deletionCfg.TrashDir,
			"backlinks": preview.Backlinks,
		}, warnings, nil)
	}

	serviceResult, err := objectsvc.DeleteByReference(objectsvc.DeleteByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		Schema:      sch,
		Reference:   reference,
		Behavior:    deletionCfg.Behavior,
		TrashDir:    deletionCfg.TrashDir,
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	warnings := make([]commandexec.Warning, 0, len(serviceResult.WarningMessages)+1)
	warnings = append(warnings, deleteBacklinkCommandWarnings(serviceResult.Backlinks)...)
	warnings = append(warnings, warningMessagesToCommandWarnings(serviceResult.WarningMessages, indexUpdateFailedWarningCode)...)

	data := map[string]interface{}{
		"deleted":  serviceResult.ObjectID,
		"behavior": serviceResult.Behavior,
	}
	if serviceResult.TrashPath != "" {
		relDest, relErr := filepath.Rel(vaultPath, serviceResult.TrashPath)
		if relErr == nil {
			data["trash_path"] = filepath.ToSlash(relDest)
		}
	}

	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

func runDeleteBulk(vaultPath string, vaultCfg *config.VaultConfig, ids []string, confirm bool) commandexec.Result {
	fileIDs, sectionIDs := splitSectionIDs(ids)
	warnings := sectionSkipWarnings(sectionIDs)
	deletionCfg := vaultCfg.GetDeletionConfig()
	request := objectsvc.DeleteBulkRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		ObjectIDs:   fileIDs,
		Behavior:    deletionCfg.Behavior,
		TrashDir:    deletionCfg.TrashDir,
	}

	if !confirm {
		preview, err := objectsvc.PreviewDeleteBulk(request)
		if err != nil {
			return mapContentMutationError(err)
		}
		return commandexec.Success(map[string]interface{}{
			"preview":  true,
			"action":   preview.Action,
			"items":    canonicalDeletePreviewItems(preview.Items),
			"skipped":  canonicalDeleteResults(preview.Skipped),
			"total":    preview.Total,
			"warnings": warnings,
			"behavior": preview.Behavior,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := objectsvc.ApplyDeleteBulk(request)
	if err != nil {
		return mapContentMutationError(err)
	}

	allWarnings := append([]commandexec.Warning{}, warnings...)
	allWarnings = append(allWarnings, warningMessagesToCommandWarnings(summary.WarningMessages, indexUpdateFailedWarningCode)...)
	return commandexec.SuccessWithWarnings(map[string]interface{}{
		"ok":       summary.Errors == 0,
		"action":   summary.Action,
		"results":  canonicalDeleteResults(summary.Results),
		"total":    summary.Total,
		"skipped":  summary.Skipped,
		"errors":   summary.Errors,
		"deleted":  summary.Deleted,
		"behavior": summary.Behavior,
	}, allWarnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}

func deleteBacklinkCommandWarnings(backlinks []model.Reference) []commandexec.Warning {
	if len(backlinks) == 0 {
		return nil
	}

	backlinkIDs := make([]string, 0, len(backlinks))
	for _, bl := range backlinks {
		backlinkIDs = append(backlinkIDs, bl.SourceID)
	}

	return []commandexec.Warning{{
		Code:    codes.WarnBacklinks,
		Message: fmt.Sprintf("Object is referenced by %d other objects", len(backlinks)),
		Ref:     strings.Join(backlinkIDs, ", "),
	}}
}
