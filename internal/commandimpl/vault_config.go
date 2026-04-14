package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/vaultconfigsvc"
)

func HandleVaultConfigShow(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.Show(vaultConfigShowRequest(req))
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	return commandexec.Success(map[string]interface{}{
		"config_path":           result.ConfigPath,
		"exists":                result.Exists,
		"auto_reindex":          result.AutoReindex,
		"auto_reindex_explicit": result.AutoReindexExplicit,
		"daily_template":        result.DailyTemplate,
		"directories": map[string]interface{}{
			"configured": result.Directories.Configured,
			"daily":      result.Directories.Daily,
			"type":       result.Directories.Object,
			"page":       result.Directories.Page,
			"template":   result.Directories.Template,
		},
		"capture": map[string]interface{}{
			"destination": result.Capture.Destination,
			"heading":     result.Capture.Heading,
		},
		"deletion": map[string]interface{}{
			"behavior":  result.Deletion.Behavior,
			"trash_dir": result.Deletion.TrashDir,
		},
		"queries_count":            result.QueriesCount,
		"protected_prefixes":       result.ProtectedPrefixes,
		"protected_prefixes_count": len(result.ProtectedPrefixes),
	}, &commandexec.Meta{Count: len(result.ProtectedPrefixes)})
}

func HandleVaultConfigAutoReindexSet(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.SetAutoReindex(vaultconfigsvc.SetAutoReindexRequest{
		VaultPath: req.VaultPath,
		Value:     boolArgDefault(req.Args, "value", true),
	})
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	return commandexec.Success(map[string]interface{}{
		"config_path":           result.ConfigPath,
		"created":               result.Created,
		"changed":               result.Changed,
		"auto_reindex":          result.AutoReindex,
		"auto_reindex_explicit": result.AutoReindexExplicit,
	}, nil)
}

func HandleVaultConfigAutoReindexUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.UnsetAutoReindex(vaultconfigsvc.UnsetAutoReindexRequest{
		VaultPath: req.VaultPath,
	})
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	return commandexec.Success(map[string]interface{}{
		"config_path":           result.ConfigPath,
		"changed":               result.Changed,
		"auto_reindex":          result.AutoReindex,
		"auto_reindex_explicit": result.AutoReindexExplicit,
	}, nil)
}

func HandleVaultConfigProtectedPrefixesList(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.ListProtectedPrefixes(vaultconfigsvc.ListProtectedPrefixesRequest{
		VaultPath: req.VaultPath,
	})
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	return commandexec.Success(map[string]interface{}{
		"config_path":        result.ConfigPath,
		"exists":             result.Exists,
		"protected_prefixes": result.ProtectedPrefixes,
	}, &commandexec.Meta{Count: len(result.ProtectedPrefixes)})
}

func HandleVaultConfigProtectedPrefixesAdd(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.AddProtectedPrefix(vaultconfigsvc.AddProtectedPrefixRequest{
		VaultPath: req.VaultPath,
		Prefix:    strings.TrimSpace(stringArg(req.Args, "prefix")),
	})
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	return commandexec.Success(map[string]interface{}{
		"config_path":        result.ConfigPath,
		"created":            result.Created,
		"changed":            result.Changed,
		"prefix":             result.Prefix,
		"protected_prefixes": result.ProtectedPrefixes,
	}, nil)
}

func HandleVaultConfigProtectedPrefixesRemove(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.RemoveProtectedPrefix(vaultconfigsvc.RemoveProtectedPrefixRequest{
		VaultPath: req.VaultPath,
		Prefix:    strings.TrimSpace(stringArg(req.Args, "prefix")),
	})
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	return commandexec.Success(map[string]interface{}{
		"config_path":        result.ConfigPath,
		"changed":            result.Changed,
		"removed":            result.Removed,
		"protected_prefixes": result.ProtectedPrefixes,
	}, nil)
}

func HandleVaultConfigDirectoriesGet(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.GetDirectories(vaultconfigsvc.GetDirectoriesRequest{
		VaultPath: req.VaultPath,
	})
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	return commandexec.Success(vaultDirectoriesData(result.ConfigPath, result.Exists, result.Directories), nil)
}

