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
	result, err := vaultconfigsvc.Show(rt)
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
	mutation, autoReindex, explicit, err := vaultconfigsvc.SetAutoReindex(rt, boolArgDefault(req.Args, "value", true))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigAutoReindexResult{
		ConfigPath:          mutation.ConfigPath,
		Created:             &mutation.Created,
		Changed:             mutation.Changed,
		AutoReindex:         autoReindex,
		AutoReindexExplicit: explicit,
	}, nil)
}

func HandleVaultConfigAutoReindexUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	mutation, autoReindex, explicit, err := vaultconfigsvc.UnsetAutoReindex(rt)
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigAutoReindexResult{
		ConfigPath:          mutation.ConfigPath,
		Changed:             mutation.Changed,
		AutoReindex:         autoReindex,
		AutoReindexExplicit: explicit,
	}, nil)
}

func HandleVaultConfigProtectedPrefixesList(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	configPath, exists, prefixes, err := vaultconfigsvc.ListProtectedPrefixes(rt)
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigProtectedPrefixesListResult{
		ConfigPath:        configPath,
		Exists:            exists,
		ProtectedPrefixes: prefixes,
	}, &commandexec.Meta{Count: len(prefixes)})
}

func HandleVaultConfigProtectedPrefixesAdd(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	mutation, prefix, prefixes, err := vaultconfigsvc.AddProtectedPrefix(rt, strings.TrimSpace(stringArg(req.Args, "prefix")))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigProtectedPrefixAddResult{
		ConfigPath:        mutation.ConfigPath,
		Created:           mutation.Created,
		Changed:           mutation.Changed,
		Prefix:            prefix,
		ProtectedPrefixes: prefixes,
	}, nil)
}

func HandleVaultConfigProtectedPrefixesRemove(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	mutation, removed, prefixes, err := vaultconfigsvc.RemoveProtectedPrefix(rt, strings.TrimSpace(stringArg(req.Args, "prefix")))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigProtectedPrefixRemoveResult{
		ConfigPath:        mutation.ConfigPath,
		Changed:           mutation.Changed,
		Removed:           removed,
		ProtectedPrefixes: prefixes,
	}, nil)
}

func HandleVaultConfigExcludeList(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	configPath, exists, patterns, err := vaultconfigsvc.ListExclude(rt)
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigExcludeListResult{
		ConfigPath: configPath,
		Exists:     exists,
		Exclude:    patterns,
	}, &commandexec.Meta{Count: len(patterns)})
}

func HandleVaultConfigExcludeAdd(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	mutation, pattern, patterns, err := vaultconfigsvc.AddExclude(rt, strings.TrimSpace(stringArg(req.Args, "pattern")))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigExcludeAddResult{
		ConfigPath: mutation.ConfigPath,
		Created:    mutation.Created,
		Changed:    mutation.Changed,
		Pattern:    pattern,
		Exclude:    patterns,
	}, nil)
}

func HandleVaultConfigExcludeRemove(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	mutation, removed, patterns, err := vaultconfigsvc.RemoveExclude(rt, strings.TrimSpace(stringArg(req.Args, "pattern")))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.VaultConfigExcludeRemoveResult{
		ConfigPath: mutation.ConfigPath,
		Changed:    mutation.Changed,
		Removed:    removed,
		Exclude:    patterns,
	}, nil)
}

func HandleVaultConfigDirectoriesGet(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	configPath, exists, directories, err := vaultconfigsvc.GetDirectories(rt)
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(vaultDirectoriesData(configPath, exists, directories), nil)
}

func HandleVaultConfigDirectoriesSet(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	mutation, directories, err := vaultconfigsvc.SetDirectories(rt,
		optionalStringArg(req.Args, "daily"),
		optionalStringArg(req.Args, "type"),
		optionalStringArg(req.Args, "page"),
		optionalStringArg(req.Args, "template"))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	data := vaultDirectoriesData(mutation.ConfigPath, true, directories)
	data.Created = &mutation.Created
	data.Changed = &mutation.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigDirectoriesUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	mutation, directories, err := vaultconfigsvc.UnsetDirectories(rt,
		boolArg(req.Args, "daily"),
		boolArg(req.Args, "type"),
		boolArg(req.Args, "page"),
		boolArg(req.Args, "template"))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	data := vaultDirectoriesData(mutation.ConfigPath, true, directories)
	data.Changed = &mutation.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigCaptureGet(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	configPath, exists, configured, capture, err := vaultconfigsvc.GetCapture(rt)
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(vaultCaptureData(configPath, exists, configured, capture), nil)
}

func HandleVaultConfigCaptureSet(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	mutation, configured, capture, err := vaultconfigsvc.SetCapture(rt,
		optionalStringArg(req.Args, "destination"),
		optionalStringArg(req.Args, "heading"))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	data := vaultCaptureData(mutation.ConfigPath, true, configured, capture)
	data.Created = &mutation.Created
	data.Changed = &mutation.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigCaptureUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	mutation, configured, capture, err := vaultconfigsvc.UnsetCapture(rt,
		boolArg(req.Args, "destination"),
		boolArg(req.Args, "heading"))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	data := vaultCaptureData(mutation.ConfigPath, true, configured, capture)
	data.Changed = &mutation.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigDeletionGet(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	configPath, exists, configured, deletion, err := vaultconfigsvc.GetDeletion(rt)
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(vaultDeletionData(configPath, exists, configured, deletion), nil)
}

func HandleVaultConfigDeletionSet(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	mutation, configured, deletion, err := vaultconfigsvc.SetDeletion(rt,
		optionalStringArg(req.Args, "behavior"),
		optionalStringArg(req.Args, "trash-dir"))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	data := vaultDeletionData(mutation.ConfigPath, true, configured, deletion)
	data.Created = &mutation.Created
	data.Changed = &mutation.Changed
	return commandexec.Success(data, nil)
}

func HandleVaultConfigDeletionUnset(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	mutation, configured, deletion, err := vaultconfigsvc.UnsetDeletion(rt,
		boolArg(req.Args, "behavior"),
		boolArg(req.Args, "trash-dir"))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	data := vaultDeletionData(mutation.ConfigPath, true, configured, deletion)
	data.Changed = &mutation.Changed
	return commandexec.Success(data, nil)
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
