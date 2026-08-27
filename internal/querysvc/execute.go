package querysvc

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/bulkops"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/shellquote"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type ExecuteRequest struct {
	QueryString string
	Inputs      []string
	Refresh     bool
	IDsOnly     bool
	Limit       int
	Offset      int
	CountOnly   bool
	Apply       []string
}

type ExecuteResult struct {
	QueryKind string
	TypeName  string
	Total     int
	Returned  int
	Offset    int
	Limit     int
	IDs       []string
	Objects   []model.Object
	Traits    []model.Trait
	Sections  []model.Section
	Links     []model.Link
}

func (r *ExecuteResult) HasMore() bool {
	return r != nil && r.Offset+r.Returned < r.Total
}

func (r *ExecuteResult) NextOffset() int {
	if r == nil {
		return 0
	}
	return r.Offset + r.Returned
}

type Warning struct {
	Code    codes.WarningCode
	Message string
	Ref     string
}

type ApplyPlan struct {
	Command string
	Args    map[string]interface{}
	Empty   bool
}

type RunResult struct {
	Query     *ExecuteResult
	IsSaved   bool
	SavedName string
	Warnings  []Warning
	ApplyPlan *ApplyPlan
	CountOnly bool
	IDsOnly   bool
}

// Run resolves, validates, refreshes, and executes one query invocation. When
// Apply is present it also builds the nested mutation plan, leaving only nested
// command dispatch and response-envelope shaping to commandimpl.
func Run(rt *vaultruntime.Runtime, req ExecuteRequest) (*RunResult, error) {
	queryString := strings.TrimSpace(req.QueryString)
	if queryString == "" {
		return nil, svcerr.New(codes.ErrMissingArgument, "specify a query string").
			WithSuggestion("Run 'rvn query saved list' to see saved queries")
	}
	if req.Limit < 0 {
		return nil, svcerr.New(codes.ErrInvalidInput, "--limit must be >= 0").
			WithSuggestion("Use --limit 0 for no limit")
	}
	if req.Offset < 0 {
		return nil, svcerr.New(codes.ErrInvalidInput, "--offset must be >= 0").
			WithSuggestion("Use --offset 0 for no offset")
	}
	if len(req.Apply) > 0 && (req.Limit > 0 || req.Offset > 0 || req.CountOnly) {
		return nil, svcerr.New(codes.ErrInvalidInput, "--limit, --offset, and --count-only cannot be used with --apply").
			WithSuggestion("Remove pagination/count-only flags when using --apply")
	}
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault runtime is required", err)
	}
	if rt.VaultCfg == nil {
		return nil, svcerr.New(codes.ErrConfigInvalid, "vault config runtime is required").
			WithSuggestion("Fix raven.yaml and try again")
	}

	name, saved, inputTokens, isSaved := MatchInvocation(rt.VaultCfg, queryString)
	resolvedQuery := queryString
	if isSaved {
		var err error
		resolvedQuery, err = ResolveSavedQuery(name, saved, inputTokens, req.Inputs)
		if err != nil {
			return nil, err
		}
		if !IsFullQueryRoot(resolvedQuery) {
			return nil, svcerr.New(codes.ErrQueryInvalid, fmt.Sprintf("saved query '%s' must start with 'type:', 'trait:', 'section', or 'link'", name))
		}
	}

	if rt.SchemaLoadErr != nil {
		return nil, svcerr.Wrap(codes.ErrSchemaInvalid, "failed to load schema", rt.SchemaLoadErr).
			WithSuggestion("Fix schema.yaml and try again")
	}
	if err := rt.OpenDB(); err != nil {
		if errors.Is(err, index.ErrIndexRebuildRequired) {
			return nil, svcerr.Wrap(codes.ErrDatabaseVersion, "index schema is stale or a rebuild was interrupted", err).
				WithSuggestion("Run 'rvn reindex --full' to rebuild the index")
		}
		return nil, svcerr.Wrap(codes.ErrDatabase, "failed to open database", err).
			WithSuggestion("Run 'rvn reindex' to rebuild the database")
	}

	if !isSaved && !IsFullQueryRoot(resolvedQuery) {
		return nil, svcerr.New(codes.ErrQueryInvalid, "unknown query: "+resolvedQuery).
			WithSuggestion(unknownQuerySuggestion(rt, resolvedQuery))
	}
	compatible, err := rt.DB.SchemaCompatible()
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrDatabase, "failed to read index schema version", err).
			WithSuggestion("Run 'rvn reindex --full' to rebuild the index")
	}
	if !compatible {
		return nil, svcerr.New(codes.ErrDatabaseVersion, "index schema is stale or incompatible").
			WithSuggestion("Run 'rvn reindex --full' to rebuild the index")
	}

	var warnings []Warning
	if req.Refresh {
		report, refreshErr := readsvc.SmartReindex(rt)
		if refreshErr != nil {
			return nil, svcerr.Wrap(codes.ErrDatabase, fmt.Sprintf("failed to refresh index: %v", refreshErr), refreshErr).
				WithSuggestion("Run 'rvn reindex' to rebuild the database")
		}
		warnings = refreshWarnings(report)
	}

	result, err := execute(rt, ExecuteRequest{
		QueryString: resolvedQuery,
		IDsOnly:     req.IDsOnly,
		Limit:       req.Limit,
		Offset:      req.Offset,
		CountOnly:   req.CountOnly,
	})
	if err != nil {
		return nil, executeError(resolvedQuery, err)
	}

	runResult := &RunResult{
		Query:     result,
		IsSaved:   isSaved,
		SavedName: name,
		Warnings:  warnings,
		CountOnly: req.CountOnly,
		IDsOnly:   req.IDsOnly,
	}
	if len(req.Apply) > 0 {
		plan, planErr := planApply(result, req.Apply)
		if planErr != nil {
			return nil, planErr
		}
		runResult.ApplyPlan = plan
	}
	return runResult, nil
}

