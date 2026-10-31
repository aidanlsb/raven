package commandimpl

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func newIndexReadRuntime(vaultPath string, args map[string]any) (*vaultruntime.Runtime, []commandexec.Warning, commandexec.Result) {
	rt, failure := newReadRuntime(vaultPath, vaultruntime.Options{})
	if failure.Error != nil {
		return nil, nil, failure
	}
	warnings, refreshFailure := refreshIndexIfNeeded(rt, args)
	if refreshFailure.Error != nil {
		rt.Close()
		return nil, nil, refreshFailure
	}
	if openFailure := openIndexDB(rt); openFailure.Error != nil {
		rt.Close()
		return nil, nil, openFailure
	}
	return rt, warnings, commandexec.Result{}
}

func shouldRefreshIndex(rt *vaultruntime.Runtime, args map[string]any) bool {
	var refresh *bool
	if _, ok := args["refresh"]; ok {
		value := boolArg(args, "refresh")
		refresh = &value
	}
	return readsvc.ShouldRefresh(rt, boolArg(args, "no-refresh"), refresh)
}

func refreshIndexIfNeeded(rt *vaultruntime.Runtime, args map[string]any) ([]commandexec.Warning, commandexec.Result) {
	if !shouldRefreshIndex(rt, args) || !readsvc.CanRefresh(rt) {
		return nil, commandexec.Result{}
	}
	report, err := readsvc.EnsureIndexFresh(rt)
	if err != nil {
		return nil, mapIndexRefreshFailure(err)
	}
	return indexRefreshCommandWarnings(report), commandexec.Result{}
}

func openIndexDB(rt *vaultruntime.Runtime) commandexec.Result {
	if rt == nil {
		return commandexec.Failure("INTERNAL_ERROR", "vault runtime is required", nil, "")
	}
	if rt.DB != nil {
		return commandexec.Result{}
	}
	if err := rt.OpenDB(); err != nil {
		return mapVaultRuntimeSetupFailure(err)
	}
	return commandexec.Result{}
}

func mapIndexRefreshFailure(err error) commandexec.Result {
	if err == nil {
		return commandexec.Failure("DATABASE_ERROR", "failed to refresh index", nil, "Run 'rvn reindex' to rebuild the database")
	}
	if _, ok := svcerr.AsError(err); ok {
		return commandexec.FromServiceError(err)
	}
	if failure, ok := mapIndexRebuildRequired(err); ok {
		return failure
	}
	return commandexec.Failure(
		"DATABASE_ERROR",
		fmt.Sprintf("failed to refresh index: %v", err),
		nil,
		"Run 'rvn reindex' to rebuild the database",
	)
}

func indexRefreshCommandWarnings(report readsvc.SmartReindexReport) []commandexec.Warning {
	issues := readsvc.RefreshWarnings(report)
	if len(issues) == 0 {
		return nil
	}
	warnings := make([]commandexec.Warning, 0, len(issues))
	for _, issue := range issues {
		warnings = append(warnings, commandexec.Warning{
			Code: issue.Code, Message: issue.Message, Ref: issue.Ref,
		})
	}
	return warnings
}

func withIndexRefreshWarnings(warnings []commandexec.Warning, result commandexec.Result) commandexec.Result {
	if len(warnings) == 0 || !result.OK {
		return result
	}
	result.Warnings = append(warnings, result.Warnings...)
	return result
}
