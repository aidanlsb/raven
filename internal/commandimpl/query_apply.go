package commandimpl

import (
	"context"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/bulkops"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/readsvc"
)

// handleQueryApply turns canonical query results into one bulk mutation plan
// and delegates preview/apply execution to the same nested command for every
// transport.
func handleQueryApply(ctx context.Context, req commandexec.Request, result *readsvc.ExecuteQueryResult, applyArgs []string, queryTimeMs int64) commandexec.Result {
	if result.QueryKind == "link" {
		return commandexec.Failure(
			"INVALID_INPUT",
			fmt.Sprintf("--apply is not supported for %s queries", result.QueryKind),
			nil,
			"Use --ids and pass results to a compatible command",
		)
	}

	rawApply, err := bulkops.ParseRawApply(applyArgs)
	if err != nil {
		return mapBulkopsFailure(err)
	}
	if result.QueryKind == "section" && rawApply.Command != string(bulkops.ObjectApplyMove) {
		return commandexec.Failure(
			"INVALID_INPUT",
			fmt.Sprintf("--apply is not supported for %s queries", result.QueryKind),
			nil,
			"Use --ids and pass results to a compatible command",
		)
	}

	if result.QueryKind == "trait" {
		plan, err := bulkops.PlanTraitApply(rawApply, result.Traits)
		if err != nil {
			return mapBulkopsFailure(err)
		}
		return invokeNestedCommand(ctx, req, "update", map[string]interface{}{
			"stdin":     true,
			"value":     plan.NewValue,
			"trait_ids": traitIDsToInterfaces(result.Traits),
		}, queryTimeMs)
	}

	ids := make([]string, 0, len(result.Objects)+len(result.Sections))
	if result.QueryKind == "section" {
		for _, row := range result.Sections {
			ids = append(ids, row.ID)
		}
	} else {
		for _, row := range result.Objects {
			ids = append(ids, row.ID)
		}
	}
	ids = dedupeQueryApplyIDs(ids)
	if len(ids) == 0 {
		// No matches: nothing is delegated to a nested command, so set the
		// phase here to keep query --apply responses uniform.
		phase := commandexec.MutationPhaseApplied
		if !req.Confirm {
			phase = commandexec.MutationPhasePreview
		}
		return commandexec.Success(commandpayload.QueryApplyEmptyResult{
			Preview: !req.Confirm,
			Action:  rawApply.Command,
			Items:   []interface{}{},
			Total:   0,
		}, &commandexec.Meta{Count: 0, QueryTimeMs: queryTimeMs}).WithMutationPhase(phase)
	}

	plan, err := bulkops.PlanObjectApply(rawApply, ids)
	if err != nil {
		return mapBulkopsFailure(err)
	}

	switch plan.Command {
	case bulkops.ObjectApplySet:
		return invokeNestedCommand(ctx, req, "set", map[string]interface{}{
			"stdin":      true,
			"fields":     plan.SetUpdates,
			"references": stringsToInterfaces(plan.IDs),
		}, queryTimeMs)
	case bulkops.ObjectApplyDelete:
		return invokeNestedCommand(ctx, req, "delete", map[string]interface{}{
			"stdin":      true,
			"references": stringsToInterfaces(plan.IDs),
		}, queryTimeMs)
	case bulkops.ObjectApplyAdd:
		return invokeNestedCommand(ctx, req, "add", map[string]interface{}{
			"stdin":      true,
			"text":       plan.AddText,
			"object_ids": stringsToInterfaces(plan.IDs),
		}, queryTimeMs)
	case bulkops.ObjectApplyMove:
		return invokeNestedCommand(ctx, req, "move", map[string]interface{}{
			"stdin":       true,
			"destination": plan.MoveDestination,
			"update-refs": true,
			"object_ids":  stringsToInterfaces(plan.IDs),
		}, queryTimeMs)
	default:
		return commandexec.Failure(
			"INVALID_INPUT",
			fmt.Sprintf("unknown apply command: %s", plan.Command),
			nil,
			"Supported commands: set, delete, add, move",
		)
	}
}

func invokeNestedCommand(ctx context.Context, req commandexec.Request, commandID string, args map[string]interface{}, queryTimeMs int64) commandexec.Result {
	invoker, ok := commandexec.InvokerFromContext(ctx)
	if !ok {
		return commandexec.Failure("INTERNAL_ERROR", "query apply runtime is unavailable", nil, "Retry the command")
	}

	result := invoker.Execute(ctx, commandexec.Request{
		CommandID:      commandID,
		VaultPath:      req.VaultPath,
		ConfigPath:     req.ConfigPath,
		StatePath:      req.StatePath,
		ExecutablePath: req.ExecutablePath,
		Caller:         req.Caller,
		Args:           args,
		Confirm:        req.Confirm,
	})

	if result.Meta == nil {
		result.Meta = &commandexec.Meta{}
	}
	result.Meta.QueryTimeMs = queryTimeMs
	return result
}

func stringsToInterfaces(values []string) []interface{} {
	out := make([]interface{}, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func traitIDsToInterfaces(items []model.Trait) []interface{} {
	out := make([]interface{}, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func mapBulkopsFailure(err error) commandexec.Result {
	bulkErr, ok := bulkops.AsError(err)
	if !ok {
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
	}
	return commandexec.Failure(bulkErr.Code, bulkErr.Message, nil, bulkErr.Suggestion)
}

func dedupeQueryApplyIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
