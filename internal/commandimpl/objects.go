package commandimpl

import (
	"context"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/readsvc"
)

// HandleObjects executes the canonical `objects` command: a cheap index-backed
// listing of every file-backed object without bodies.
func HandleObjects(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	rt, failure := newConfigCommandVaultRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	limit, _ := intArg(req.Args, "limit")
	offset, _ := intArg(req.Args, "offset")
	result, err := readsvc.ListObjects(rt, limit, offset)
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	items := make([]commandpayload.ObjectSummaryItem, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, commandpayload.ObjectSummaryItem{
			ID:       item.ID,
			Type:     item.Type,
			Name:     item.Name,
			FilePath: item.FilePath,
		})
	}
	returned := len(items)
	paging := commandpayload.Pagination{
		Total:    result.Total,
		Returned: returned,
		Offset:   result.Offset,
		Limit:    result.Limit,
		HasMore:  result.Offset+returned < result.Total,
	}
	if paging.HasMore {
		next := result.Offset + returned
		paging.NextOffset = &next
	}

	out := commandexec.Success(commandpayload.ObjectsResult{
		Items:      items,
		Pagination: paging,
	}, &commandexec.Meta{Count: returned, QueryTimeMs: time.Since(start).Milliseconds()})
	return withSchemaLoadWarning(rt, out)
}