func execute(rt *vaultruntime.Runtime, req ExecuteRequest) (*ExecuteResult, error) {
	result, err := readsvc.ExecuteQuery(rt, readsvc.ExecuteQueryRequest{
		QueryString: req.QueryString,
		IDsOnly:     req.IDsOnly,
		Limit:       req.Limit,
		Offset:      req.Offset,
		CountOnly:   req.CountOnly,
	})
	if err != nil {
		return nil, err
	}
	return &ExecuteResult{
		QueryKind: result.QueryKind,
		TypeName:  result.TypeName,
		Total:     result.Total,
		Returned:  result.Returned,
		Offset:    result.Offset,
		Limit:     result.Limit,
		IDs:       result.IDs,
		Objects:   result.Objects,
		Traits:    result.Traits,
		Sections:  result.Sections,
		Links:     result.Links,
	}, nil
}

func executeError(queryString string, err error) error {
	var validationErr *query.ValidationError
	if errors.As(err, &validationErr) {
		return svcerr.Wrap(codes.ErrQueryInvalid, validationErr.Message, err).
			WithSuggestion(validationErr.Suggestion)
	}
	var executionErr *query.ExecutionError
	if errors.As(err, &executionErr) {
		return svcerr.Wrap(codes.ErrQueryInvalid, executionErr.Message, err).
			WithSuggestion(executionErr.Suggestion)
	}
	if _, parseErr := query.Parse(queryString); parseErr != nil {
		return svcerr.Wrap(codes.ErrQueryInvalid, fmt.Sprintf("parse error: %v", parseErr), err).
			WithSuggestion(queryParseSuggestion(queryString))
	}
	return svcerr.Wrap(codes.ErrDatabase, err.Error(), err).
		WithSuggestion("Run 'rvn reindex' to rebuild the database")
}

func queryParseSuggestion(queryString string) string {
	if hint, ok := querySyntaxHint(queryString); ok {
		return hint
	}
	return "Run 'rvn docs querying query-language' for RQL syntax and examples"
}

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

func unknownQuerySuggestion(rt *vaultruntime.Runtime, queryString string) string {
	base := "Queries must start with 'type:', 'trait:', 'section', or 'link', or be a saved query name. Run 'rvn query saved list' to see saved queries."
	q := strings.TrimSpace(queryString)
	if q == "" || strings.ContainsAny(q, " \t\r\n") {
		if hint, ok := querySyntaxHint(q); ok {
			return hint
		}
		return base
	}
	if rt.Schema != nil {
		if _, ok := rt.Schema.Types[q]; ok {
			return base + fmt.Sprintf(" Did you mean to query type %q? Try: %s", q, "rvn query type:"+q)
		}
	}
	resolver, err := rt.DB.Resolver(indexschema.ResolverOptions{
		DailyDirectory: rt.VaultCfg.GetDailyDirectory(),
		Schema:         rt.Schema,
	})
	if err != nil {
		return base
	}
	resolved := resolver.Resolve(q)
	if resolved.Ambiguous {
		return base + " Did you mean to resolve a reference? Try: rvn resolve " + shellquote.QuoteIfNeeded(q)
	}
	if resolved.TargetID == "" {
		return base
	}
	return base + fmt.Sprintf(
		" Did you mean to open/read an object reference? Try: %s or %s",
		"rvn read "+shellquote.QuoteIfNeeded(q),
		"rvn open "+shellquote.QuoteIfNeeded(q),
	)
}

