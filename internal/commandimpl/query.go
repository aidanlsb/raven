package commandimpl

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/bulkops"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/querysvc"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/schema"
)

// HandleQuery executes the canonical `query` command path.
func HandleQuery(ctx context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	queryString := strings.TrimSpace(stringArg(req.Args, "query_string"))
	if queryString == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "specify a query string", nil, "Run 'rvn query saved list' to see saved queries")
	}

	applyArgs := keyValuePairs(req.Args["apply"])

	resolvedQuery, queryName, isSavedQuery, err := resolveQueryString(queryString, req.Args["inputs"], vaultCfg)
	if err != nil {
		return mapQuerySvcFailure(err)
	}

	if isSavedQuery && !isFullQueryString(resolvedQuery) {
		return commandexec.Failure("QUERY_INVALID", fmt.Sprintf("saved query '%s' must start with 'object:' or 'trait:'", queryName), nil, "")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return commandexec.Failure("SCHEMA_INVALID", "failed to load schema", nil, "Fix schema.yaml and try again")
	}
	db, err := index.Open(vaultPath)
	if err != nil {
		return commandexec.Failure("DATABASE_ERROR", "failed to open database", nil, "Run 'rvn reindex' to rebuild the database")
	}
	defer db.Close()
	db.SetDailyDirectory(vaultCfg.GetDailyDirectory())

	rt := &readsvc.Runtime{
		VaultPath: vaultPath,
		VaultCfg:  vaultCfg,
		Schema:    sch,
		DB:        db,
	}

	if boolArg(req.Args, "refresh") {
		if _, err := readsvc.SmartReindex(rt); err != nil {
			return commandexec.Failure("DATABASE_ERROR", fmt.Sprintf("failed to refresh index: %v", err), nil, "Run 'rvn reindex' to rebuild the database")
		}
	} else {
		_, _, _ = readsvc.CheckStaleness(rt)
	}

	limit, _ := intArg(req.Args, "limit")
	offset, _ := intArg(req.Args, "offset")
	idsOnly := boolArg(req.Args, "ids")
	countOnly := boolArg(req.Args, "count-only")

	if limit < 0 {
		return commandexec.Failure("INVALID_INPUT", "--limit must be >= 0", nil, "Use --limit 0 for no limit")
	}
	if offset < 0 {
		return commandexec.Failure("INVALID_INPUT", "--offset must be >= 0", nil, "Use --offset 0 for no offset")
	}
	if len(applyArgs) > 0 && (limit > 0 || offset > 0 || countOnly) {
		return commandexec.Failure(
			"INVALID_INPUT",
			"--limit, --offset, and --count-only cannot be used with --apply",
			nil,
			"Remove pagination/count-only flags when using --apply",
		)
	}

	result, err := readsvc.ExecuteQuery(rt, readsvc.ExecuteQueryRequest{
		QueryString: resolvedQuery,
		IDsOnly:     idsOnly,
		Limit:       limit,
		Offset:      offset,
		CountOnly:   countOnly,
	})
	if err != nil {
		return mapExecuteQueryFailure(resolvedQuery, err)
	}

	if len(applyArgs) > 0 {
		return handleQueryApply(ctx, req, result, applyArgs, time.Since(start).Milliseconds())
	}

	meta := &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()}
	if countOnly {
		meta.Count = result.Total
		key := "type"
		if result.QueryType == "trait" {
			key = "trait"
		}
		return commandexec.Success(map[string]interface{}{
			"query_type": result.QueryType,
			key:          result.TypeName,
			"total":      result.Total,
		}, meta)
	}

	if idsOnly {
		meta.Count = result.Returned
		return commandexec.Success(map[string]interface{}{
			"ids":      result.IDs,
			"total":    result.Total,
			"returned": result.Returned,
			"offset":   result.Offset,
			"limit":    result.Limit,
		}, meta)
	}

	if result.QueryType == "object" {
		meta.Count = result.Returned
		data := map[string]interface{}{
			"query_type": "object",
			"items":      objectQueryItems(result),
			"total":      result.Total,
			"returned":   result.Returned,
			"offset":     result.Offset,
			"limit":      result.Limit,
		}
		if isSavedQuery && queryName != "" {
			data["saved_query"] = queryName
		} else {
			data["type"] = result.TypeName
		}
		return commandexec.Success(data, meta)
	}

	meta.Count = result.Returned
	data := map[string]interface{}{
		"query_type": "trait",
		"items":      traitQueryItems(result),
		"total":      result.Total,
		"returned":   result.Returned,
		"offset":     result.Offset,
		"limit":      result.Limit,
	}
	if isSavedQuery && queryName != "" {
		data["saved_query"] = queryName
	} else {
		data["trait"] = result.TypeName
	}
	return commandexec.Success(data, meta)
}

