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
			"object":     result.Directories.Object,
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
