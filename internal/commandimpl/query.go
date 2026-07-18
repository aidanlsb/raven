package commandimpl

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/bulkops"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/querysvc"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/schema"
)

// HandleQuery executes the canonical `query` command path.
func HandleQuery(ctx context.Context, req commandexec.Request) (out commandexec.Result) {
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

	if isSavedQuery && !querysvc.IsFullQueryRoot(resolvedQuery) {
		return commandexec.Failure("QUERY_INVALID", fmt.Sprintf("saved query '%s' must start with 'type:', 'trait:', 'section', or 'asset'", queryName), nil, "")
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

	// A non-saved string that is not a concrete query root is an unknown query.
	// Return a rich, resolver-aware hint here (rather than letting execution fail
	// with a generic parse error) so both CLI and MCP callers get the same help.
	if !isSavedQuery && !querysvc.IsFullQueryRoot(resolvedQuery) {
		suggestion := buildUnknownQuerySuggestion(db, resolvedQuery, vaultCfg.GetDailyDirectory(), sch)
		return commandexec.Failure(codes.ErrQueryInvalid, "unknown query: "+resolvedQuery, nil, suggestion)
	}

	compatible, err := db.SchemaCompatible()
	if err != nil {
		return commandexec.Failure(codes.ErrDatabase, "failed to read index schema version", nil, "Run 'rvn reindex --full' to rebuild the index")
	}
	if !compatible {
		return commandexec.Failure(codes.ErrDatabaseVersion, "index schema is stale or incompatible", nil, "Run 'rvn reindex --full' to rebuild the index")
	}

	rt := &readsvc.Runtime{
		VaultPath: vaultPath,
		VaultCfg:  vaultCfg,
		Schema:    sch,
		DB:        db,
	}

	var refreshWarnings []commandexec.Warning
	defer func() {
		if out.OK && len(refreshWarnings) > 0 {
			out.Warnings = append(append([]commandexec.Warning{}, refreshWarnings...), out.Warnings...)
		}
	}()

	if boolArg(req.Args, "refresh") {
		report, err := readsvc.SmartReindex(rt)
		if err != nil {
			return commandexec.Failure("DATABASE_ERROR", fmt.Sprintf("failed to refresh index: %v", err), nil, "Run 'rvn reindex' to rebuild the database")
		}
		refreshWarnings = append(refreshFailureWarnings(report), refreshUnknownFieldWarnings(report)...)
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
		payload := commandpayload.QueryCountResult{
			QueryKind: result.QueryKind,
			Total:     result.Total,
		}
		switch result.QueryKind {
		case "trait":
			payload.Trait = result.TypeName
		case "asset", "section":
			// Asset and section counts carry no type/trait discriminator.
		default:
			payload.Type = result.TypeName
		}
		return commandexec.Success(payload, meta)
	}

	if idsOnly {
		meta.Count = result.Returned
		return commandexec.Success(commandpayload.QueryIDsResult{
			IDs:        result.IDs,
			Pagination: queryPagination(result),
		}, meta)
	}

	if result.QueryKind == "type" {
		meta.Count = result.Returned
		payload := commandpayload.QueryObjectResult{
			QueryKind:  "type",
			Items:      objectQueryItems(result),
			Pagination: queryPagination(result),
		}
		if isSavedQuery && queryName != "" {
			payload.SavedQuery = queryName
		} else {
			payload.Type = result.TypeName
		}
		return commandexec.Success(payload, meta)
	}

	if result.QueryKind == "asset" {
		meta.Count = result.Returned
		payload := commandpayload.QueryAssetResult{
			QueryKind:  "asset",
			Items:      assetQueryItems(result),
			Pagination: queryPagination(result),
		}
		if isSavedQuery && queryName != "" {
			payload.SavedQuery = queryName
		}
		return commandexec.Success(payload, meta)
	}

	if result.QueryKind == "section" {
		meta.Count = result.Returned
		payload := commandpayload.QuerySectionResult{
			QueryKind:  "section",
			Items:      sectionQueryItems(result),
			Pagination: queryPagination(result),
		}
		if isSavedQuery && queryName != "" {
			payload.SavedQuery = queryName
		}
		return commandexec.Success(payload, meta)
	}

	meta.Count = result.Returned
	payload := commandpayload.QueryTraitResult{
		QueryKind:  "trait",
		Items:      traitQueryItems(result),
		Pagination: queryPagination(result),
	}
	if isSavedQuery && queryName != "" {
		payload.SavedQuery = queryName
	} else {
		payload.Trait = result.TypeName
	}
	return commandexec.Success(payload, meta)
}

// queryPagination builds the shared paging affordances for a query response.
// `has_more` is always present alongside total/returned/offset/limit so agents
// can loop without guessing. `next_offset` is a forward cursor included only
// when more results remain. For unlimited queries (the default) total equals
// returned, so has_more is false and no next_offset is emitted.
func queryPagination(result *readsvc.ExecuteQueryResult) commandpayload.Pagination {
	paging := commandpayload.Pagination{
		Total:    result.Total,
		Returned: result.Returned,
		Offset:   result.Offset,
		Limit:    result.Limit,
		HasMore:  result.HasMore(),
	}
	if paging.HasMore {
		next := result.NextOffset()
		paging.NextOffset = &next
	}
	return paging
}

func handleQueryApply(ctx context.Context, req commandexec.Request, result *readsvc.ExecuteQueryResult, applyArgs []string, queryTimeMs int64) commandexec.Result {
	if result.QueryKind == "asset" || result.QueryKind == "section" {
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

	ids := make([]string, 0, len(result.Objects))
	for _, row := range result.Objects {
		ids = append(ids, row.ID)
	}
	ids = dedupeQueryApplyIDs(ids)
	if len(ids) == 0 {
		// No matches: nothing is delegated to a nested command, so set the
		// phase here to keep query --apply responses uniform.
		phase := commandexec.MutationPhaseApplied
		if !req.Confirm {
			phase = commandexec.MutationPhasePreview
		}
		return commandexec.Success(map[string]interface{}{
			"preview": !req.Confirm,
			"action":  rawApply.Command,
			"items":   []interface{}{},
			"total":   0,
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

func resolveQueryString(queryString string, rawInputs interface{}, vaultCfg *config.VaultConfig) (resolved, queryName string, isSaved bool, err error) {
	name, saved, inputTokens, matched := querysvc.MatchInvocation(vaultCfg, queryString)
	if !matched {
		return queryString, "", false, nil
	}

	resolvedQuery, err := querysvc.ResolveSavedQuery(name, saved, inputTokens, keyValuePairs(rawInputs))
	if err != nil {
		return "", "", true, err
	}
	return resolvedQuery, name, true, nil
}

func objectQueryItems(result *readsvc.ExecuteQueryResult) []commandpayload.ObjectItem {
	items := make([]commandpayload.ObjectItem, len(result.Objects))
	for i, row := range result.Objects {
		items[i] = commandpayload.ObjectItem{
			Num:      result.Offset + i + 1,
			ID:       row.ID,
			Type:     row.Type,
			Fields:   row.Fields,
			FilePath: row.FilePath,
			Line:     row.LineStart,
		}
	}
	return items
}

func traitQueryItems(result *readsvc.ExecuteQueryResult) []commandpayload.TraitItem {
	items := make([]commandpayload.TraitItem, len(result.Traits))
	for i, row := range result.Traits {
		items[i] = commandpayload.TraitItem{
			Num:       result.Offset + i + 1,
			ID:        row.ID,
			TraitType: row.TraitType,
			Value:     row.IndexValueString(),
			Content:   row.Content,
			FilePath:  row.FilePath,
			Line:      row.Line,
			ObjectID:  row.ParentObjectID,
		}
	}
	return items
}

func assetQueryItems(result *readsvc.ExecuteQueryResult) []commandpayload.AssetItem {
	items := make([]commandpayload.AssetItem, len(result.Assets))
	for i, row := range result.Assets {
		items[i] = commandpayload.AssetItem{
			Num:       result.Offset + i + 1,
			ID:        row.ID,
			FilePath:  row.FilePath,
			Filename:  row.Filename,
			Extension: row.Extension,
			MediaType: row.MediaType,
			SizeBytes: row.SizeBytes,
		}
	}
	return items
}

func sectionQueryItems(result *readsvc.ExecuteQueryResult) []commandpayload.SectionItem {
	items := make([]commandpayload.SectionItem, len(result.Sections))
	for i, row := range result.Sections {
		items[i] = commandpayload.SectionItem{
			Num:             result.Offset + i + 1,
			ID:              row.ID,
			FileObjectID:    row.FileObjectID,
			FilePath:        row.FilePath,
			Slug:            row.Slug,
			Title:           row.Title,
			Level:           row.Level,
			LineStart:       row.LineStart,
			LineEnd:         row.LineEnd,
			DirectLineEnd:   row.LineEnd,
			SubtreeLineEnd:  row.SubtreeLineEnd,
			ParentSectionID: row.ParentSectionID,
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
		return commandexec.Failure(
			"QUERY_INVALID",
			fmt.Sprintf("parse error: %v", parseErr),
			nil,
			queryParseSuggestion(queryString),
		)
	}

	return commandexec.Failure("DATABASE_ERROR", err.Error(), nil, "Run 'rvn reindex' to rebuild the database")
}

func queryParseSuggestion(queryString string) string {
	if hint, ok := querySyntaxHint(queryString); ok {
		return hint
	}
	return "Run 'rvn docs querying query-language' for RQL syntax and examples"
}

// querySyntaxHint returns a suggestion for a common RQL syntax mistake (single
// quotes, SQL-style where clauses) or ok=false when no specific hint applies.
// It is shared by the full-query parse-error path and the unknown-query hint so
// both give the same guidance for malformed query text.
func querySyntaxHint(queryString string) (string, bool) {
	trimmed := strings.TrimSpace(queryString)
	lower := strings.ToLower(trimmed)

	if strings.Contains(trimmed, "'") {
		return `RQL strings use double quotes, not single quotes. For example: .status=="open"`, true
	}
	if strings.Contains(" "+lower+" ", " where ") {
		return "RQL does not use 'where'. Put predicates directly after the query root, for example: type:issue .status==open", true
	}
	return "", false
}

func mapQuerySvcFailure(err error) commandexec.Result {
	return commandexec.FromServiceError(err)
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

func refreshFailureWarnings(report readsvc.SmartReindexReport) []commandexec.Warning {
	if len(report.Failures) == 0 {
		return nil
	}

	const maxListed = 5
	listed := make([]string, 0, maxListed)
	for i, failure := range report.Failures {
		if i >= maxListed {
			break
		}
		listed = append(listed, fmt.Sprintf("%s (%s: %s)", failure.Path, failure.Stage, failure.ErrMsg))
	}
	message := fmt.Sprintf(
		"refresh skipped %d file(s); index may be incomplete",
		len(report.Failures),
	)
	if len(listed) > 0 {
		message = fmt.Sprintf("%s: %s", message, strings.Join(listed, "; "))
	}
	if len(report.Failures) > maxListed {
		message = fmt.Sprintf("%s; and %d more", message, len(report.Failures)-maxListed)
	}

	return []commandexec.Warning{{
		Code:    codes.WarnIndexUpdateFailed,
		Message: message,
		Ref:     "Run 'rvn reindex' or 'rvn check' to inspect failed files",
	}}
}

func refreshUnknownFieldWarnings(report readsvc.SmartReindexReport) []commandexec.Warning {
	if len(report.Warnings) == 0 {
		return nil
	}

	const maxListed = 5
	listed := make([]string, 0, maxListed)
	for i, warning := range report.Warnings {
		if i >= maxListed {
			break
		}
		listed = append(listed, warning.Message)
	}
	message := fmt.Sprintf(
		"refresh found %d unknown frontmatter key(s)",
		len(report.Warnings),
	)
	if len(listed) > 0 {
		message = fmt.Sprintf("%s: %s", message, strings.Join(listed, "; "))
	}
	if len(report.Warnings) > maxListed {
		message = fmt.Sprintf("%s; and %d more", message, len(report.Warnings)-maxListed)
	}

	return []commandexec.Warning{{
		Code:    codes.WarnUnknownField,
		Message: message,
		Ref:     "Add the field to schema.yaml or remove it; run 'rvn check' for details",
	}}
}
