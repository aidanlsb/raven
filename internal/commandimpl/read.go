package commandimpl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/configsvc"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/vault"
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

	rt, failure := newReadRuntime(req.VaultPath, readsvc.RuntimeOptions{OpenDB: true})
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	results, err := readsvc.Search(rt, query, searchType, limit)
	if err != nil {
		return mapSearchFailure(err)
	}

	return commandexec.Success(map[string]interface{}{
		"query":   query,
		"results": formatSearchResults(results),
	}, &commandexec.Meta{Count: len(results), QueryTimeMs: time.Since(start).Milliseconds()})
}

func mapSearchFailure(err error) commandexec.Result {
	if err == nil {
		return commandexec.Failure("INTERNAL_ERROR", "search failed", nil, "")
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

	rt, failure := newReadRuntime(req.VaultPath, readsvc.RuntimeOptions{OpenDB: true})
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	if backlinksStdinMode(req.Args) {
		return handleBacklinksStdin(rt, req, start)
	}

	reference := stringArg(req.Args, "target")
	if strings.TrimSpace(reference) == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires target argument", nil, "Usage: rvn backlinks <target> or rvn backlinks --stdin")
	}
	resolved, err := readsvc.ResolveReferenceWithDynamicDates(reference, rt, true)
	if err != nil {
		return mapResolveFailure(err, reference)
	}

	links, err := readsvc.Backlinks(rt, resolved.ObjectID)
	if err != nil {
		return commandexec.Failure("DATABASE_ERROR", fmt.Sprintf("failed to read backlinks: %v", err), nil, "")
	}

	return commandexec.Success(map[string]interface{}{
		"target": resolved.ObjectID,
		"items":  links,
	}, &commandexec.Meta{Count: len(links), QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleOutlinks executes the canonical `outlinks` command.
func HandleOutlinks(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()

	rt, failure := newReadRuntime(req.VaultPath, readsvc.RuntimeOptions{OpenDB: true})
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	if outlinksStdinMode(req.Args) {
		return handleOutlinksStdin(rt, req, start)
	}

	reference := stringArg(req.Args, "source")
	if strings.TrimSpace(reference) == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires source argument", nil, "Usage: rvn outlinks <source> or rvn outlinks --stdin")
	}
	resolved, err := readsvc.ResolveReferenceWithDynamicDates(reference, rt, true)
	if err != nil {
		return mapResolveFailure(err, reference)
	}

	links, err := readsvc.Outlinks(rt, resolved.ObjectID)
	if err != nil {
		return commandexec.Failure("DATABASE_ERROR", fmt.Sprintf("failed to read outlinks: %v", err), nil, "")
	}

	return commandexec.Success(map[string]interface{}{
		"source": resolved.ObjectID,
		"items":  links,
	}, &commandexec.Meta{Count: len(links), QueryTimeMs: time.Since(start).Milliseconds()})
}

func handleBacklinksStdin(rt *readsvc.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
	targets := stringSliceArg(req.Args["targets"])
	if len(targets) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no targets provided via stdin", nil, "Pipe targets to stdin, one per line")
	}

	groups := make([]model.BacklinksGroup, 0, len(targets))
	errors := make([]model.ReferenceInputError, 0)
	total := 0
	for _, target := range targets {
		resolved, err := readsvc.ResolveReferenceWithDynamicDates(target, rt, true)
		if err != nil {
			errors = append(errors, referenceInputError(target, mapResolveFailure(err, target)))
			continue
		}
		links, err := readsvc.Backlinks(rt, resolved.ObjectID)
		if err != nil {
			errors = append(errors, referenceInputError(target, commandexec.Failure("DATABASE_ERROR", fmt.Sprintf("failed to read backlinks: %v", err), nil, "")))
			continue
		}
		groups = append(groups, model.BacklinksGroup{
			Input:  target,
			Target: resolved.ObjectID,
			Items:  links,
			Count:  len(links),
		})
		total += len(links)
	}

	return commandexec.Success(map[string]interface{}{
		"stdin":           true,
		"items_by_target": groups,
		"errors":          errors,
		"total_inputs":    len(targets),
		"resolved":        len(groups),
	}, &commandexec.Meta{Count: total, QueryTimeMs: time.Since(start).Milliseconds()})
}

func backlinksStdinMode(args map[string]interface{}) bool {
	return boolArg(args, "stdin") || len(stringSliceArg(args["targets"])) > 0
}

func handleOutlinksStdin(rt *readsvc.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
	sources := stringSliceArg(req.Args["sources"])
	if len(sources) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no sources provided via stdin", nil, "Pipe sources to stdin, one per line")
	}

	groups := make([]model.OutlinksGroup, 0, len(sources))
	errors := make([]model.ReferenceInputError, 0)
	total := 0
	for _, source := range sources {
		resolved, err := readsvc.ResolveReferenceWithDynamicDates(source, rt, true)
		if err != nil {
			errors = append(errors, referenceInputError(source, mapResolveFailure(err, source)))
			continue
		}
		links, err := readsvc.Outlinks(rt, resolved.ObjectID)
		if err != nil {
			errors = append(errors, referenceInputError(source, commandexec.Failure("DATABASE_ERROR", fmt.Sprintf("failed to read outlinks: %v", err), nil, "")))
			continue
		}
		groups = append(groups, model.OutlinksGroup{
			Input:  source,
			Source: resolved.ObjectID,
			Items:  links,
			Count:  len(links),
		})
		total += len(links)
	}

	return commandexec.Success(map[string]interface{}{
		"stdin":           true,
		"items_by_source": groups,
		"errors":          errors,
		"total_inputs":    len(sources),
		"resolved":        len(groups),
	}, &commandexec.Meta{Count: total, QueryTimeMs: time.Since(start).Milliseconds()})
}

