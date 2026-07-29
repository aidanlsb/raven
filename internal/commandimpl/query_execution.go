package commandimpl

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/querysvc"
	"github.com/aidanlsb/raven/internal/readsvc"
)

// queryExecution owns the invocation-scoped runtime and resolved query needed
// by the canonical query command. Keeping setup here gives CLI and MCP callers
// one path for config/schema loading, resolver readiness, and index refresh.
type queryExecution struct {
	runtime         *readsvc.Runtime
	resolvedQuery   string
	queryName       string
	isSavedQuery    bool
	refreshWarnings []commandexec.Warning
}

type queryExecutionOptions struct {
	IDsOnly   bool
	Limit     int
	Offset    int
	CountOnly bool
}

func prepareQueryExecution(req commandexec.Request) (execution *queryExecution, failure commandexec.Result) {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return nil, commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	rt, failure := newConfigCommandVaultRuntime(vaultPath)
	if failure.Error != nil {
		return nil, failure
	}
	ready := false
	defer func() {
		if !ready {
			rt.Close()
		}
	}()

	queryString := strings.TrimSpace(stringArg(req.Args, "query_string"))
	if queryString == "" {
		return nil, commandexec.Failure("MISSING_ARGUMENT", "specify a query string", nil, "Run 'rvn query saved list' to see saved queries")
	}

	resolvedQuery, queryName, isSavedQuery, err := resolveQueryString(queryString, req.Args["inputs"], rt.VaultCfg)
	if err != nil {
		return nil, mapQuerySvcFailure(err)
	}
	if isSavedQuery && !querysvc.IsFullQueryRoot(resolvedQuery) {
		return nil, commandexec.Failure("QUERY_INVALID", fmt.Sprintf("saved query '%s' must start with 'type:', 'trait:', 'section', or 'link'", queryName), nil, "")
	}

	if rt.SchemaLoadErr != nil {
		return nil, commandexec.Failure("SCHEMA_INVALID", "failed to load schema", nil, "Fix schema.yaml and try again")
	}
	if failure := openCommandRuntimeDB(rt); failure.Error != nil {
		return nil, failure
	}

	// A non-saved string that is not a concrete query root is an unknown query.
	// Build the hint only after the index and schema are ready so the shared
	// resolver can offer object-aware suggestions to both transports.
	if !isSavedQuery && !querysvc.IsFullQueryRoot(resolvedQuery) {
		suggestion := buildUnknownQuerySuggestion(rt.DB, resolvedQuery, rt.VaultCfg.GetDailyDirectory(), rt.Schema)
		return nil, commandexec.Failure(codes.ErrQueryInvalid, "unknown query: "+resolvedQuery, nil, suggestion)
	}

	compatible, err := rt.DB.SchemaCompatible()
	if err != nil {
		return nil, commandexec.Failure(codes.ErrDatabase, "failed to read index schema version", nil, "Run 'rvn reindex --full' to rebuild the index")
	}
	if !compatible {
		return nil, commandexec.Failure(codes.ErrDatabaseVersion, "index schema is stale or incompatible", nil, "Run 'rvn reindex --full' to rebuild the index")
	}

	var refreshWarnings []commandexec.Warning
	if boolArg(req.Args, "refresh") {
		report, err := readsvc.SmartReindex(rt)
		if err != nil {
			return nil, commandexec.Failure("DATABASE_ERROR", fmt.Sprintf("failed to refresh index: %v", err), nil, "Run 'rvn reindex' to rebuild the database")
		}
		refreshWarnings = append(refreshFailureWarnings(report), refreshUnknownFieldWarnings(report)...)
		refreshWarnings = append(refreshWarnings, refreshReferenceResolutionWarnings(report)...)
	}

	execution = &queryExecution{
		runtime:         rt,
		resolvedQuery:   resolvedQuery,
		queryName:       queryName,
		isSavedQuery:    isSavedQuery,
		refreshWarnings: refreshWarnings,
	}
	ready = true
	return execution, commandexec.Result{}
}

func (e *queryExecution) Close() {
	if e != nil && e.runtime != nil {
		e.runtime.Close()
	}
}

func queryExecutionOptionsFromRequest(req commandexec.Request, hasApply bool) (queryExecutionOptions, commandexec.Result) {
	limit, _ := intArg(req.Args, "limit")
	offset, _ := intArg(req.Args, "offset")
	options := queryExecutionOptions{
		IDsOnly:   boolArg(req.Args, "ids"),
		Limit:     limit,
		Offset:    offset,
		CountOnly: boolArg(req.Args, "count-only"),
	}

	if options.Limit < 0 {
		return queryExecutionOptions{}, commandexec.Failure("INVALID_INPUT", "--limit must be >= 0", nil, "Use --limit 0 for no limit")
	}
	if options.Offset < 0 {
		return queryExecutionOptions{}, commandexec.Failure("INVALID_INPUT", "--offset must be >= 0", nil, "Use --offset 0 for no offset")
	}
	if hasApply && (options.Limit > 0 || options.Offset > 0 || options.CountOnly) {
		return queryExecutionOptions{}, commandexec.Failure(
			"INVALID_INPUT",
			"--limit, --offset, and --count-only cannot be used with --apply",
			nil,
			"Remove pagination/count-only flags when using --apply",
		)
	}
	return options, commandexec.Result{}
}

func (e *queryExecution) Execute(options queryExecutionOptions) (*readsvc.ExecuteQueryResult, commandexec.Result) {
	result, err := readsvc.ExecuteQuery(e.runtime, readsvc.ExecuteQueryRequest{
		QueryString: e.resolvedQuery,
		IDsOnly:     options.IDsOnly,
		Limit:       options.Limit,
		Offset:      options.Offset,
		CountOnly:   options.CountOnly,
	})
	if err != nil {
		return nil, mapExecuteQueryFailure(e.resolvedQuery, err)
	}
	return result, commandexec.Result{}
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

func refreshReferenceResolutionWarnings(report readsvc.SmartReindexReport) []commandexec.Warning {
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
		message = fmt.Sprintf("%s: %s", message, strings.Join(listed, "; "))
	}
	if len(report.ReferenceResolutionWarnings) > maxListed {
		message = fmt.Sprintf("%s; and %d more", message, len(report.ReferenceResolutionWarnings)-maxListed)
	}

	return []commandexec.Warning{{
		Code:    codes.WarnRefResolutionIncomplete,
		Message: message,
		Ref:     "The files were indexed successfully, but backlinks may be stale. Run 'rvn reindex' to retry reference resolution.",
	}}
}

func mapQuerySvcFailure(err error) commandexec.Result {
	return commandexec.FromServiceError(err)
}
