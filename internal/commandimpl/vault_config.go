package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/vaultconfigsvc"
)

func HandleVaultConfigShow(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.Show(rt, vaultConfigShowRequest(req))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigShowResult{
		ConfigPath:          result.ConfigPath,
		Exists:              result.Exists,
		AutoReindex:         result.AutoReindex,
		AutoReindexExplicit: result.AutoReindexExplicit,
		DailyTemplate:       result.DailyTemplate,
		Directories: commandpayload.VaultDirectories{
			Configured: result.Directories.Configured,
			Daily:      result.Directories.Daily,
			Type:       result.Directories.Object,
			Page:       result.Directories.Page,
			Template:   result.Directories.Template,
		},
		Capture: commandpayload.VaultCapture{
			Destination: result.Capture.Destination,
			Heading:     result.Capture.Heading,
		},
		Deletion: commandpayload.VaultDeletion{
			Behavior: result.Deletion.Behavior,
			TrashDir: result.Deletion.TrashDir,
		},
		QueriesCount:           result.QueriesCount,
		ProtectedPrefixes:      result.ProtectedPrefixes,
		ProtectedPrefixesCount: len(result.ProtectedPrefixes),
		Exclude:                result.Exclude,
		ExcludeCount:           len(result.Exclude),
	}, &commandexec.Meta{Count: len(result.ProtectedPrefixes) + len(result.Exclude)})
}

