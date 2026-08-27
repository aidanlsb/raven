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
		Args:           queryApplyCommandArgs(plan),
		Confirm:        req.Confirm,
	})
	if result.Meta == nil {
		result.Meta = &commandexec.Meta{}
	}
	result.Meta.QueryTimeMs = queryTimeMs
	return result
}

func queryApplyCommandArgs(plan *querysvc.ApplyPlan) map[string]interface{} {
	switch plan.Command {
	case "update":
		return map[string]interface{}{
			"stdin": true, "value": plan.NewValue, "trait_ids": stringsToInterfaces(plan.TraitIDs),
		}
	case "set":
		return map[string]interface{}{
			"stdin": true, "fields": plan.SetUpdates, "references": stringsToInterfaces(plan.IDs),
		}
	case "delete":
		return map[string]interface{}{"stdin": true, "references": stringsToInterfaces(plan.IDs)}
	case "add":
		return map[string]interface{}{
			"stdin": true, "text": plan.AddText, "object_ids": stringsToInterfaces(plan.IDs),
		}
	case "move":
		return map[string]interface{}{
			"stdin": true, "destination": plan.MoveDestination,
			"update-refs": true, "object_ids": stringsToInterfaces(plan.IDs),
		}
	default:
		return nil
	}
}

func stringsToInterfaces(values []string) []interface{} {
	out := make([]interface{}, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
