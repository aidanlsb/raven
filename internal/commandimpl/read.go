package commandimpl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
	"github.com/aidanlsb/raven/internal/vaultsession"
)

// HandleSearch executes the canonical `search` command.
func HandleSearch(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	query := strings.TrimSpace(stringArg(req.Args, "query"))
	searchType := stringArg(req.Args, "type")
	limit, ok := intArg(req.Args, "limit")
	if !ok {
		limit = 20
	}
	offset, _ := intArg(req.Args, "offset")
	if query == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires search query", nil, "Usage: rvn search <query>")
	}

	sess, failure := openSession(req.VaultPath, vaultsession.Read(refreshPolicy(req.Args)))
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()
	refreshWarnings := sess.RefreshWarnings()

	results, total, err := readsvc.Search(rt, query, searchType, limit, offset)
	if err != nil {
		return mapSearchFailure(err)
	}
	payload := readsvc.SearchPayload(query, results, total, offset, limit)
	return withIndexRefreshWarnings(refreshWarnings, commandexec.Success(payload, &commandexec.Meta{Count: len(payload.Items), QueryTimeMs: time.Since(start).Milliseconds()}))
}

func mapSearchFailure(err error) commandexec.Result {
	if err == nil {
		return commandexec.Failure("INTERNAL_ERROR", "search failed", nil, "")
	}
	if failure, ok := mapIndexRebuildRequired(err); ok {
		return failure
	}

	if isSearchSyntaxError(err) {
		return commandexec.Failure(
			"INVALID_INPUT",
			"invalid search query",
			map[string]interface{}{"cause": err.Error()},
			"Quote special characters or use a simpler full-text query and retry.",
		)
	}

	return commandexec.Failure("DATABASE_ERROR", fmt.Sprintf("search failed: %v", err), nil, "Run 'rvn reindex' to rebuild the database")
}

func isSearchSyntaxError(err error) bool {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "fts5: syntax error") ||
		strings.Contains(message, "malformed match expression") ||
		strings.Contains(message, "unterminated string") ||
		strings.Contains(message, "no such column:")
}

// HandleBacklinks executes the canonical `backlinks` command.
func HandleBacklinks(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()

	sess, failure := openSession(req.VaultPath, vaultsession.Read(refreshPolicy(req.Args)))
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()
	refreshWarnings := sess.RefreshWarnings()

	if backlinksStdinMode(req.Args) {
		return withIndexRefreshWarnings(refreshWarnings, handleBacklinksStdin(rt, req, start))
	}

	reference := stringArg(req.Args, "reference")
	if strings.TrimSpace(reference) == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires reference argument", nil, "Usage: rvn backlinks <reference> or rvn backlinks --stdin")
	}
	result := readsvc.BacklinksForReferences(rt, []string{reference})
	if len(result.Failures) > 0 {
		return mapTraversalFailure(result.Failures[0])
	}
	group := result.Groups[0]

	return withIndexRefreshWarnings(refreshWarnings, commandexec.Success(result.Payload(), &commandexec.Meta{Count: group.Count, QueryTimeMs: time.Since(start).Milliseconds()}))
}

// HandleOutlinks executes the canonical `outlinks` command.
func HandleOutlinks(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()

	sess, failure := openSession(req.VaultPath, vaultsession.Read(refreshPolicy(req.Args)))
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()
	refreshWarnings := sess.RefreshWarnings()

	if outlinksStdinMode(req.Args) {
		return withIndexRefreshWarnings(refreshWarnings, handleOutlinksStdin(rt, req, start))
	}

	reference := stringArg(req.Args, "reference")
	if strings.TrimSpace(reference) == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires reference argument", nil, "Usage: rvn outlinks <reference> or rvn outlinks --stdin")
	}
	result := readsvc.OutlinksForReferences(rt, []string{reference})
	if len(result.Failures) > 0 {
		return mapTraversalFailure(result.Failures[0])
	}
	group := result.Groups[0]

	return withIndexRefreshWarnings(refreshWarnings, commandexec.Success(result.Payload(), &commandexec.Meta{Count: group.Count, QueryTimeMs: time.Since(start).Milliseconds()}))
}

func handleBacklinksStdin(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
	references := stringSliceArg(req.Args["references"])
	if len(references) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no references provided via stdin", nil, "Pipe references to stdin, one per line")
	}

	serviceResult := readsvc.BacklinksForReferences(rt, references)
	errors := make([]model.ReferenceInputError, 0)
	for _, failure := range serviceResult.Failures {
		errors = append(errors, referenceInputError(failure.Input, mapTraversalFailure(failure)))
	}

	return commandexec.Success(serviceResult.StdinPayload(len(references), errors), &commandexec.Meta{Count: serviceResult.Total, QueryTimeMs: time.Since(start).Milliseconds()})
}

