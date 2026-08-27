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
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
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
	if query == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires search query", nil, "Usage: rvn search <query>")
	}

	rt, failure := newReadRuntime(req.VaultPath, vaultruntime.Options{OpenDB: true})
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	results, err := readsvc.Search(rt, query, searchType, limit)
	if err != nil {
		return mapSearchFailure(err)
	}

	return commandexec.Success(commandpayload.SearchResult{
		Query: query,
		Items: searchMatchItems(results),
	}, &commandexec.Meta{Count: len(results), QueryTimeMs: time.Since(start).Milliseconds()})
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
		strings.Contains(message, "unterminated string")
}

// HandleBacklinks executes the canonical `backlinks` command.
func HandleBacklinks(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()

	rt, failure := newReadRuntime(req.VaultPath, vaultruntime.Options{OpenDB: true})
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	if backlinksStdinMode(req.Args) {
		return handleBacklinksStdin(rt, req, start)
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

	return commandexec.Success(map[string]interface{}{
		"target": group.Target,
		"items":  referenceItems(group.Items),
	}, &commandexec.Meta{Count: group.Count, QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleOutlinks executes the canonical `outlinks` command.
func HandleOutlinks(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()

	rt, failure := newReadRuntime(req.VaultPath, vaultruntime.Options{OpenDB: true})
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	if outlinksStdinMode(req.Args) {
		return handleOutlinksStdin(rt, req, start)
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

	return commandexec.Success(map[string]interface{}{
		"source": group.Source,
		"items":  referenceItems(group.Items),
	}, &commandexec.Meta{Count: group.Count, QueryTimeMs: time.Since(start).Milliseconds()})
}

func handleBacklinksStdin(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
	references := stringSliceArg(req.Args["references"])
	if len(references) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no references provided via stdin", nil, "Pipe references to stdin, one per line")
	}

	serviceResult := readsvc.BacklinksForReferences(rt, references)
	groups := serviceResult.Groups
	errors := make([]model.ReferenceInputError, 0)
	for _, failure := range serviceResult.Failures {
		errors = append(errors, referenceInputError(failure.Input, mapTraversalFailure(failure)))
	}

	return commandexec.Success(map[string]interface{}{
		"stdin":           true,
		"items_by_target": groups,
		"errors":          errors,
		"total_inputs":    len(references),
		"resolved":        len(groups),
	}, &commandexec.Meta{Count: serviceResult.Total, QueryTimeMs: time.Since(start).Milliseconds()})
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
	groups := serviceResult.Groups
	errors := make([]model.ReferenceInputError, 0)
	for _, failure := range serviceResult.Failures {
		errors = append(errors, referenceInputError(failure.Input, mapTraversalFailure(failure)))
	}

	return commandexec.Success(map[string]interface{}{
		"stdin":           true,
		"items_by_source": groups,
		"errors":          errors,
		"total_inputs":    len(references),
		"resolved":        len(groups),
	}, &commandexec.Meta{Count: serviceResult.Total, QueryTimeMs: time.Since(start).Milliseconds()})
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

	rt, failure := newReadRuntime(req.VaultPath, vaultruntime.Options{OpenDB: true})
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	result, err := readsvc.ResolveReference(rt, reference)
	if err != nil {
		return commandexec.Success(map[string]interface{}{
			"resolved":  false,
			"reference": reference,
		}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	}
	if result.Ambiguous {
		matches := make([]map[string]interface{}, 0, len(result.Matches))
		for _, match := range result.Matches {
			entry := map[string]interface{}{"object_id": match}
			if result.MatchSources != nil {
				if source, ok := result.MatchSources[match]; ok {
					entry["match_source"] = source
				}
			}
			matches = append(matches, entry)
		}

		return commandexec.Success(map[string]interface{}{
			"resolved":  false,
			"ambiguous": true,
			"reference": reference,
			"items":     matches,
		}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	}

	resolved := result.Resolved
	relPath := resolved.FilePath
	if rel, relErr := filepath.Rel(req.VaultPath, resolved.FilePath); relErr == nil {
		relPath = rel
	}

	data := map[string]interface{}{
		"resolved":   true,
		"object_id":  resolved.ObjectID,
		"file_path":  relPath,
		"is_section": resolved.IsSection,
	}
	if result.ObjectType != "" {
		data["type"] = result.ObjectType
	}
	if resolved.MatchSource != "" {
		data["match_source"] = resolved.MatchSource
	}
	if resolved.IsSection {
		data["file_object_id"] = resolved.FileObjectID
		if resolved.LineStart > 0 {
			data["line_start"] = resolved.LineStart
		}
		if resolved.LineEnd != nil {
			data["line_end"] = resolved.LineEnd
			data["direct_line_end"] = resolved.LineEnd
		}
		if resolved.SubtreeLineEnd != nil {
			data["subtree_line_end"] = resolved.SubtreeLineEnd
		}
	}

	return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleRead executes the canonical `read` command.
func HandleRead(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	reference := stringArg(req.Args, "reference")
	raw := boolArg(req.Args, "raw")
	lines := boolArg(req.Args, "lines")
	startLine, _ := intArg(req.Args, "start-line")
	endLine, _ := intArg(req.Args, "end-line")

	rt, failure := newReadRuntime(req.VaultPath, vaultruntime.Options{OpenDB: false})
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	if boolArg(req.Args, "sections") {
		outline, err := readsvc.ReadSections(rt, reference)
		if err != nil {
			return mapReadFailure(err)
		}
		return withSchemaLoadWarning(rt, commandexec.Success(commandpayload.ReadSectionsResult{
			ObjectID: outline.ObjectID,
			Path:     outline.Path,
			Sections: sectionOutlineItems(outline.Sections),
		}, &commandexec.Meta{Count: len(outline.Sections), QueryTimeMs: time.Since(start).Milliseconds()}))
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
		payload := commandpayload.ReadRawResult{
			ObjectID:  result.ObjectID,
			Path:      result.Path,
			Content:   result.Content,
			LineCount: result.LineCount,
		}
		if result.StartLine > 0 {
			payload.StartLine = result.StartLine
			payload.EndLine = result.EndLine
		}
		if len(result.Lines) > 0 {
			payload.Lines = result.Lines
		}
		return withSchemaLoadWarning(rt, commandexec.Success(payload, meta))
	}

	meta.Count = result.BacklinksCount
	return withSchemaLoadWarning(rt, commandexec.Success(commandpayload.ReadContentResult{
		ObjectID:   result.ObjectID,
		Path:       result.Path,
		Content:    result.Content,
		LineCount:  result.LineCount,
		References: result.References,
		Backlinks:  result.Backlinks,
	}, meta))
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

func sectionOutlineItems(sections []model.Section) []commandpayload.ReadSectionItem {
	out := make([]commandpayload.ReadSectionItem, 0, len(sections))
	for _, section := range sections {
		out = append(out, commandpayload.ReadSectionItem{
			ID:              section.ID,
			Slug:            section.Slug,
			Title:           section.Title,
			Level:           section.Level,
			LineStart:       section.LineStart,
			LineEnd:         section.LineEnd,
			SubtreeLineEnd:  section.SubtreeLineEnd,
			ParentSectionID: section.ParentSectionID,
		})
	}
	return out
}

// HandleOpen executes the canonical `open` command.
func HandleOpen(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	rt, failure := newReadRuntime(vaultPath, vaultruntime.Options{OpenDB: false})
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

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

		relPaths := make([]string, 0, len(result.Targets))
		for _, target := range result.Targets {
			relPaths = append(relPaths, target.RelativePath)
		}

		errs := make([]string, 0, len(result.Failures))
		for _, failure := range result.Failures {
			errs = append(errs, fmt.Sprintf("%s: %s", failure.Reference, failure.Message))
		}

		return commandexec.Success(map[string]interface{}{
			"files":   relPaths,
			"targets": result.Targets,
			"opened":  result.Opened,
			"editor":  result.Editor,
			"errors":  errs,
		}, &commandexec.Meta{Count: len(relPaths)})
	}

	reference := strings.TrimSpace(stringArg(req.Args, "reference"))
	if reference == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires reference argument", nil, "Usage: rvn open <reference>")
	}

	target, opened, editor, err := readsvc.OpenReference(rt, cfg, reference)
	if err != nil {
		return mapOpenFailure(err)
	}

	data := map[string]interface{}{
		"object_id": target.ObjectID,
		"file":      target.RelativePath,
		"opened":    opened,
		"editor":    editor,
	}
	if target.IsSection {
		data["is_section"] = true
		data["file_object_id"] = target.FileObjectID
		if target.LineStart > 0 {
			data["line_start"] = target.LineStart
		}
	}
	return commandexec.Success(data, nil)
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

func newReadRuntime(vaultPath string, opts vaultruntime.Options) (*vaultruntime.Runtime, commandexec.Result) {
	rt, err := vaultruntime.New(strings.TrimSpace(vaultPath), opts)
	if err != nil {
		return nil, mapReadRuntimeSetupFailure(err)
	}
	return rt, commandexec.Result{}
}

func mapReadRuntimeSetupFailure(err error) commandexec.Result {
	if err == nil {
		return commandexec.Failure("INTERNAL_ERROR", "failed to initialize read runtime", nil, "")
	}
	if failure, ok := mapIndexRebuildRequired(err); ok {
		return failure
	}

	if errors.Is(err, vaultruntime.ErrVaultPathRequired) {
		return commandexec.Failure("VAULT_NOT_SPECIFIED", "no vault path resolved", nil, "Use --vault-path, --vault, active_vault, or default_vault")
	}

	var setupErr *vaultruntime.SetupError
	if errors.As(err, &setupErr) {
		switch setupErr.Stage {
		case vaultruntime.StageConfig:
			return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
		case vaultruntime.StageSchema:
			return commandexec.Failure("SCHEMA_INVALID", "failed to load schema.yaml", nil, "Fix schema.yaml and try again")
		case vaultruntime.StageDatabase:
			return commandexec.Failure("DATABASE_ERROR", "failed to open database", nil, "Run 'rvn reindex' to rebuild the database")
		}
	}

	return commandexec.Failure("DATABASE_ERROR", "failed to open database", nil, "Run 'rvn reindex' to rebuild the database")
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

func searchMatchItems(results []model.SearchMatch) []commandpayload.SearchMatchItem {
	items := make([]commandpayload.SearchMatchItem, len(results))
	for i, r := range results {
		items[i] = commandpayload.SearchMatchItem{
			ObjectID:  r.ObjectID,
			Title:     r.Title,
			FilePath:  r.FilePath,
			Snippet:   r.Snippet,
			Rank:      r.Rank,
			IsSection: r.IsSection,
		}
		if r.IsSection {
			items[i].FileObjectID = r.FileObjectID
			items[i].LineStart = r.LineStart
			items[i].LineEnd = r.LineEnd
			items[i].DirectLineEnd = r.LineEnd
			items[i].SubtreeLineEnd = r.SubtreeLineEnd
		}
	}
	return items
}

func referenceItems(references []model.Reference) []model.Reference {
	items := make([]model.Reference, len(references))
	copy(items, references)
	return items
}
