package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
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
		return commandexec.FromServiceError(err)
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
		return commandexec.FromServiceError(err)
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
	})
	if err != nil {
		return commandexec.FromServiceError(err)
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
		return commandexec.FromServiceError(err)
	}

	return commandexec.Success(map[string]interface{}{
		"name":    result.Name,
		"removed": result.Removed,
	}, nil)
}

func savedQueryData(q querysvc.SavedQueryInfo) map[string]interface{} {
	return q.Payload()
}