func outlinksStdinMode(args map[string]interface{}) bool {
	return boolArg(args, "stdin") || len(stringSliceArg(args["sources"])) > 0
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

	rt, failure := newReadRuntime(req.VaultPath, readsvc.RuntimeOptions{OpenDB: true})
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	resolved, err := readsvc.ResolveReferenceWithDynamicDates(reference, rt, true)

	var ambiguousErr *readsvc.AmbiguousRefError
	if errors.As(err, &ambiguousErr) {
		matches := make([]map[string]interface{}, 0, len(ambiguousErr.Matches))
		for _, match := range ambiguousErr.Matches {
			entry := map[string]interface{}{"object_id": match}
			if ambiguousErr.MatchSources != nil {
				if source, ok := ambiguousErr.MatchSources[match]; ok {
					entry["match_source"] = source
				}
			}
			matches = append(matches, entry)
		}

		return commandexec.Success(map[string]interface{}{
			"resolved":  false,
			"ambiguous": true,
			"reference": reference,
			"matches":   matches,
		}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	}

	if err != nil {
		return commandexec.Success(map[string]interface{}{
			"resolved":  false,
			"reference": reference,
		}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	}

	relPath := resolved.FilePath
	if rel, relErr := filepath.Rel(req.VaultPath, resolved.FilePath); relErr == nil {
		relPath = rel
	}

	objectType := ""
	if rt.DB != nil {
		if obj, objErr := rt.DB.GetObject(resolved.ObjectID); objErr == nil && obj != nil {
			objectType = obj.Type
		}
	}

	data := map[string]interface{}{
		"resolved":   true,
		"object_id":  resolved.ObjectID,
		"file_path":  relPath,
		"is_section": resolved.IsSection,
	}
	if objectType != "" {
		data["type"] = objectType
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
	reference := stringArg(req.Args, "path")
	raw := boolArg(req.Args, "raw")
	lines := boolArg(req.Args, "lines")
	startLine, _ := intArg(req.Args, "start-line")
	endLine, _ := intArg(req.Args, "end-line")

	rt, failure := newReadRuntime(req.VaultPath, readsvc.RuntimeOptions{OpenDB: false})
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

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

	data := map[string]interface{}{
		"object_id":  result.ObjectID,
		"path":       result.Path,
		"content":    result.Content,
		"line_count": result.LineCount,
	}
	if result.StartLine > 0 {
		data["start_line"] = result.StartLine
		data["end_line"] = result.EndLine
	}

	rawMode := raw || lines || startLine > 0 || endLine > 0
	meta := &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()}
	if rawMode {
		if len(result.Lines) > 0 {
			data["lines"] = result.Lines
		}
		return commandexec.Success(data, meta)
	}

	data["references"] = result.References
	data["backlinks"] = result.Backlinks
	meta.Count = result.BacklinksCount
	return commandexec.Success(data, meta)
}

// HandleOpen executes the canonical `open` command.
func HandleOpen(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	rt, failure := newReadRuntime(vaultPath, readsvc.RuntimeOptions{OpenDB: false})
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	cfgCtx, err := configsvc.ShowContext(configContextOptions(req))
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", err.Error(), nil, "")
	}
	cfg := cfgCtx.Cfg
	editor := ""
	if cfg != nil {
		editor = cfg.GetEditor()
	}

	if boolArg(req.Args, "stdin") {
		references := stringSliceArg(req.Args["object_ids"])
		if len(references) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no object IDs provided via stdin", nil, "Provide object IDs via stdin or object_ids")
		}

		targets, failures := readsvc.ResolveOpenTargets(rt, references)
		if len(targets) == 0 {
			if len(failures) > 0 {
				return commandexec.Failure("REF_NOT_FOUND", fmt.Sprintf("no files to open: %s: %s", failures[0].Reference, failures[0].Message), nil, "Check references and run 'rvn reindex' if needed")
			}
			return commandexec.Failure("REF_NOT_FOUND", "no files to open", nil, "Check references and run 'rvn reindex' if needed")
		}

		filePaths := make([]string, 0, len(targets))
		relPaths := make([]string, 0, len(targets))
		for _, target := range targets {
			filePaths = append(filePaths, target.FilePath)
			relPaths = append(relPaths, target.RelativePath)
		}

		errs := make([]string, 0, len(failures))
		for _, failure := range failures {
			errs = append(errs, fmt.Sprintf("%s: %s", failure.Reference, failure.Message))
		}

		return commandexec.Success(map[string]interface{}{
			"files":   relPaths,
			"targets": targets,
			"opened":  vault.OpenFilesInEditor(cfg, filePaths),
			"editor":  editor,
			"errors":  errs,
		}, &commandexec.Meta{Count: len(relPaths)})
	}

	reference := strings.TrimSpace(stringArg(req.Args, "reference"))
	if reference == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires reference argument", nil, "Usage: rvn open <reference>")
	}

	target, err := readsvc.ResolveOpenTarget(rt, reference)
	if err != nil {
		return mapOpenFailure(err)
	}

	data := map[string]interface{}{
		"object_id": target.ObjectID,
		"file":      target.RelativePath,
		"opened":    vault.OpenInEditorAtLine(cfg, target.FilePath, target.LineStart),
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
	var ambiguous *readsvc.AmbiguousRefError
	if errors.As(err, &ambiguous) {
		return commandexec.Failure("REF_AMBIGUOUS", ambiguous.Error(), nil, "Use a full object ID/path to disambiguate")
	}

	var notFound *readsvc.RefNotFoundError
	if errors.As(err, &notFound) {
		return commandexec.Failure("REF_NOT_FOUND", notFound.Error(), nil, "Check the object reference and run 'rvn reindex' if needed")
	}

	return commandexec.Failure("REF_NOT_FOUND", fmt.Sprintf("reference '%s' not found", reference), nil, "Check the object reference and run 'rvn reindex' if needed")
}

