package mcp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/readsvc"
)

func (s *Server) callDirectSearch(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}

	normalized := normalizeArgs(args)
	queryStr := strings.TrimSpace(toString(normalized["query"]))
	if queryStr == "" {
		return errorEnvelope("MISSING_ARGUMENT", "requires query argument", "Usage: rvn search <query>", nil), true
	}

	limit := intValueDefault(normalized["limit"], 20)
	searchType := strings.TrimSpace(toString(normalized["type"]))

	rt, err := readsvc.NewRuntime(vaultPath, readsvc.RuntimeOptions{OpenDB: true})
	if err != nil {
		return errorEnvelope("DATABASE_ERROR", "failed to open database", "Run 'rvn reindex' to rebuild the database", nil), true
	}
	defer rt.Close()

	results, err := readsvc.Search(rt, queryStr, searchType, limit)
	if err != nil {
		return errorEnvelope("DATABASE_ERROR", fmt.Sprintf("search failed: %v", err), "", nil), true
	}

	data := map[string]interface{}{
		"query":   queryStr,
		"results": formatSearchMatches(results),
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectBacklinks(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}

	normalized := normalizeArgs(args)
	reference := strings.TrimSpace(toString(normalized["target"]))
	if reference == "" {
		return errorEnvelope("MISSING_ARGUMENT", "requires target argument", "Usage: rvn backlinks <target>", nil), true
	}

	rt, err := readsvc.NewRuntime(vaultPath, readsvc.RuntimeOptions{OpenDB: true})
	if err != nil {
		return errorEnvelope("DATABASE_ERROR", "failed to open database", "Run 'rvn reindex' to rebuild the database", nil), true
	}
	defer rt.Close()

	resolved, err := readsvc.ResolveReferenceWithDynamicDates(reference, rt, true)
	if err != nil {
		return mapDirectResolveError(err, reference)
	}

	links, err := readsvc.Backlinks(rt, resolved.ObjectID)
	if err != nil {
		return errorEnvelope("DATABASE_ERROR", fmt.Sprintf("failed to read backlinks: %v", err), "", nil), true
	}
	data := map[string]interface{}{
		"target": resolved.ObjectID,
		"items":  links,
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectOutlinks(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}

	normalized := normalizeArgs(args)
	reference := strings.TrimSpace(toString(normalized["source"]))
	if reference == "" {
		return errorEnvelope("MISSING_ARGUMENT", "requires source argument", "Usage: rvn outlinks <source>", nil), true
	}

	rt, err := readsvc.NewRuntime(vaultPath, readsvc.RuntimeOptions{OpenDB: true})
	if err != nil {
		return errorEnvelope("DATABASE_ERROR", "failed to open database", "Run 'rvn reindex' to rebuild the database", nil), true
	}
	defer rt.Close()

	resolved, err := readsvc.ResolveReferenceWithDynamicDates(reference, rt, true)
	if err != nil {
		return mapDirectResolveError(err, reference)
	}

	links, err := readsvc.Outlinks(rt, resolved.ObjectID)
	if err != nil {
		return errorEnvelope("DATABASE_ERROR", fmt.Sprintf("failed to read outlinks: %v", err), "", nil), true
	}
	data := map[string]interface{}{
		"source": resolved.ObjectID,
		"items":  links,
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectResolve(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}

	normalized := normalizeArgs(args)
	reference := strings.TrimSpace(toString(normalized["reference"]))
	if reference == "" {
		return errorEnvelope("MISSING_ARGUMENT", "requires reference argument", "Usage: rvn resolve <reference>", nil), true
	}

	rt, err := readsvc.NewRuntime(vaultPath, readsvc.RuntimeOptions{OpenDB: true})
	if err != nil {
		return errorEnvelope("DATABASE_ERROR", "failed to open database", "Run 'rvn reindex' to rebuild the database", nil), true
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

		return successEnvelope(map[string]interface{}{
			"resolved":  false,
			"ambiguous": true,
			"reference": reference,
			"matches":   matches,
		}, nil), false
	}

	if err != nil {
		return successEnvelope(map[string]interface{}{
			"resolved":  false,
			"reference": reference,
		}, nil), false
	}

	relPath := resolved.FilePath
	if rel, relErr := filepath.Rel(vaultPath, resolved.FilePath); relErr == nil {
		relPath = rel
	}

	objectType := ""
	if obj, objErr := rt.DB.GetObject(resolved.ObjectID); objErr == nil && obj != nil {
		objectType = obj.Type
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
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectQuery(args map[string]interface{}) (string, bool) {
	normalized := normalizeArgs(args)
	listMode := boolValue(normalized["list"])
	queryString := strings.TrimSpace(toString(normalized["query_string"]))
	if !listMode && queryString == "" {
		return errorEnvelope("MISSING_ARGUMENT", "specify a query string", "Run 'rvn query --list' to see saved queries", nil), true
	}

	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}

	rt, err := readsvc.NewRuntime(vaultPath, readsvc.RuntimeOptions{OpenDB: true})
	if err != nil {
		return errorEnvelope("DATABASE_ERROR", "failed to open database", "Run 'rvn reindex' to rebuild the database", nil), true
	}
	defer rt.Close()

	if listMode {
		return successEnvelope(map[string]interface{}{
			"queries": directSavedQueriesList(rt.VaultCfg),
		}, nil), false
	}

	resolvedQuery, queryName, isSavedQuery, err := directResolveQueryString(queryString, normalized["inputs"], rt.VaultCfg)
	if err != nil {
		return errorEnvelope("QUERY_INVALID", err.Error(), "", nil), true
	}

	limit := intValueDefault(normalized["limit"], 0)
	offset := intValueDefault(normalized["offset"], 0)
	idsOnly := boolValue(normalized["ids"])
	countOnly := boolValue(normalized["count-only"])
	confirm := boolValue(normalized["confirm"])
	applyArgs := stringSliceValues(normalized["apply"])

	if limit < 0 {
		return errorEnvelope("INVALID_INPUT", "--limit must be >= 0", "Use --limit 0 for no limit", nil), true
	}
	if offset < 0 {
		return errorEnvelope("INVALID_INPUT", "--offset must be >= 0", "Use --offset 0 for no offset", nil), true
	}

	if len(applyArgs) > 0 && (limit > 0 || offset > 0 || countOnly) {
		return errorEnvelope(
			"INVALID_INPUT",
			"--limit, --offset, and --count-only cannot be used with --apply",
			"Remove pagination/count-only flags when using --apply",
			nil,
		), true
	}

	if boolValue(normalized["refresh"]) {
		if _, err := readsvc.SmartReindex(rt); err != nil {
			return errorEnvelope("DATABASE_ERROR", fmt.Sprintf("failed to refresh index: %v", err), "Run 'rvn reindex' to rebuild the database", nil), true
		}
	} else {
		// JSON mode doesn't emit staleness warnings; we still run the check to keep shared path exercised.
		_, _, _ = readsvc.CheckStaleness(rt)
	}

	result, err := readsvc.ExecuteQuery(rt, readsvc.ExecuteQueryRequest{
		QueryString: resolvedQuery,
		IDsOnly:     idsOnly,
		Limit:       limit,
		Offset:      offset,
		CountOnly:   countOnly,
	})
	if err != nil {
		var validationErr *query.ValidationError
		if errors.As(err, &validationErr) {
			return errorEnvelope("QUERY_INVALID", validationErr.Message, validationErr.Suggestion, nil), true
		}
		return errorEnvelope("QUERY_INVALID", fmt.Sprintf("parse error: %v", err), "", nil), true
	}

	if len(applyArgs) > 0 {
		return s.callDirectQueryApply(vaultPath, rt, resolvedQuery, result, applyArgs, confirm)
	}

	if countOnly {
		key := "type"
		if result.QueryType == "trait" {
			key = "trait"
		}
		return successEnvelope(map[string]interface{}{
			"query_type": result.QueryType,
			key:          result.TypeName,
			"total":      result.Total,
		}, nil), false
	}

	if idsOnly {
		return successEnvelope(map[string]interface{}{
			"ids":      result.IDs,
			"total":    result.Total,
			"returned": result.Returned,
			"offset":   result.Offset,
			"limit":    result.Limit,
		}, nil), false
	}

	if result.QueryType == "object" {
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
		typeKey := "type"
		typeVal := result.TypeName
		if isSavedQuery && queryName != "" {
			typeKey = "saved_query"
			typeVal = queryName
		}
		return successEnvelope(map[string]interface{}{
			"query_type": result.QueryType,
			typeKey:      typeVal,
			"items":      items,
			"total":      result.Total,
			"returned":   result.Returned,
			"offset":     result.Offset,
			"limit":      result.Limit,
		}, nil), false
	}

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
	typeKey := "trait"
	typeVal := result.TypeName
	if isSavedQuery && queryName != "" {
		typeKey = "saved_query"
		typeVal = queryName
	}
	return successEnvelope(map[string]interface{}{
		"query_type": result.QueryType,
		typeKey:      typeVal,
		"items":      items,
		"total":      result.Total,
		"returned":   result.Returned,
		"offset":     result.Offset,
		"limit":      result.Limit,
	}, nil), false
}

func (s *Server) callDirectQueryApply(
	vaultPath string,
	rt *readsvc.Runtime,
	resolvedQuery string,
	result *readsvc.ExecuteQueryResult,
	applyArgs []string,
	confirm bool,
) (string, bool) {
	applyStr := strings.Join(applyArgs, " ")
	applyParts := strings.Fields(applyStr)
	if len(applyParts) == 0 {
		return errorEnvelope("INVALID_INPUT", "no apply command specified", "Use --apply <command> [args...]", nil), true
	}

	applyCmd := applyParts[0]
	applyOpArgs := applyParts[1:]

	if result.QueryType == "trait" {
		if applyCmd != "update" {
			return errorEnvelope(
				"INVALID_INPUT",
				fmt.Sprintf("'%s' is not supported for trait queries", applyCmd),
				"For trait queries, use: --apply \"update <new_value>\"",
				nil,
			), true
		}
		newValue := strings.TrimSpace(strings.Join(applyOpArgs, " "))
		if newValue == "" {
			return errorEnvelope("MISSING_ARGUMENT", "no value specified", "Usage: --apply \"update <new_value>\"", nil), true
		}

		if !confirm {
			preview, err := readsvc.PreviewTraitUpdate(result.Traits, newValue, rt.Schema)
			if err != nil {
				var valErr *readsvc.TraitValueValidationError
				if errors.As(err, &valErr) {
					return errorEnvelope("VALIDATION_FAILED", valErr.Error(), valErr.Suggestion(), nil), true
				}
				return errorEnvelope("VALIDATION_FAILED", err.Error(), "", nil), true
			}

			return successEnvelope(map[string]interface{}{
				"preview": true,
				"action":  preview.Action,
				"items":   preview.Items,
				"skipped": preview.Skipped,
				"total":   preview.Total,
			}, nil), false
		}

		summary, err := readsvc.ApplyTraitUpdate(vaultPath, result.Traits, newValue, rt.Schema, func(filePath string) {
			maybeDirectReindexFile(vaultPath, filePath, rt.VaultCfg)
		})
		if err != nil {
			var valErr *readsvc.TraitValueValidationError
			if errors.As(err, &valErr) {
				return errorEnvelope("VALIDATION_FAILED", valErr.Error(), valErr.Suggestion(), nil), true
			}
			return errorEnvelope("VALIDATION_FAILED", err.Error(), "", nil), true
		}

		return successEnvelope(map[string]interface{}{
			"action":   summary.Action,
			"results":  summary.Results,
			"total":    summary.Total,
			"modified": summary.Modified,
			"skipped":  summary.Skipped,
			"errors":   summary.Errors,
		}, nil), false
	}

	ids := make([]string, 0, len(result.Objects))
	for _, row := range result.Objects {
		ids = append(ids, row.ID)
	}
	ids = dedupeIDs(ids)

	if len(ids) == 0 {
		return successEnvelope(map[string]interface{}{
			"preview": !confirm,
			"action":  applyCmd,
			"items":   []interface{}{},
			"total":   0,
		}, nil), false
	}

	fileIDs := make([]string, 0, len(ids))
	embeddedIDs := make([]string, 0)
	for _, id := range ids {
		if strings.Contains(id, "#") {
			embeddedIDs = append(embeddedIDs, id)
			continue
		}
		fileIDs = append(fileIDs, id)
	}

	switch applyCmd {
	case "set":
		updates, err := parseApplySetArgs(applyOpArgs)
		if err != nil {
			return errorEnvelope("INVALID_INPUT", err.Error(), "Use format: field=value", nil), true
		}
		return s.callDirectSetBulk(vaultPath, rt.VaultCfg, rt.Schema, ids, updates, map[string]interface{}{
			"confirm": confirm,
		})
	case "delete":
		return s.callDirectDeleteBulk(vaultPath, rt.VaultCfg, fileIDs, embeddedIDs, map[string]interface{}{
			"confirm": confirm,
		})
	case "add":
		if len(applyOpArgs) == 0 {
			return errorEnvelope("MISSING_ARGUMENT", "no text to add", "Usage: --apply add <text>", nil), true
		}
		return s.callDirectAddBulk(vaultPath, rt.VaultCfg, rt.Schema, fileIDs, strings.Join(applyOpArgs, " "), "", map[string]interface{}{
			"confirm": confirm,
		})
	case "move":
		if len(applyOpArgs) == 0 {
			return errorEnvelope("MISSING_ARGUMENT", "no destination provided", "Usage: --apply move <destination-directory/>", nil), true
		}
		return s.callDirectMoveBulk(vaultPath, rt.VaultCfg, rt.Schema, applyOpArgs[0], fileIDs, embeddedIDs, map[string]interface{}{
			"confirm":     confirm,
			"update-refs": true,
		})
	default:
		return errorEnvelope("INVALID_INPUT", fmt.Sprintf("unknown apply command: %s", applyCmd), "Supported commands: set, delete, add, move", nil), true
	}
}

func parseApplySetArgs(args []string) (map[string]string, error) {
	updates := make(map[string]string)
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid field format: %s", arg)
		}
		updates[parts[0]] = parts[1]
	}
	if len(updates) == 0 {
		return nil, fmt.Errorf("no fields to set")
	}
	return updates, nil
}

func (s *Server) callDirectRead(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	reference := strings.TrimSpace(toString(normalized["path"]))
	if reference == "" {
		return errorEnvelope("MISSING_ARGUMENT", "requires path argument", "Usage: rvn read <reference>", nil), true
	}

	rt, err := readsvc.NewRuntime(vaultPath, readsvc.RuntimeOptions{OpenDB: false})
	if err != nil {
		return errorEnvelope("CONFIG_INVALID", "failed to load vault config", "Fix raven.yaml and try again", nil), true
	}
	defer rt.Close()

	result, err := readsvc.Read(rt, readsvc.ReadRequest{
		Reference: reference,
		Raw:       boolValue(normalized["raw"]),
		Lines:     boolValue(normalized["lines"]),
		StartLine: intValueDefault(normalized["start-line"], 0),
		EndLine:   intValueDefault(normalized["end-line"], 0),
	})
	if err != nil {
		var ambiguous *readsvc.AmbiguousRefError
		if errors.As(err, &ambiguous) {
			return errorEnvelope("REF_AMBIGUOUS", ambiguous.Error(), "Use a full object ID/path to disambiguate", nil), true
		}

		var notFound *readsvc.RefNotFoundError
		if errors.As(err, &notFound) {
			return errorEnvelope("REF_NOT_FOUND", notFound.Error(), "Check the reference and try again", nil), true
		}

		var invalidRange *readsvc.InvalidLineRangeError
		if errors.As(err, &invalidRange) {
			return errorEnvelope("INVALID_INPUT", invalidRange.Error(), invalidRange.Suggestion(), nil), true
		}

		if os.IsNotExist(err) {
			return errorEnvelope("FILE_NOT_FOUND", err.Error(), "Check the path and try again", nil), true
		}
		return errorEnvelope("FILE_READ_ERROR", err.Error(), "", nil), true
	}

	data := map[string]interface{}{
		"path":       result.Path,
		"content":    result.Content,
		"line_count": result.LineCount,
	}

	rawMode := boolValue(normalized["raw"]) ||
		boolValue(normalized["lines"]) ||
		intValueDefault(normalized["start-line"], 0) > 0 ||
		intValueDefault(normalized["end-line"], 0) > 0

	if rawMode {
		if result.StartLine > 0 {
			data["start_line"] = result.StartLine
			data["end_line"] = result.EndLine
		}
		if len(result.Lines) > 0 {
			lines := make([]map[string]interface{}, 0, len(result.Lines))
			for _, line := range result.Lines {
				lines = append(lines, map[string]interface{}{
					"num":  line.Num,
					"text": line.Text,
				})
			}
			data["lines"] = lines
		}
		return successEnvelope(data, nil), false
	}

	refs := make([]map[string]interface{}, 0, len(result.References))
	for _, ref := range result.References {
		entry := map[string]interface{}{
			"text": ref.Text,
		}
		if ref.Path != nil {
			entry["path"] = *ref.Path
		}
		refs = append(refs, entry)
	}

	backlinks := make([]map[string]interface{}, 0, len(result.Backlinks))
	for _, group := range result.Backlinks {
		backlinks = append(backlinks, map[string]interface{}{
			"source": group.Source,
			"lines":  group.Lines,
		})
	}

	data["references"] = refs
	data["backlinks"] = backlinks
	return successEnvelope(data, nil), false
}

func directSavedQueriesList(vaultCfg *config.VaultConfig) []map[string]interface{} {
	if vaultCfg == nil || len(vaultCfg.Queries) == 0 {
		return []map[string]interface{}{}
	}
	names := make([]string, 0, len(vaultCfg.Queries))
	for name := range vaultCfg.Queries {
		names = append(names, name)
	}
	sort.Strings(names)

	queries := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		q := vaultCfg.Queries[name]
		queries = append(queries, map[string]interface{}{
			"name":        name,
			"query":       q.Query,
			"args":        q.Args,
			"description": q.Description,
		})
	}
	return queries
}