func backlinksStdinMode(args map[string]interface{}) bool {
	return boolArg(args, "stdin") || len(stringSliceArg(args["references"])) > 0
}

func handleOutlinksStdin(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
	references := stringSliceArg(req.Args["references"])
	if len(references) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no references provided via stdin", nil, "Pipe references to stdin, one per line")
	}

	serviceResult := readsvc.OutlinksForReferences(rt, references)
	errors := make([]model.ReferenceInputError, 0)
	for _, failure := range serviceResult.Failures {
		errors = append(errors, referenceInputError(failure.Input, mapTraversalFailure(failure)))
	}

	return commandexec.Success(serviceResult.StdinPayload(len(references), errors), &commandexec.Meta{Count: serviceResult.Total, QueryTimeMs: time.Since(start).Milliseconds()})
}

func outlinksStdinMode(args map[string]interface{}) bool {
	return boolArg(args, "stdin") || len(stringSliceArg(args["references"])) > 0
}

func referenceInputError(input string, result commandexec.Result) model.ReferenceInputError {
	err := model.ReferenceInputError{
		Input: input,
		Code:  "INTERNAL_ERROR",
	}
	if result.Error == nil {
		err.Message = "reference traversal failed"
		return err
	}
	err.Code = string(result.Error.Code)
	err.Message = result.Error.Message
	err.Suggestion = result.Error.Suggestion
	err.Details = result.Error.Details
	return err
}

// HandleResolve executes the canonical `resolve` command.
func HandleResolve(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	reference := stringArg(req.Args, "reference")

	sess, failure := openSession(req.VaultPath, vaultsession.Read(refreshPolicy(req.Args)))
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()
	refreshWarnings := sess.RefreshWarnings()

	result, err := readsvc.ResolveReference(rt, reference)
	if err != nil {
		return withIndexRefreshWarnings(refreshWarnings, commandexec.Success(readsvc.ResolveMiss(reference), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()}))
	}
	if result.Ambiguous {
		return withIndexRefreshWarnings(refreshWarnings, commandexec.Success(result.AmbiguousPayload(reference), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()}))
	}

	relPath := result.Resolved.FilePath
	if rel, relErr := filepath.Rel(req.VaultPath, result.Resolved.FilePath); relErr == nil {
		relPath = rel
	}

	return withIndexRefreshWarnings(refreshWarnings, commandexec.Success(result.HitPayload(relPath), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()}))
}

// HandleRead executes the canonical `read` command.
func HandleRead(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	reference := stringArg(req.Args, "reference")
	raw := boolArg(req.Args, "raw")
	lines := boolArg(req.Args, "lines")
	startLine, _ := intArg(req.Args, "start-line")
	endLine, _ := intArg(req.Args, "end-line")

	sess, failure := openSession(req.VaultPath, vaultsession.Config())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	if err := sess.Refresh(refreshPolicy(req.Args)); err != nil {
		return commandexec.FromServiceError(err)
	}
	rt := sess.Runtime()
	refreshWarnings := sess.RefreshWarnings()

	if boolArg(req.Args, "sections") {
		outline, err := readsvc.ReadSections(rt, reference)
		if err != nil {
			return mapReadFailure(err)
		}
		payload := outline.SectionsPayload()
		return withIndexRefreshWarnings(refreshWarnings, withSchemaLoadWarning(rt, commandexec.Success(payload, &commandexec.Meta{Count: len(payload.Sections), QueryTimeMs: time.Since(start).Milliseconds()})))
	}

	result, err := readsvc.Read(rt, readsvc.ReadRequest{
		Reference: reference,
		Raw:       raw,
		Lines:     lines,
		StartLine: startLine,
		EndLine:   endLine,
	})
	if err != nil {
		return mapReadFailure(err)
	}

	rawMode := raw || lines || startLine > 0 || endLine > 0
	meta := &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()}
	if rawMode {
		return withIndexRefreshWarnings(refreshWarnings, withSchemaLoadWarning(rt, commandexec.Success(result.RawPayload(), meta)))
	}

	meta.Count = result.BacklinksCount
	return withIndexRefreshWarnings(refreshWarnings, withSchemaLoadWarning(rt, commandexec.Success(result.ContentPayload(), meta)))
}

