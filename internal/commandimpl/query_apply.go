package commandimpl

import (
	"context"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/querysvc"
)

// handleQueryApply performs the transport-owned nested command dispatch for a
// mutation plan already validated and built by querysvc.
func handleQueryApply(ctx context.Context, req commandexec.Request, plan *querysvc.ApplyPlan, queryTimeMs int64) commandexec.Result {
	if plan.Empty {
		phase := commandexec.MutationPhaseApplied
		if !req.Confirm {
			phase = commandexec.MutationPhasePreview
		}
		return commandexec.Success(commandpayload.QueryApplyEmptyResult{
			Preview: !req.Confirm,
			Action:  plan.Command,
			Items:   []interface{}{},
			Total:   0,
		}, &commandexec.Meta{Count: 0, QueryTimeMs: queryTimeMs}).WithMutationPhase(phase)
	}

	invoker, ok := commandexec.InvokerFromContext(ctx)
	if !ok {
		return commandexec.Failure("INTERNAL_ERROR", "query apply runtime is unavailable", nil, "Retry the command")
	}
	result := invoker.Execute(ctx, commandexec.Request{
		CommandID:      plan.Command,
		VaultPath:      req.VaultPath,
		ConfigPath:     req.ConfigPath,
		StatePath:      req.StatePath,
		ExecutablePath: req.ExecutablePath,
		Caller:         req.Caller,
		Args:           plan.Args,
		Confirm:        req.Confirm,
	})
	if result.Meta == nil {
		result.Meta = &commandexec.Meta{}
	}
	result.Meta.QueryTimeMs = queryTimeMs
	return result
}