func directResolveQueryString(queryString string, rawInputs interface{}, vaultCfg *config.VaultConfig) (resolved, queryName string, isSaved bool, err error) {
	if vaultCfg == nil || len(vaultCfg.Queries) == 0 {
		return queryString, "", false, nil
	}

	tokens := strings.Fields(queryString)
	if len(tokens) == 0 {
		return "", "", false, fmt.Errorf("empty query string")
	}

	name := tokens[0]
	saved, ok := vaultCfg.Queries[name]
	if !ok {
		return queryString, "", false, nil
	}

	inlineArgs := tokens[1:]
	declaredArgs := directNormalizeSavedQueryArgs(saved.Args)
	inputs, err := directParseSavedQueryInputs(inlineArgs, rawInputs, declaredArgs)
	if err != nil {
		return "", "", true, err
	}
	resolvedQuery, err := directResolveSavedQueryTemplate(name, saved.Query, inputs)
	if err != nil {
		return "", "", true, err
	}
	return resolvedQuery, name, true, nil
}

func directNormalizeSavedQueryArgs(args []string) []string {
	out := make([]string, 0, len(args))
	seen := make(map[string]struct{}, len(args))
	for _, arg := range args {
		name := strings.TrimSpace(arg)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func directParseSavedQueryInputs(inlineArgs []string, rawInputs interface{}, declaredArgs []string) (map[string]string, error) {
	inputs := make(map[string]string)
	positional := make([]string, 0)

	addToken := func(token string) {
		if strings.Contains(token, "=") {
			parts := strings.SplitN(token, "=", 2)
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if key != "" {
				inputs[key] = val
			}
			return
		}
		positional = append(positional, token)
	}

	for _, token := range inlineArgs {
		addToken(token)
	}
	for _, pair := range keyValuePairs(rawInputs) {
		addToken(pair)
	}

	if len(positional) > len(declaredArgs) {
		return nil, fmt.Errorf("too many positional inputs: got %d, expected %d", len(positional), len(declaredArgs))
	}

	for i, value := range positional {
		argName := declaredArgs[i]
		if _, exists := inputs[argName]; exists {
			return nil, fmt.Errorf("input '%s' specified twice", argName)
		}
		inputs[argName] = value
	}

	for _, argName := range declaredArgs {
		if _, ok := inputs[argName]; !ok {
			return nil, fmt.Errorf("missing required input '%s'", argName)
		}
	}

	return inputs, nil
}

func directResolveSavedQueryTemplate(name, queryString string, inputs map[string]string) (string, error) {
	re := regexp.MustCompile(`\{\{\s*args\.([a-zA-Z0-9_-]+)\s*\}\}`)
	matches := re.FindAllStringSubmatch(queryString, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		argName := match[1]
		val, ok := inputs[argName]
		if !ok {
			return "", fmt.Errorf("saved query '%s' is missing input '%s'", name, argName)
		}
		queryString = strings.ReplaceAll(queryString, match[0], val)
	}
	return queryString, nil
}

func dedupeIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
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
