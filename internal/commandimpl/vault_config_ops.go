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
		},
	},

	"vault_config_auto_reindex_unset": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
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
		},
	},

	"vault_config_protected_prefixes_list": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			configPath, exists, prefixes, err := vaultconfigsvc.ListProtectedPrefixes(rt)
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.VaultConfigProtectedPrefixesListResult{
				ConfigPath:        configPath,
				Exists:            exists,
				ProtectedPrefixes: prefixes,
			}, &commandexec.Meta{Count: len(prefixes)})
		},
	},

	"vault_config_protected_prefixes_add": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			mutation, prefix, prefixes, err := vaultconfigsvc.AddProtectedPrefix(rt, stringArg(req.Args, "prefix"))
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
		},
	},

	"vault_config_protected_prefixes_remove": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			mutation, removed, prefixes, err := vaultconfigsvc.RemoveProtectedPrefix(rt, stringArg(req.Args, "prefix"))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.VaultConfigProtectedPrefixRemoveResult{
				ConfigPath:        mutation.ConfigPath,
				Changed:           mutation.Changed,
				Removed:           removed,
				ProtectedPrefixes: prefixes,
			}, nil)
		},
	},

	"vault_config_exclude_list": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			configPath, exists, patterns, err := vaultconfigsvc.ListExclude(rt)
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.VaultConfigExcludeListResult{
				ConfigPath: configPath,
				Exists:     exists,
				Exclude:    patterns,
			}, &commandexec.Meta{Count: len(patterns)})
		},
	},

	"vault_config_exclude_add": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			mutation, pattern, patterns, err := vaultconfigsvc.AddExclude(rt, stringArg(req.Args, "pattern"))
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
		},
	},

	"vault_config_exclude_remove": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			mutation, removed, patterns, err := vaultconfigsvc.RemoveExclude(rt, stringArg(req.Args, "pattern"))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.VaultConfigExcludeRemoveResult{
				ConfigPath: mutation.ConfigPath,
				Changed:    mutation.Changed,
				Removed:    removed,
				Exclude:    patterns,
			}, nil)
		},
	},

	"vault_config_directories_get": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			configPath, exists, directories, err := vaultconfigsvc.GetDirectories(rt)
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(vaultDirectoriesData(configPath, exists, directories), nil)
		},
	},

	"vault_config_directories_set": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
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
		},
	},

	"vault_config_directories_unset": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
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
		},
	},

	"vault_config_capture_get": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			configPath, exists, configured, capture, err := vaultconfigsvc.GetCapture(rt)
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(vaultCaptureData(configPath, exists, configured, capture), nil)
		},
	},

	"vault_config_capture_set": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
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
		},
	},

	"vault_config_capture_unset": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			mutation, configured, capture, err := vaultconfigsvc.UnsetCapture(rt,
				boolArg(req.Args, "destination"),
				boolArg(req.Args, "heading"))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			data := vaultCaptureData(mutation.ConfigPath, true, configured, capture)
			data.Changed = &mutation.Changed
			return commandexec.Success(data, nil)
		},
	},

	"vault_config_deletion_get": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			configPath, exists, configured, deletion, err := vaultconfigsvc.GetDeletion(rt)
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(vaultDeletionData(configPath, exists, configured, deletion), nil)
		},
	},

	"vault_config_deletion_set": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
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
		},
	},

	"vault_config_deletion_unset": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			mutation, configured, deletion, err := vaultconfigsvc.UnsetDeletion(rt,
				boolArg(req.Args, "behavior"),
				boolArg(req.Args, "trash-dir"))
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			data := vaultDeletionData(mutation.ConfigPath, true, configured, deletion)
			data.Changed = &mutation.Changed
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
