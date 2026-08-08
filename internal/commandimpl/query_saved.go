package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/querysvc"
)

// HandleQuerySavedList executes the canonical `query_saved_list` command.
func HandleQuerySavedList(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	rt, failure := newLazyConfigCommandRuntime(vaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	result, err := querysvc.List(rt, querysvc.ListRequest{VaultPath: vaultPath})
	if err != nil {
		return mapQuerySvcFailure(err)
	}

	queries := make([]map[string]interface{}, 0, len(result.Queries))
	for _, savedQuery := range result.Queries {
		queries = append(queries, savedQueryData(savedQuery))
	}
	return commandexec.Success(map[string]interface{}{
		"queries": queries,
	}, &commandexec.Meta{Count: len(queries)})
}

// HandleQuerySavedGet executes the canonical `query_saved_get` command.
func HandleQuerySavedGet(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	rt, failure := newLazyConfigCommandRuntime(vaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	result, err := querysvc.Get(rt, querysvc.GetRequest{
		VaultPath: vaultPath,
		Name:      strings.TrimSpace(stringArg(req.Args, "name")),
	})
	if err != nil {
		return mapQuerySvcFailure(err)
	}

	return commandexec.Success(savedQueryData(result.Query), nil)
}

// HandleQuerySavedSet executes the canonical `query_saved_set` command.
func HandleQuerySavedSet(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	rt, failure := newLazyConfigCommandRuntime(vaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	result, err := querysvc.Set(rt, querysvc.SetRequest{
		VaultPath:   vaultPath,
		Name:        strings.TrimSpace(stringArg(req.Args, "name")),
		QueryString: strings.TrimSpace(stringArg(req.Args, "query_string")),
		Args:        stringSliceArg(req.Args["arg"]),
		Description: strings.TrimSpace(stringArg(req.Args, "description")),
		Options:     savedQueryOptionsFromArgs(req.Args),
	})
	if err != nil {
		return mapQuerySvcFailure(err)
	}

	data := savedQueryData(result.Query)
	data["status"] = string(result.Status)
	return commandexec.Success(data, nil)
}

// HandleQuerySavedRemove executes the canonical `query_saved_remove` command.
func HandleQuerySavedRemove(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	rt, failure := newLazyConfigCommandRuntime(vaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	result, err := querysvc.Remove(rt, querysvc.RemoveRequest{
		VaultPath: vaultPath,
		Name:      strings.TrimSpace(stringArg(req.Args, "name")),
	})
	if err != nil {
		return mapQuerySvcFailure(err)
	}

	return commandexec.Success(map[string]interface{}{
		"name":    result.Name,
		"removed": result.Removed,
	}, nil)
}

func savedQueryData(q querysvc.SavedQueryInfo) map[string]interface{} {
	return q.Payload()
}

func savedQueryOptionsFromArgs(args map[string]interface{}) *config.QueryOptions {
	if args == nil {
		return nil
	}
	if raw, ok := args["options"]; ok {
		return savedQueryOptionsFromRaw(raw)
	}

	opts := &config.QueryOptions{}
	if v, ok := boolPointerArg(args, "refresh"); ok {
		opts.Refresh = v
	}
	if v, ok := boolPointerArg(args, "ids"); ok {
		opts.IDs = v
	}
	if v, ok := intPointerArg(args, "limit"); ok {
		opts.Limit = v
	}
	if v, ok := intPointerArg(args, "offset"); ok {
		opts.Offset = v
	}
	if v, ok := boolPointerArg(args, "count-only"); ok {
		opts.CountOnly = v
	}
	if _, ok := args["apply"]; ok {
		opts.Apply = stringSliceArg(args["apply"])
	}
	if v, ok := boolPointerArg(args, "confirm"); ok {
		opts.Confirm = v
	}
	if v, ok := boolPointerArg(args, "pipe"); ok {
		opts.Pipe = v
	} else if v, ok := boolPointerArg(args, "no-pipe"); ok && *v {
		pipe := false
		opts.Pipe = &pipe
	}
	if v, ok := boolPointerArg(args, "browse"); ok {
		opts.Browse = v
	}
	if opts.IsEmpty() {
		return nil
	}
	return opts
}

func savedQueryOptionsFromRaw(raw interface{}) *config.QueryOptions {
	switch v := raw.(type) {
	case nil:
		return nil
	case *config.QueryOptions:
		if v.IsEmpty() {
			return nil
		}
		out := *v
		out.Apply = append([]string(nil), v.Apply...)
		return &out
	case config.QueryOptions:
		if v.IsEmpty() {
			return nil
		}
		out := v
		out.Apply = append([]string(nil), v.Apply...)
		return &out
	case map[string]interface{}:
		opts := &config.QueryOptions{}
		if v, ok := boolPointerRaw(v["refresh"]); ok {
			opts.Refresh = v
		}
		if v, ok := boolPointerRaw(v["ids"]); ok {
			opts.IDs = v
		}
		if v, ok := intPointerRaw(v["limit"]); ok {
			opts.Limit = v
		}
		if v, ok := intPointerRaw(v["offset"]); ok {
			opts.Offset = v
		}
		if v, ok := boolPointerRaw(v["count_only"]); ok {
			opts.CountOnly = v
		}
		if rawApply, ok := v["apply"]; ok {
			opts.Apply = stringSliceArg(rawApply)
		}
		if v, ok := boolPointerRaw(v["confirm"]); ok {
			opts.Confirm = v
		}
		if v, ok := boolPointerRaw(v["pipe"]); ok {
			opts.Pipe = v
		}
		if v, ok := boolPointerRaw(v["browse"]); ok {
			opts.Browse = v
		}
		if opts.IsEmpty() {
			return nil
		}
		return opts
	default:
		return nil
	}
}

func boolPointerArg(args map[string]interface{}, key string) (*bool, bool) {
	raw, ok := args[key]
	if !ok {
		return nil, false
	}
	return boolPointerRaw(raw)
}

func boolPointerRaw(raw interface{}) (*bool, bool) {
	switch v := raw.(type) {
	case bool:
		return &v, true
	case string:
		parsed := strings.EqualFold(v, "true")
		return &parsed, true
	default:
		return nil, false
	}
}

func intPointerArg(args map[string]interface{}, key string) (*int, bool) {
	raw, ok := args[key]
	if !ok {
		return nil, false
	}
	return intPointerRaw(raw)
}

func intPointerRaw(raw interface{}) (*int, bool) {
	switch v := raw.(type) {
	case int:
		return &v, true
	case int64:
		parsed := int(v)
		return &parsed, true
	case float64:
		parsed := int(v)
		return &parsed, true
	case float32:
		parsed := int(v)
		return &parsed, true
	default:
		return nil, false
	}
}