func planApply(result *ExecuteResult, applyArgs []string) (*ApplyPlan, error) {
	if result.QueryKind == "link" {
		return nil, unsupportedApplyError(result.QueryKind)
	}
	rawApply, err := bulkops.ParseRawApply(applyArgs)
	if err != nil {
		return nil, bulkopsError(err)
	}
	if result.QueryKind == "section" && rawApply.Command != string(bulkops.ObjectApplyMove) {
		return nil, unsupportedApplyError(result.QueryKind)
	}
	if result.QueryKind == "trait" {
		plan, err := bulkops.PlanTraitApply(rawApply, result.Traits)
		if err != nil {
			return nil, bulkopsError(err)
		}
		return &ApplyPlan{
			Command: "update",
			Args: map[string]interface{}{
				"stdin":     true,
				"value":     plan.NewValue,
				"trait_ids": traitIDsToInterfaces(result.Traits),
			},
		}, nil
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
	ids = dedupeApplyIDs(ids)
	if len(ids) == 0 {
		return &ApplyPlan{Command: rawApply.Command, Empty: true}, nil
	}
	plan, err := bulkops.PlanObjectApply(rawApply, ids)
	if err != nil {
		return nil, bulkopsError(err)
	}
	switch plan.Command {
	case bulkops.ObjectApplySet:
		return &ApplyPlan{Command: "set", Args: map[string]interface{}{
			"stdin": true, "fields": plan.SetUpdates, "references": stringsToInterfaces(plan.IDs),
		}}, nil
	case bulkops.ObjectApplyDelete:
		return &ApplyPlan{Command: "delete", Args: map[string]interface{}{
			"stdin": true, "references": stringsToInterfaces(plan.IDs),
		}}, nil
	case bulkops.ObjectApplyAdd:
		return &ApplyPlan{Command: "add", Args: map[string]interface{}{
			"stdin": true, "text": plan.AddText, "object_ids": stringsToInterfaces(plan.IDs),
		}}, nil
	case bulkops.ObjectApplyMove:
		return &ApplyPlan{Command: "move", Args: map[string]interface{}{
			"stdin": true, "destination": plan.MoveDestination, "update-refs": true, "object_ids": stringsToInterfaces(plan.IDs),
		}}, nil
	default:
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("unknown apply command: %s", plan.Command)).
			WithSuggestion("Supported commands: set, delete, add, move")
	}
}

func unsupportedApplyError(queryKind string) error {
	return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("--apply is not supported for %s queries", queryKind)).
		WithSuggestion("Use --ids and pass results to a compatible command")
}

func bulkopsError(err error) error {
	bulkErr, ok := bulkops.AsError(err)
	if !ok {
		return svcerr.Wrap(codes.ErrInternal, err.Error(), err)
	}
	return svcerr.Wrap(bulkErr.Code, bulkErr.Message, err).WithSuggestion(bulkErr.Suggestion)
}

func dedupeApplyIDs(ids []string) []string {
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

func refreshWarnings(report readsvc.SmartReindexReport) []Warning {
	warnings := refreshFailureWarnings(report)
	warnings = append(warnings, refreshUnknownFieldWarnings(report)...)
	return append(warnings, refreshReferenceResolutionWarnings(report)...)
}

func refreshFailureWarnings(report readsvc.SmartReindexReport) []Warning {
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
	message := fmt.Sprintf("refresh skipped %d file(s); index may be incomplete", len(report.Failures))
	if len(listed) > 0 {
		message += ": " + strings.Join(listed, "; ")
	}
	if len(report.Failures) > maxListed {
		message = fmt.Sprintf("%s; and %d more", message, len(report.Failures)-maxListed)
	}
	return []Warning{{
		Code: codes.WarnIndexUpdateFailed, Message: message,
		Ref: "Run 'rvn reindex' or 'rvn check' to inspect failed files",
	}}
}

func refreshUnknownFieldWarnings(report readsvc.SmartReindexReport) []Warning {
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
	message := fmt.Sprintf("refresh found %d unknown frontmatter key(s)", len(report.Warnings))
	if len(listed) > 0 {
		message += ": " + strings.Join(listed, "; ")
	}
	if len(report.Warnings) > maxListed {
		message = fmt.Sprintf("%s; and %d more", message, len(report.Warnings)-maxListed)
	}
	return []Warning{{
		Code: codes.WarnUnknownField, Message: message,
		Ref: "Add the field to schema.yaml or remove it; run 'rvn check' for details",
	}}
}

func refreshReferenceResolutionWarnings(report readsvc.SmartReindexReport) []Warning {
	if len(report.ReferenceResolutionWarnings) == 0 {
		return nil
	}
	const maxListed = 5
	listed := make([]string, 0, maxListed)
	for i, warning := range report.ReferenceResolutionWarnings {
		if i >= maxListed {
			break
		}
		listed = append(listed, warning.Message)
	}
	message := fmt.Sprintf(
		"refresh indexed %d file(s), but reference resolution did not complete",
		len(report.ReferenceResolutionWarnings),
	)
	if len(listed) > 0 {
		message += ": " + strings.Join(listed, "; ")
	}
	if len(report.ReferenceResolutionWarnings) > maxListed {
		message = fmt.Sprintf("%s; and %d more", message, len(report.ReferenceResolutionWarnings)-maxListed)
	}
	return []Warning{{
		Code: codes.WarnRefResolutionIncomplete, Message: message,
		Ref: "The files were indexed successfully, but backlinks may be stale. Run 'rvn reindex' to retry reference resolution.",
	}}
}
