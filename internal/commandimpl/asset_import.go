package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/assetsvc"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
)

// HandleAssetImport executes the canonical `asset import` command.
func HandleAssetImport(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure(codes.ErrInvalidInput, "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	rt, failure := newConfigOnlyCommandVaultRuntime(vaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	serviceResult, err := assetsvc.Import(assetsvc.ImportRequest{
		VaultPath:   vaultPath,
		VaultConfig: rt.VaultCfg,
		Source:      stringArg(req.Args, "source"),
		Destination: stringArg(req.Args, "destination"),
		Move:        boolArg(req.Args, "move"),
		Force:       boolArg(req.Args, "force"),
		Preview:     req.Preview,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	item := map[string]interface{}{
		"id":         serviceResult.Asset.ID,
		"path":       serviceResult.Asset.FilePath,
		"file_path":  serviceResult.Asset.FilePath,
		"media_type": serviceResult.Asset.MediaType,
		"size_bytes": serviceResult.Asset.SizeBytes,
		"mode":       string(serviceResult.Mode),
	}
	data := map[string]interface{}{
		"id":         serviceResult.Asset.ID,
		"path":       serviceResult.Asset.FilePath,
		"file_path":  serviceResult.Asset.FilePath,
		"media_type": serviceResult.Asset.MediaType,
		"size_bytes": serviceResult.Asset.SizeBytes,
		"mode":       string(serviceResult.Mode),
		"items":      []interface{}{item},
	}
	if req.Preview {
		data["preview"] = true
		return commandexec.Success(data, &commandexec.Meta{Count: 1})
	}

	postData, postWarnings := applyChangeSet(rt, serviceResult.ChangeSet, req.IndexJournalOperation)
	data = mergeDataFields(data, postData)

	sourceRemoved := false
	if serviceResult.Mode == assetsvc.ModeMove && len(postWarnings) == 0 {
		if err := assetsvc.FinalizeMove(serviceResult); err != nil {
			return commandexec.FromServiceError(err)
		}
		sourceRemoved = true
	}
	if serviceResult.Mode == assetsvc.ModeMove {
		data["source_removed"] = sourceRemoved
		item["source_removed"] = sourceRemoved
	}

	return commandexec.SuccessWithWarnings(data, postWarnings, &commandexec.Meta{Count: 1})
}