func HandleVaultConfigAutoReindexSet(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.SetAutoReindex(rt, vaultconfigsvc.SetAutoReindexRequest{
		VaultPath: req.VaultPath,
		Value:     boolArgDefault(req.Args, "value", true),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigAutoReindexResult{
		ConfigPath:          result.ConfigPath,
		Created:             &result.Created,
		Changed:             result.Changed,
		AutoReindex:         result.AutoReindex,
		AutoReindexExplicit: result.AutoReindexExplicit,
	}, nil)
}

func HandleVaultConfigAutoReindexUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.UnsetAutoReindex(rt, vaultconfigsvc.UnsetAutoReindexRequest{
		VaultPath: req.VaultPath,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigAutoReindexResult{
		ConfigPath:          result.ConfigPath,
		Changed:             result.Changed,
		AutoReindex:         result.AutoReindex,
		AutoReindexExplicit: result.AutoReindexExplicit,
	}, nil)
}

func HandleVaultConfigProtectedPrefixesList(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.ListProtectedPrefixes(rt, vaultconfigsvc.ListProtectedPrefixesRequest{
		VaultPath: req.VaultPath,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigProtectedPrefixesListResult{
		ConfigPath:        result.ConfigPath,
		Exists:            result.Exists,
		ProtectedPrefixes: result.ProtectedPrefixes,
	}, &commandexec.Meta{Count: len(result.ProtectedPrefixes)})
}

func HandleVaultConfigProtectedPrefixesAdd(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.AddProtectedPrefix(rt, vaultconfigsvc.AddProtectedPrefixRequest{
		VaultPath: req.VaultPath,
		Prefix:    strings.TrimSpace(stringArg(req.Args, "prefix")),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigProtectedPrefixAddResult{
		ConfigPath:        result.ConfigPath,
		Created:           result.Created,
		Changed:           result.Changed,
		Prefix:            result.Prefix,
		ProtectedPrefixes: result.ProtectedPrefixes,
	}, nil)
}

func HandleVaultConfigProtectedPrefixesRemove(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.RemoveProtectedPrefix(rt, vaultconfigsvc.RemoveProtectedPrefixRequest{
		VaultPath: req.VaultPath,
		Prefix:    strings.TrimSpace(stringArg(req.Args, "prefix")),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigProtectedPrefixRemoveResult{
		ConfigPath:        result.ConfigPath,
		Changed:           result.Changed,
		Removed:           result.Removed,
		ProtectedPrefixes: result.ProtectedPrefixes,
	}, nil)
}

func HandleVaultConfigExcludeList(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.ListExclude(rt, vaultconfigsvc.ListExcludeRequest{
		VaultPath: req.VaultPath,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigExcludeListResult{
		ConfigPath: result.ConfigPath,
		Exists:     result.Exists,
		Exclude:    result.Exclude,
	}, &commandexec.Meta{Count: len(result.Exclude)})
}

func HandleVaultConfigExcludeAdd(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.AddExclude(rt, vaultconfigsvc.AddExcludeRequest{
		VaultPath: req.VaultPath,
		Pattern:   strings.TrimSpace(stringArg(req.Args, "pattern")),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigExcludeAddResult{
		ConfigPath: result.ConfigPath,
		Created:    result.Created,
		Changed:    result.Changed,
		Pattern:    result.Pattern,
		Exclude:    result.Exclude,
	}, nil)
}

func HandleVaultConfigExcludeRemove(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.RemoveExclude(rt, vaultconfigsvc.RemoveExcludeRequest{
		VaultPath: req.VaultPath,
		Pattern:   strings.TrimSpace(stringArg(req.Args, "pattern")),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigExcludeRemoveResult{
		ConfigPath: result.ConfigPath,
		Changed:    result.Changed,
		Removed:    result.Removed,
		Exclude:    result.Exclude,
	}, nil)
}

func HandleVaultConfigDirectoriesGet(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.GetDirectories(rt, vaultconfigsvc.GetDirectoriesRequest{
		VaultPath: req.VaultPath,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(vaultDirectoriesData(result.ConfigPath, result.Exists, result.Directories), nil)
}

func HandleVaultConfigDirectoriesSet(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.SetDirectories(rt, vaultconfigsvc.SetDirectoriesRequest{
		VaultPath: req.VaultPath,
		Daily:     optionalStringArg(req.Args, "daily"),
		Object:    optionalStringArg(req.Args, "type"),
		Page:      optionalStringArg(req.Args, "page"),
		Template:  optionalStringArg(req.Args, "template"),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	data := vaultDirectoriesData(result.ConfigPath, true, result.Directories)
	data.Created = &result.Created
	data.Changed = &result.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigDirectoriesUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.UnsetDirectories(rt, vaultconfigsvc.UnsetDirectoriesRequest{
		VaultPath: req.VaultPath,
		Daily:     boolArg(req.Args, "daily"),
		Object:    boolArg(req.Args, "type"),
		Page:      boolArg(req.Args, "page"),
		Template:  boolArg(req.Args, "template"),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	data := vaultDirectoriesData(result.ConfigPath, true, result.Directories)
	data.Changed = &result.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigCaptureGet(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.GetCapture(rt, vaultconfigsvc.GetCaptureRequest{
		VaultPath: req.VaultPath,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(vaultCaptureData(result.ConfigPath, result.Exists, result.Configured, result.Capture), nil)
}

func HandleVaultConfigCaptureSet(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.SetCapture(rt, vaultconfigsvc.SetCaptureRequest{
		VaultPath:   req.VaultPath,
		Destination: optionalStringArg(req.Args, "destination"),
		Heading:     optionalStringArg(req.Args, "heading"),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	data := vaultCaptureData(result.ConfigPath, true, result.Configured, result.Capture)
	data.Created = &result.Created
	data.Changed = &result.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigCaptureUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.UnsetCapture(rt, vaultconfigsvc.UnsetCaptureRequest{
		VaultPath:   req.VaultPath,
		Destination: boolArg(req.Args, "destination"),
		Heading:     boolArg(req.Args, "heading"),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	data := vaultCaptureData(result.ConfigPath, true, result.Configured, result.Capture)
	data.Changed = &result.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigDeletionGet(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.GetDeletion(rt, vaultconfigsvc.GetDeletionRequest{
		VaultPath: req.VaultPath,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(vaultDeletionData(result.ConfigPath, result.Exists, result.Configured, result.Deletion), nil)
}

func HandleVaultConfigDeletionSet(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.SetDeletion(rt, vaultconfigsvc.SetDeletionRequest{
		VaultPath: req.VaultPath,
		Behavior:  optionalStringArg(req.Args, "behavior"),
		TrashDir:  optionalStringArg(req.Args, "trash-dir"),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	data := vaultDeletionData(result.ConfigPath, true, result.Configured, result.Deletion)
	data.Created = &result.Created
	data.Changed = &result.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigDeletionUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := vaultconfigsvc.UnsetDeletion(rt, vaultconfigsvc.UnsetDeletionRequest{
		VaultPath: req.VaultPath,
		Behavior:  boolArg(req.Args, "behavior"),
		TrashDir:  boolArg(req.Args, "trash-dir"),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	data := vaultDeletionData(result.ConfigPath, true, result.Configured, result.Deletion)
	data.Changed = &result.Changed
	return commandexec.Success(data, nil)
}

func vaultConfigShowRequest(req commandexec.Request) vaultconfigsvc.ShowRequest {
	return vaultconfigsvc.ShowRequest{
		VaultPath: req.VaultPath,
	}
}

func vaultDirectoriesData(configPath string, exists bool, info vaultconfigsvc.DirectoriesInfo) commandpayload.VaultConfigDirectoriesResult {
	return commandpayload.VaultConfigDirectoriesResult{
		ConfigPath: configPath,
		Exists:     exists,
		Configured: info.Configured,
		Daily:      info.Daily,
		Type:       info.Object,
		Page:       info.Page,
		Template:   info.Template,
	}
}

func vaultCaptureData(configPath string, exists, configured bool, info vaultconfigsvc.CaptureInfo) commandpayload.VaultConfigCaptureResult {
	return commandpayload.VaultConfigCaptureResult{
		ConfigPath:  configPath,
		Exists:      exists,
		Configured:  configured,
		Destination: info.Destination,
		Heading:     info.Heading,
	}
}

func vaultDeletionData(configPath string, exists, configured bool, info vaultconfigsvc.DeletionInfo) commandpayload.VaultConfigDeletionResult {
	return commandpayload.VaultConfigDeletionResult{
		ConfigPath: configPath,
		Exists:     exists,
		Configured: configured,
		Behavior:   info.Behavior,
		TrashDir:   info.TrashDir,
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