func mapReadFailure(err error) commandexec.Result {
	var ambiguous *readsvc.AmbiguousRefError
	if errors.As(err, &ambiguous) {
		return commandexec.Failure("REF_AMBIGUOUS", ambiguous.Error(), nil, "Use a full object ID/path to disambiguate")
	}

	var notFound *readsvc.RefNotFoundError
	if errors.As(err, &notFound) {
		return commandexec.Failure("REF_NOT_FOUND", notFound.Error(), nil, "Check the reference and try again")
	}

	var invalidRange *readsvc.InvalidLineRangeError
	if errors.As(err, &invalidRange) {
		return commandexec.Failure("INVALID_INPUT", invalidRange.Error(), nil, invalidRange.Suggestion())
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
	var ambiguous *readsvc.AmbiguousRefError
	if errors.As(err, &ambiguous) {
		details := map[string]interface{}{
			"reference": ambiguous.Reference,
			"matches":   ambiguous.Matches,
		}
		if len(ambiguous.MatchSources) > 0 {
			details["match_sources"] = ambiguous.MatchSources
		}
		return commandexec.Failure("REF_AMBIGUOUS", ambiguous.Error(), details, "Use a full object ID/path to disambiguate")
	}
	var notFound *readsvc.RefNotFoundError
	if errors.As(err, &notFound) {
		return commandexec.Failure("REF_NOT_FOUND", notFound.Error(), nil, "Check the reference and try again")
	}
	return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
}

func newReadRuntime(vaultPath string, opts readsvc.RuntimeOptions) (*readsvc.Runtime, commandexec.Result) {
	rt, err := readsvc.NewRuntime(strings.TrimSpace(vaultPath), opts)
	if err != nil {
		return nil, mapReadRuntimeSetupFailure(err)
	}
	return rt, commandexec.Result{}
}

func mapReadRuntimeSetupFailure(err error) commandexec.Result {
	if err == nil {
		return commandexec.Failure("INTERNAL_ERROR", "failed to initialize read runtime", nil, "")
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "vault path is required"):
		return commandexec.Failure("VAULT_NOT_SPECIFIED", "no vault path resolved", nil, "Use --vault-path, --vault, active_vault, or default_vault")
	case isReadRuntimeConfigError(message):
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	default:
		return commandexec.Failure("DATABASE_ERROR", "failed to open database", nil, "Run 'rvn reindex' to rebuild the database")
	}
}

func isReadRuntimeConfigError(message string) bool {
	return strings.Contains(message, "vault config") || strings.Contains(message, "raven.yaml")
}

func formatSearchResults(results []model.SearchMatch) []map[string]interface{} {
	formatted := make([]map[string]interface{}, len(results))
	for i, r := range results {
		formatted[i] = map[string]interface{}{
			"object_id": r.ObjectID,
			"title":     r.Title,
			"file_path": r.FilePath,
			"snippet":   r.Snippet,
			"rank":      r.Rank,
		}
		if r.IsSection {
			formatted[i]["is_section"] = true
			formatted[i]["file_object_id"] = r.FileObjectID
			formatted[i]["line_start"] = r.LineStart
			formatted[i]["line_end"] = r.LineEnd
			formatted[i]["direct_line_end"] = r.LineEnd
			formatted[i]["subtree_line_end"] = r.SubtreeLineEnd
		}
	}
	return formatted
}
