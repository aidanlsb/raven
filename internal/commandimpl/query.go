package commandimpl

import (
	"context"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
)

// HandleQuery executes the canonical `query` command path.
func HandleQuery(ctx context.Context, req commandexec.Request) (out commandexec.Result) {
	start := time.Now()
	execution, failure := prepareQueryExecution(req)
	if failure.Error != nil {
		return failure
	}
	defer execution.Close()
	defer func() {
		if out.OK && len(execution.refreshWarnings) > 0 {
			out.Warnings = append(append([]commandexec.Warning{}, execution.refreshWarnings...), out.Warnings...)
		}
	}()

	applyArgs := keyValuePairs(req.Args["apply"])
	options, failure := queryExecutionOptionsFromRequest(req, len(applyArgs) > 0)
	if failure.Error != nil {
		return failure
	}
	result, failure := execution.Execute(options)
	if failure.Error != nil {
		return failure
	}

	queryTimeMs := time.Since(start).Milliseconds()
	if len(applyArgs) > 0 {
		return handleQueryApply(ctx, req, result, applyArgs, queryTimeMs)
	}

	return shapeQueryResult(result, queryResultShapeOptions{
		IDsOnly:        options.IDsOnly,
		CountOnly:      options.CountOnly,
		IsSavedQuery:   execution.isSavedQuery,
		SavedQueryName: execution.queryName,
		QueryTimeMs:    queryTimeMs,
	})
}
