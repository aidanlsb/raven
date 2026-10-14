package commandimpl

import (
	"context"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/vaultconfigsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// vaultConfigOperation defines a single vault config operation and how to execute it.
type vaultConfigOperation struct {
	Execute func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result
}

// vaultConfigOperationTable maps command IDs to their operation definitions.
var vaultConfigOperationTable = map[string]vaultConfigOperation{
	"vault_config_auto_reindex_set": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.SetAutoReindex(rt, boolArgDefault(req.Args, "value", true))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.VaultConfigAutoReindexResult{
				ConfigPath:          result.ConfigPath,
				Created:             &result.Created,
				Changed:             result.Changed,
				AutoReindex:         result.AutoReindex,
				AutoReindexExplicit: result.Explicit,
			}, nil)
		},
	},

	"vault_config_auto_reindex_unset": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.UnsetAutoReindex(rt)
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.VaultConfigAutoReindexResult{
				ConfigPath:          result.ConfigPath,
				Changed:             result.Changed,
				AutoReindex:         result.AutoReindex,
				AutoReindexExplicit: result.Explicit,
			}, nil)
		},
	},

	"vault_config_protected_prefixes_list": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.ListProtectedPrefixes(rt)
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.VaultConfigProtectedPrefixesListResult{
				ConfigPath:        result.ConfigPath,
				Exists:            result.Exists,
				ProtectedPrefixes: result.Prefixes,
			}, &commandexec.Meta{Count: len(result.Prefixes)})
		},
	},

	"vault_config_protected_prefixes_add": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.AddProtectedPrefix(rt, stringArg(req.Args, "prefix"))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.VaultConfigProtectedPrefixAddResult{
				ConfigPath:        result.ConfigPath,
				Created:           result.Created,
				Changed:           result.Changed,
				Prefix:            result.Prefix,
				ProtectedPrefixes: result.Prefixes,
			}, nil)
		},
	},

	"vault_config_protected_prefixes_remove": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.RemoveProtectedPrefix(rt, stringArg(req.Args, "prefix"))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.VaultConfigProtectedPrefixRemoveResult{
				ConfigPath:        result.ConfigPath,
				Changed:           result.Changed,
				Removed:           result.Prefix,
				ProtectedPrefixes: result.Prefixes,
			}, nil)
		},
	},

	"vault_config_exclude_list": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.ListExclude(rt)
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.VaultConfigExcludeListResult{
				ConfigPath: result.ConfigPath,
				Exists:     result.Exists,
				Exclude:    result.Patterns,
			}, &commandexec.Meta{Count: len(result.Patterns)})
		},
	},

	"vault_config_exclude_add": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.AddExclude(rt, stringArg(req.Args, "pattern"))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.VaultConfigExcludeAddResult{
				ConfigPath: result.ConfigPath,
				Created:    result.Created,
				Changed:    result.Changed,
				Pattern:    result.Pattern,
				Exclude:    result.Patterns,
			}, nil)
		},
	},

	"vault_config_exclude_remove": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.RemoveExclude(rt, stringArg(req.Args, "pattern"))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.VaultConfigExcludeRemoveResult{
				ConfigPath: result.ConfigPath,
				Changed:    result.Changed,
				Removed:    result.Pattern,
				Exclude:    result.Patterns,
			}, nil)
		},
	},

	"vault_config_directories_get": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.GetDirectories(rt)
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(vaultDirectoriesData(result.ConfigPath, result.Exists, result.Directories), nil)
		},
	},

	"vault_config_directories_set": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.SetDirectories(rt,
				optionalStringArg(req.Args, "daily"),
				optionalStringArg(req.Args, "type"),
				optionalStringArg(req.Args, "page"),
				optionalStringArg(req.Args, "template"))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			data := vaultDirectoriesData(result.ConfigPath, true, result.Directories)
			data.Created = &result.Created
			data.Changed = &result.Changed
			return commandexec.Success(data, nil)
		},
	},

	"vault_config_directories_unset": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.UnsetDirectories(rt,
				boolArg(req.Args, "daily"),
				boolArg(req.Args, "type"),
				boolArg(req.Args, "page"),
				boolArg(req.Args, "template"))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			data := vaultDirectoriesData(result.ConfigPath, true, result.Directories)
			data.Changed = &result.Changed
			return commandexec.Success(data, nil)
		},
	},

	"vault_config_capture_get": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.GetCapture(rt)
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(vaultCaptureData(result.ConfigPath, result.Exists, result.Configured, result.Capture), nil)
		},
	},

	"vault_config_capture_set": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.SetCapture(rt,
				optionalStringArg(req.Args, "destination"),
				optionalStringArg(req.Args, "heading"))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			data := vaultCaptureData(result.ConfigPath, true, result.Configured, result.Capture)
			data.Created = &result.Created
			data.Changed = &result.Changed
			return commandexec.Success(data, nil)
		},
	},

	"vault_config_capture_unset": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.UnsetCapture(rt,
				boolArg(req.Args, "destination"),
				boolArg(req.Args, "heading"))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			data := vaultCaptureData(result.ConfigPath, true, result.Configured, result.Capture)
			data.Changed = &result.Changed
			return commandexec.Success(data, nil)
		},
	},

	"vault_config_deletion_get": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.GetDeletion(rt)
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(vaultDeletionData(result.ConfigPath, result.Exists, result.Configured, result.Deletion), nil)
		},
	},

	"vault_config_deletion_set": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.SetDeletion(rt,
				optionalStringArg(req.Args, "behavior"),
				optionalStringArg(req.Args, "trash-dir"))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			data := vaultDeletionData(result.ConfigPath, true, result.Configured, result.Deletion)
			data.Created = &result.Created
			data.Changed = &result.Changed
			return commandexec.Success(data, nil)
		},
	},

	"vault_config_deletion_unset": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.UnsetDeletion(rt,
				boolArg(req.Args, "behavior"),
				boolArg(req.Args, "trash-dir"))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			data := vaultDeletionData(result.ConfigPath, true, result.Configured, result.Deletion)
			data.Changed = &result.Changed
			return commandexec.Success(data, nil)
		},
	},
}

// handleVaultConfigOperation is a generic handler for vault config operations.
// It looks up the operation in the table and executes it.
func handleVaultConfigOperation(_ context.Context, req commandexec.Request, commandID string) commandexec.Result {
	op, exists := vaultConfigOperationTable[commandID]
	if !exists {
		return commandexec.Failure("INVALID_INPUT", "unknown vault config operation: "+commandID, nil, "")
	}

	rt, failure := newLazyConfigCommandRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	return op.Execute(rt, req)
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
