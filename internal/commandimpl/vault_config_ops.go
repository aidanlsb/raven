package commandimpl

import (
	"context"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/vaultconfigsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
	"github.com/aidanlsb/raven/internal/vaultsession"
)

type vaultConfigOperation struct {
	Execute func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result
}

var vaultConfigOperationTable = map[string]vaultConfigOperation{
	"vault_config_auto_reindex_set": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.SetAutoReindex(rt, boolArgDefault(req.Args, "value", true)))
		},
	},
	"vault_config_auto_reindex_unset": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.UnsetAutoReindex(rt))
		},
	},
	"vault_config_protected_prefixes_list": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.ListProtectedPrefixes(rt)
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(*result, &commandexec.Meta{Count: len(result.ProtectedPrefixes)})
		},
	},
	"vault_config_protected_prefixes_add": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.AddProtectedPrefix(rt, stringArg(req.Args, "prefix")))
		},
	},
	"vault_config_protected_prefixes_remove": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.RemoveProtectedPrefix(rt, stringArg(req.Args, "prefix")))
		},
	},
	"vault_config_exclude_list": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			result, err := vaultconfigsvc.ListExclude(rt)
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(*result, &commandexec.Meta{Count: len(result.Exclude)})
		},
	},
	"vault_config_exclude_add": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.AddExclude(rt, stringArg(req.Args, "pattern")))
		},
	},
	"vault_config_exclude_remove": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.RemoveExclude(rt, stringArg(req.Args, "pattern")))
		},
	},
	"vault_config_directories_get": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.GetDirectories(rt))
		},
	},
	"vault_config_directories_set": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.SetDirectories(rt,
				optionalStringArg(req.Args, "daily"),
				optionalStringArg(req.Args, "type"),
				optionalStringArg(req.Args, "page"),
				optionalStringArg(req.Args, "template")))
		},
	},
	"vault_config_directories_unset": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.UnsetDirectories(rt,
				boolArg(req.Args, "daily"),
				boolArg(req.Args, "type"),
				boolArg(req.Args, "page"),
				boolArg(req.Args, "template")))
		},
	},
	"vault_config_capture_get": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.GetCapture(rt))
		},
	},
	"vault_config_capture_set": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.SetCapture(rt,
				optionalStringArg(req.Args, "destination"),
				optionalStringArg(req.Args, "heading")))
		},
	},
	"vault_config_capture_unset": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.UnsetCapture(rt,
				boolArg(req.Args, "destination"),
				boolArg(req.Args, "heading")))
		},
	},
	"vault_config_deletion_get": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.GetDeletion(rt))
		},
	},
	"vault_config_deletion_set": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.SetDeletion(rt,
				optionalStringArg(req.Args, "behavior"),
				optionalStringArg(req.Args, "trash-dir")))
		},
	},
	"vault_config_deletion_unset": {
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request) commandexec.Result {
			return vaultConfigSuccess(vaultconfigsvc.UnsetDeletion(rt,
				boolArg(req.Args, "behavior"),
				boolArg(req.Args, "trash-dir")))
		},
	},
}

func handleVaultConfigOperation(_ context.Context, req commandexec.Request, commandID string) commandexec.Result {
	op, exists := vaultConfigOperationTable[commandID]
	if !exists {
		return commandexec.Failure("INVALID_INPUT", "unknown vault config operation: "+commandID, nil, "")
	}

	sess, failure := openSession(req.VaultPath, vaultsession.Lazy())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	return op.Execute(sess.Runtime(), req)
}

func vaultConfigSuccess[T any](result *T, err error) commandexec.Result {
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(*result, nil)
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