func handleQueryApply(ctx context.Context, req commandexec.Request, result *readsvc.ExecuteQueryResult, applyArgs []string, queryTimeMs int64) commandexec.Result {
	rawApply, err := bulkops.ParseRawApply(applyArgs)
	if err != nil {
		return mapBulkopsFailure(err)
	}

	if result.QueryType == "trait" {
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

	ids := make([]string, 0, len(result.Objects))
	for _, row := range result.Objects {
		ids = append(ids, row.ID)
	}
	ids = dedupeQueryApplyIDs(ids)
	if len(ids) == 0 {
		return commandexec.Success(map[string]interface{}{
			"preview": !req.Confirm && !boolArg(req.Args, "confirm"),
			"action":  rawApply.Command,
			"items":   []interface{}{},
			"total":   0,
		}, &commandexec.Meta{Count: 0, QueryTimeMs: queryTimeMs})
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
			"object_ids": stringsToInterfaces(plan.IDs),
		}, queryTimeMs)
	case bulkops.ObjectApplyDelete:
		return invokeNestedCommand(ctx, req, "delete", map[string]interface{}{
			"stdin":      true,
			"object_ids": stringsToInterfaces(plan.IDs),
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
		Confirm:        req.Confirm || boolArg(req.Args, "confirm"),
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
	return commandexec.Failure(string(bulkErr.Code), bulkErr.Message, nil, bulkErr.Suggestion)
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

func resolveQueryString(queryString string, rawInputs interface{}, vaultCfg *config.VaultConfig) (resolved, queryName string, isSaved bool, err error) {
	if vaultCfg == nil || len(vaultCfg.Queries) == 0 {
		return queryString, "", false, nil
	}

	trimmed := strings.TrimSpace(queryString)
	var tokens []string
	if strings.ContainsAny(trimmed, " \t\r\n") {
		if parts, ok := querysvc.SplitInlineInvocation(trimmed); ok {
			tokens = parts
		} else {
			tokens = strings.Fields(trimmed)
		}
	} else if trimmed != "" {
		tokens = []string{trimmed}
	}
	if len(tokens) == 0 {
		return "", "", false, fmt.Errorf("empty query string")
	}

	name := tokens[0]
	saved, ok := vaultCfg.Queries[name]
	if !ok {
		return queryString, "", false, nil
	}

	resolvedQuery, err := querysvc.ResolveSavedQuery(name, saved, tokens[1:], keyValuePairs(rawInputs))
	if err != nil {
		return "", "", true, err
	}
	return resolvedQuery, name, true, nil
}

func objectQueryItems(result *readsvc.ExecuteQueryResult) []map[string]interface{} {
	items := make([]map[string]interface{}, len(result.Objects))
	for i, row := range result.Objects {
		items[i] = map[string]interface{}{
			"num":       result.Offset + i + 1,
			"id":        row.ID,
			"type":      row.Type,
			"fields":    row.Fields,
			"file_path": row.FilePath,
			"line":      row.LineStart,
		}
	}
	return items
}

func traitQueryItems(result *readsvc.ExecuteQueryResult) []map[string]interface{} {
	items := make([]map[string]interface{}, len(result.Traits))
	for i, row := range result.Traits {
		items[i] = map[string]interface{}{
			"num":        result.Offset + i + 1,
			"id":         row.ID,
			"trait_type": row.TraitType,
			"value":      row.Value,
			"content":    row.Content,
			"file_path":  row.FilePath,
			"line":       row.Line,
			"object_id":  row.ParentObjectID,
		}
	}
	return items
}

func mapExecuteQueryFailure(queryString string, err error) commandexec.Result {
	var validationErr *query.ValidationError
	if errors.As(err, &validationErr) {
		return commandexec.Failure("QUERY_INVALID", validationErr.Message, nil, validationErr.Suggestion)
	}
	var executionErr *query.ExecutionError
	if errors.As(err, &executionErr) {
		return commandexec.Failure("QUERY_INVALID", executionErr.Message, nil, executionErr.Suggestion)
	}

	if _, parseErr := query.Parse(queryString); parseErr != nil {
		return commandexec.Failure("QUERY_INVALID", fmt.Sprintf("parse error: %v", parseErr), nil, "")
	}

	return commandexec.Failure("DATABASE_ERROR", err.Error(), nil, "Run 'rvn reindex' to rebuild the database")
}

func mapQuerySvcFailure(err error) commandexec.Result {
	svcErr, ok := querysvc.AsError(err)
	if !ok {
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
	}

	switch svcErr.Code {
	case querysvc.CodeInvalidInput:
		return commandexec.Failure("INVALID_INPUT", svcErr.Message, nil, svcErr.Suggestion)
	case querysvc.CodeQueryInvalid:
		return commandexec.Failure("QUERY_INVALID", svcErr.Message, nil, svcErr.Suggestion)
	case querysvc.CodeQueryNotFound:
		return commandexec.Failure("QUERY_NOT_FOUND", svcErr.Message, nil, svcErr.Suggestion)
	case querysvc.CodeConfigInvalid:
		return commandexec.Failure("CONFIG_INVALID", svcErr.Message, nil, svcErr.Suggestion)
	case querysvc.CodeFileWriteError:
		return commandexec.Failure("FILE_WRITE_ERROR", svcErr.Message, nil, svcErr.Suggestion)
	default:
		return commandexec.Failure("INTERNAL_ERROR", svcErr.Message, nil, svcErr.Suggestion)
	}
}

func isFullQueryString(queryString string) bool {
	return strings.HasPrefix(queryString, "object:") || strings.HasPrefix(queryString, "trait:")
}

// HandleQuerySavedList executes the canonical `query_saved_list` command.
func HandleQuerySavedList(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	result, err := querysvc.List(querysvc.ListRequest{VaultPath: vaultPath})
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
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	result, err := querysvc.Get(querysvc.GetRequest{
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
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	result, err := querysvc.Set(querysvc.SetRequest{
		VaultPath:   vaultPath,
		Name:        strings.TrimSpace(stringArg(req.Args, "name")),
		QueryString: strings.TrimSpace(stringArg(req.Args, "query_string")),
		Args:        stringSliceArg(req.Args["arg"]),
		Description: strings.TrimSpace(stringArg(req.Args, "description")),
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
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	result, err := querysvc.Remove(querysvc.RemoveRequest{
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
	return map[string]interface{}{
		"name":        q.Name,
		"query":       q.Query,
		"args":        q.Args,
		"description": q.Description,
	}
}
