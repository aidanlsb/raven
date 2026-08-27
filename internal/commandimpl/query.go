package commandimpl

import (
	"context"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/querysvc"
)

// HandleQuery executes the canonical `query` command path.
func HandleQuery(ctx context.Context, req commandexec.Request) (out commandexec.Result) {
	start := time.Now()
	rt, failure := newConfigCommandVaultRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	applyArgs := keyValuePairs(req.Args["apply"])
	limit, _ := intArg(req.Args, "limit")
	offset, _ := intArg(req.Args, "offset")
	result, err := querysvc.Run(rt, querysvc.ExecuteRequest{
		QueryString: stringArg(req.Args, "query_string"),
		Inputs:      keyValuePairs(req.Args["inputs"]),
		Refresh:     boolArg(req.Args, "refresh"),
		IDsOnly:     boolArg(req.Args, "ids"),
		Limit:       limit,
		Offset:      offset,
		CountOnly:   boolArg(req.Args, "count-only"),
		Apply:       applyArgs,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	queryTimeMs := time.Since(start).Milliseconds()
	if result.ApplyPlan != nil {
		out = handleQueryApply(ctx, req, result.ApplyPlan, queryTimeMs)
	} else {
		out = shapeQueryResult(result.Query, queryResultShapeOptions{
			IDsOnly:        result.IDsOnly,
			CountOnly:      result.CountOnly,
			IsSavedQuery:   result.IsSaved,
			SavedQueryName: result.SavedName,
			QueryTimeMs:    queryTimeMs,
		})
	}
	if out.OK {
		out.Warnings = append(queryCommandWarnings(result.Warnings), out.Warnings...)
	}
	return out
}

func queryCommandWarnings(serviceWarnings []querysvc.Warning) []commandexec.Warning {
	warnings := make([]commandexec.Warning, 0, len(serviceWarnings))
	for _, warning := range serviceWarnings {
		warnings = append(warnings, commandexec.Warning{
			Code: warning.Code, Message: warning.Message, Ref: warning.Ref,
		})
	}
	return warnings
}