func HandleVaultConfigDirectoriesSet(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.SetDirectories(vaultconfigsvc.SetDirectoriesRequest{
		VaultPath: req.VaultPath,
		Daily:     optionalStringArg(req.Args, "daily"),
		Object:    optionalStringArg(req.Args, "type"),
		Page:      optionalStringArg(req.Args, "page"),
		Template:  optionalStringArg(req.Args, "template"),
	})
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	data := vaultDirectoriesData(result.ConfigPath, true, result.Directories)
	data["created"] = result.Created
	data["changed"] = result.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigDirectoriesUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.UnsetDirectories(vaultconfigsvc.UnsetDirectoriesRequest{
		VaultPath: req.VaultPath,
		Daily:     boolArg(req.Args, "daily"),
		Object:    boolArg(req.Args, "type"),
		Page:      boolArg(req.Args, "page"),
		Template:  boolArg(req.Args, "template"),
	})
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	data := vaultDirectoriesData(result.ConfigPath, true, result.Directories)
	data["changed"] = result.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigCaptureGet(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.GetCapture(vaultconfigsvc.GetCaptureRequest{
		VaultPath: req.VaultPath,
	})
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	return commandexec.Success(vaultCaptureData(result.ConfigPath, result.Exists, result.Configured, result.Capture), nil)
}

func HandleVaultConfigCaptureSet(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.SetCapture(vaultconfigsvc.SetCaptureRequest{
		VaultPath:   req.VaultPath,
		Destination: optionalStringArg(req.Args, "destination"),
		Heading:     optionalStringArg(req.Args, "heading"),
	})
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	data := vaultCaptureData(result.ConfigPath, true, result.Configured, result.Capture)
	data["created"] = result.Created
	data["changed"] = result.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigCaptureUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.UnsetCapture(vaultconfigsvc.UnsetCaptureRequest{
		VaultPath:   req.VaultPath,
		Destination: boolArg(req.Args, "destination"),
		Heading:     boolArg(req.Args, "heading"),
	})
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	data := vaultCaptureData(result.ConfigPath, true, result.Configured, result.Capture)
	data["changed"] = result.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigDeletionGet(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.GetDeletion(vaultconfigsvc.GetDeletionRequest{
		VaultPath: req.VaultPath,
	})
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	return commandexec.Success(vaultDeletionData(result.ConfigPath, result.Exists, result.Configured, result.Deletion), nil)
}

func HandleVaultConfigDeletionSet(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.SetDeletion(vaultconfigsvc.SetDeletionRequest{
		VaultPath: req.VaultPath,
		Behavior:  optionalStringArg(req.Args, "behavior"),
		TrashDir:  optionalStringArg(req.Args, "trash-dir"),
	})
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	data := vaultDeletionData(result.ConfigPath, true, result.Configured, result.Deletion)
	data["created"] = result.Created
	data["changed"] = result.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigDeletionUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := vaultconfigsvc.UnsetDeletion(vaultconfigsvc.UnsetDeletionRequest{
		VaultPath: req.VaultPath,
		Behavior:  boolArg(req.Args, "behavior"),
		TrashDir:  boolArg(req.Args, "trash-dir"),
	})
	if err != nil {
		return mapVaultConfigFailure(err)
	}
	data := vaultDeletionData(result.ConfigPath, true, result.Configured, result.Deletion)
	data["changed"] = result.Changed
	return commandexec.Success(data, nil)
}

func mapVaultConfigFailure(err error) commandexec.Result {
	svcErr, ok := vaultconfigsvc.AsError(err)
	if !ok {
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
	}
	return commandexec.Failure(string(svcErr.Code), svcErr.Error(), nil, svcErr.Suggestion)
}

func vaultConfigShowRequest(req commandexec.Request) vaultconfigsvc.ShowRequest {
	return vaultconfigsvc.ShowRequest{
		VaultPath: req.VaultPath,
	}
}

func vaultDirectoriesData(configPath string, exists bool, info vaultconfigsvc.DirectoriesInfo) map[string]interface{} {
	return map[string]interface{}{
		"config_path": configPath,
		"exists":      exists,
		"configured":  info.Configured,
		"daily":       info.Daily,
		"type":        info.Object,
		"page":        info.Page,
		"template":    info.Template,
	}
}

func vaultCaptureData(configPath string, exists, configured bool, info vaultconfigsvc.CaptureInfo) map[string]interface{} {
	return map[string]interface{}{
		"config_path": configPath,
		"exists":      exists,
		"configured":  configured,
		"destination": info.Destination,
		"heading":     info.Heading,
	}
}

func vaultDeletionData(configPath string, exists, configured bool, info vaultconfigsvc.DeletionInfo) map[string]interface{} {
	return map[string]interface{}{
		"config_path": configPath,
		"exists":      exists,
		"configured":  configured,
		"behavior":    info.Behavior,
		"trash_dir":   info.TrashDir,
	}
}

func optionalStringArg(args map[string]any, key string) *string {
	if args == nil {
		return nil
	}
	raw, ok := args[key]
	if !ok {
		return nil
	}
	value := stringArg(args, key)
	if value == "" {
		if _, isString := raw.(string); isString {
			return &value
		}
		return nil
	}
	return &value
}