// withSchemaLoadWarning appends a stable SCHEMA_LOAD_FAILED warning to a
// successful read result when the runtime tolerated a schema load failure
// (RequireSchema=false). It ensures degraded read paths surface the failure
// instead of silently returning schema-free output.
func withSchemaLoadWarning(rt *vaultruntime.Runtime, result commandexec.Result) commandexec.Result {
	if rt == nil || rt.SchemaLoadErr == nil || !result.OK {
		return result
	}
	result.Warnings = append(result.Warnings, commandexec.Warning{
		Code:    codes.WarnSchemaLoadFailed,
		Message: fmt.Sprintf("schema failed to load; type-aware output may be incomplete: %v", rt.SchemaLoadErr),
	})
	return result
}

// HandleOpen executes the canonical `open` command.
func HandleOpen(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	sess, failure := openSession(vaultPath, vaultsession.Read(refreshPolicy(req.Args)))
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()
	refreshWarnings := sess.RefreshWarnings()

	cfgCtx, err := configsvc.ShowContext(configContextOptions(req))
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", err.Error(), nil, "")
	}
	cfg := cfgCtx.Cfg

	references := stringSliceArg(req.Args["references"])
	if boolArg(req.Args, "stdin") || len(references) > 0 {
		if len(references) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no references provided via stdin", nil, "Provide references via stdin or references")
		}

		result := readsvc.OpenReferences(rt, cfg, references)
		if len(result.Targets) == 0 {
			if len(result.Failures) > 0 {
				return commandexec.Failure("REF_NOT_FOUND", fmt.Sprintf("no files to open: %s: %s", result.Failures[0].Reference, result.Failures[0].Message), nil, "Check references and run 'rvn reindex' if needed")
			}
			return commandexec.Failure("REF_NOT_FOUND", "no files to open", nil, "Check references and run 'rvn reindex' if needed")
		}

		payload := result.Payload()
		return withIndexRefreshWarnings(refreshWarnings, commandexec.Success(payload, &commandexec.Meta{Count: len(payload.Files)}))
	}

	reference := strings.TrimSpace(stringArg(req.Args, "reference"))
	if reference == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires reference argument", nil, "Usage: rvn open <reference>")
	}

	target, opened, editor, err := readsvc.OpenReference(rt, cfg, reference)
	if err != nil {
		return mapOpenFailure(err)
	}

	return withIndexRefreshWarnings(refreshWarnings, commandexec.Success(target.Payload(opened, editor), nil))
}

func mapResolveFailure(err error, reference string) commandexec.Result {
	if failure, ok := mapIndexRebuildRequired(err); ok {
		return failure
	}

	if _, ok := svcerr.AsError(err); ok {
		return commandexec.FromServiceErrorWithFallback(err, "Check the object reference and run 'rvn reindex' if needed")
	}

	return commandexec.Failure("REF_NOT_FOUND", fmt.Sprintf("reference '%s' not found", reference), nil, "Check the object reference and run 'rvn reindex' if needed")
}

func mapTraversalFailure(failure readsvc.ReferenceFailure) commandexec.Result {
	if failure.Operation == "resolve" {
		return mapResolveFailure(failure.Err, failure.Input)
	}
	return commandexec.Failure(
		"DATABASE_ERROR",
		fmt.Sprintf("failed to read %s: %v", failure.Operation, failure.Err),
		nil,
		"",
	)
}

func mapReadFailure(err error) commandexec.Result {
	if failure, ok := mapIndexRebuildRequired(err); ok {
		return failure
	}

	if _, ok := svcerr.AsError(err); ok {
		return commandexec.FromServiceErrorWithFallback(err, "Check the reference and try again")
	}

	if os.IsNotExist(err) {
		return commandexec.Failure("FILE_NOT_FOUND", err.Error(), nil, "Check the path and try again")
	}

	if strings.Contains(err.Error(), "failed to open database") || strings.Contains(err.Error(), "failed to create resolver") {
		return commandexec.Failure("DATABASE_ERROR", err.Error(), nil, "Run 'rvn reindex' to rebuild the database")
	}

	return commandexec.Failure("FILE_READ_ERROR", err.Error(), nil, "")
}

func mapOpenFailure(err error) commandexec.Result {
	if failure, ok := mapIndexRebuildRequired(err); ok {
		return failure
	}

	if _, ok := svcerr.AsError(err); ok {
		return commandexec.FromServiceErrorWithFallback(err, "Check the reference and try again")
	}
	return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
}

func mapIndexRebuildRequired(err error) (commandexec.Result, bool) {
	var setupErr *vaultruntime.SetupError
	if !errors.As(err, &setupErr) || setupErr.Failure != vaultruntime.SetupFailureIndexRebuildRequired {
		return commandexec.Result{}, false
	}
	return commandexec.Failure(
		codes.ErrDatabaseVersion,
		"index schema is stale or a rebuild was interrupted",
		nil,
		"Run 'rvn reindex --full' to rebuild the index",
	), true
}
