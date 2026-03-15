package mcp

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/editsvc"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/traitsvc"
)

func (s *Server) callDirectReclassify(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, sch, normalized, errOut, isErr := s.directContext(args)
	if isErr {
		return errOut, true
	}

	objectRef := strings.TrimSpace(toString(normalized["object"]))
	if objectRef == "" {
		objectRef = strings.TrimSpace(toString(normalized["object-id"]))
	}
	newTypeName := strings.TrimSpace(toString(normalized["new-type"]))
	fieldValues, err := parseKeyValueInput(normalized["field"])
	if err != nil {
		return errorEnvelope("INVALID_INPUT", "invalid --field values", "Use --field key=value (repeatable)", nil), true
	}

	if objectRef == "" || newTypeName == "" {
		return errorEnvelope("MISSING_ARGUMENT", "requires object and new-type arguments", "Usage: rvn reclassify <object> <new-type>", nil), true
	}

	resolved, err := resolveReference(vaultPath, vaultCfg, sch, objectRef)
	if err != nil {
		var refErr *directRefError
		if errors.As(err, &refErr) {
			return errorEnvelope(refErr.Code, refErr.Message, refErr.Suggestion, nil), true
		}
		return errorEnvelope("REF_NOT_FOUND", err.Error(), "Check the object reference and run 'rvn reindex' if needed", nil), true
	}

	serviceResult, err := objectsvc.Reclassify(objectsvc.ReclassifyRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		Schema:       sch,
		ObjectRef:    objectRef,
		ObjectID:     resolved.ObjectID,
		FilePath:     resolved.FilePath,
		NewTypeName:  newTypeName,
		FieldValues:  fieldValues,
		NoMove:       boolValue(normalized["no-move"]),
		UpdateRefs:   boolValueDefault(normalized["update-refs"], true),
		Force:        boolValue(normalized["force"]),
		ParseOptions: parseOptionsFromVaultConfig(vaultCfg),
	})
	if err != nil {
		return mapDirectServiceError(err)
	}

	if serviceResult.ChangedFilePath != "" {
		maybeDirectReindexFile(vaultPath, serviceResult.ChangedFilePath, vaultCfg)
	}

	data := map[string]interface{}{
		"object_id":      serviceResult.ObjectID,
		"old_type":       serviceResult.OldType,
		"new_type":       serviceResult.NewType,
		"file":           serviceResult.File,
		"moved":          serviceResult.Moved,
		"old_path":       serviceResult.OldPath,
		"new_path":       serviceResult.NewPath,
		"updated_refs":   serviceResult.UpdatedRefs,
		"added_fields":   serviceResult.AddedFields,
		"dropped_fields": serviceResult.DroppedFields,
		"needs_confirm":  serviceResult.NeedsConfirm,
		"reason":         serviceResult.Reason,
	}

	warnings := warningMessagesToDirectWarnings(serviceResult.WarningMessages, "INDEX_UPDATE_FAILED")
	return successEnvelope(data, warnings), false
}

func (s *Server) callDirectEdit(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, sch, normalized, errOut, isErr := s.directContext(args)
	if isErr {
		return errOut, true
	}

	reference := strings.TrimSpace(toString(normalized["path"]))
	if reference == "" {
		reference = strings.TrimSpace(toString(normalized["reference"]))
	}
	if reference == "" {
		return errorEnvelope("MISSING_ARGUMENT", "requires reference argument", "Usage: rvn edit <reference> <old_str> <new_str> or --edits-json", nil), true
	}

	edits, batchMode, err := parseDirectEditInput(args, normalized)
	if err != nil {
		return mapDirectEditServiceError(err)
	}

	resolved, err := resolveReference(vaultPath, vaultCfg, sch, reference)
	if err != nil {
		var refErr *directRefError
		if errors.As(err, &refErr) {
			return errorEnvelope(refErr.Code, refErr.Message, refErr.Suggestion, nil), true
		}
		return errorEnvelope("REF_NOT_FOUND", err.Error(), "Check the reference and run 'rvn reindex' if needed", nil), true
	}

	content, err := os.ReadFile(resolved.FilePath)
	if err != nil {
		return errorEnvelope("READ_ERROR", err.Error(), "", nil), true
	}
	relPath, _ := filepath.Rel(vaultPath, resolved.FilePath)

	newContent, results, err := editsvc.ApplyEditsInMemory(string(content), relPath, edits)
	if err != nil {
		return mapDirectEditServiceError(err)
	}

	if !boolValue(normalized["confirm"]) {
		if batchMode {
			editsPreview := make([]map[string]interface{}, 0, len(results))
			for _, result := range results {
				editsPreview = append(editsPreview, map[string]interface{}{
					"index":   result.Index,
					"line":    result.Line,
					"old_str": result.OldStr,
					"new_str": result.NewStr,
					"preview": map[string]string{
						"before": result.Before,
						"after":  result.After,
					},
				})
			}
			return successEnvelope(map[string]interface{}{
				"status": "preview",
				"path":   relPath,
				"count":  len(editsPreview),
				"edits":  editsPreview,
			}, nil), false
		}

		result := results[0]
		return successEnvelope(map[string]interface{}{
			"status": "preview",
			"path":   relPath,
			"line":   result.Line,
			"preview": map[string]string{
				"before": result.Before,
				"after":  result.After,
			},
		}, nil), false
	}

	if err := atomicfile.WriteFile(resolved.FilePath, []byte(newContent), 0o644); err != nil {
		return errorEnvelope("WRITE_ERROR", err.Error(), "", nil), true
	}
	maybeDirectReindexFile(vaultPath, resolved.FilePath, vaultCfg)

	if batchMode {
		applied := make([]map[string]interface{}, 0, len(results))
		for _, result := range results {
			applied = append(applied, map[string]interface{}{
				"index":   result.Index,
				"line":    result.Line,
				"old_str": result.OldStr,
				"new_str": result.NewStr,
				"context": result.Context,
			})
		}
		return successEnvelope(map[string]interface{}{
			"status": "applied",
			"path":   relPath,
			"count":  len(applied),
			"edits":  applied,
		}, nil), false
	}

	result := results[0]
	return successEnvelope(map[string]interface{}{
		"status":  "applied",
		"path":    relPath,
		"line":    result.Line,
		"old_str": result.OldStr,
		"new_str": result.NewStr,
		"context": result.Context,
	}, nil), false
}

func (s *Server) callDirectUpdate(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, sch, normalized, errOut, isErr := s.directContext(args)
	if isErr {
		return errOut, true
	}

	newValue := strings.TrimSpace(toString(normalized["value"]))
	if newValue == "" {
		return errorEnvelope("MISSING_ARGUMENT", "no value specified", "Usage: rvn update <trait_id> <new_value>", nil), true
	}

	stdinMode := boolValue(normalized["stdin"])
	traitIDs := extractTraitUpdateIDs(normalized, stdinMode)
	confirm := boolValue(normalized["confirm"])

	if !stdinMode {
		singleID := strings.TrimSpace(toString(normalized["trait_id"]))
		if singleID == "" {
			return errorEnvelope("MISSING_ARGUMENT", "requires trait-id and new value arguments", "Usage: rvn update <trait_id> <new_value>", nil), true
		}
		if !strings.Contains(singleID, ":trait:") {
			return errorEnvelope("INVALID_INPUT", "invalid trait ID format", "Trait IDs look like: path/file.md:trait:N", nil), true
		}
		traitIDs = []string{singleID}
		confirm = true
	}

	if len(traitIDs) == 0 {
		return errorEnvelope("MISSING_ARGUMENT", "no trait IDs provided via stdin", "Provide trait IDs via trait_ids when using stdin mode", nil), true
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		return errorEnvelope("DATABASE_ERROR", err.Error(), "Run 'rvn reindex' to rebuild the database", nil), true
	}
	defer db.Close()
	db.SetDailyDirectory(vaultCfg.GetDailyDirectory())

	traits, skipped, err := traitsvc.ResolveTraitIDs(db, traitIDs)
	if err != nil {
		return mapDirectTraitServiceError(err)
	}

	if !confirm {
		preview, err := traitsvc.BuildPreview(traits, newValue, sch, skipped)
		if err != nil {
			return mapDirectTraitServiceError(err)
		}
		return successEnvelope(map[string]interface{}{
			"preview": true,
			"action":  preview.Action,
			"items":   preview.Items,
			"skipped": preview.Skipped,
			"total":   preview.Total,
		}, nil), false
	}

	summary, err := traitsvc.ApplyUpdates(vaultPath, traits, newValue, sch, skipped)
	if err != nil {
		return mapDirectTraitServiceError(err)
	}

	reindexed := make(map[string]struct{}, len(summary.ChangedFilePaths))
	for _, filePath := range summary.ChangedFilePaths {
		if filePath == "" {
			continue
		}
		if _, seen := reindexed[filePath]; seen {
			continue
		}
		reindexed[filePath] = struct{}{}
		maybeDirectReindexFile(vaultPath, filePath, vaultCfg)
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

func parseDirectEditInput(args map[string]interface{}, normalized map[string]interface{}) ([]editsvc.EditSpec, bool, error) {
	if hasAnyArg(args, "edits-json", "edits_json") {
		raw := normalized["edits-json"]
		var payload string
		switch v := raw.(type) {
		case string:
			payload = v
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return nil, false, &editsvc.Error{
					Code:       editsvc.CodeInvalidInput,
					Message:    "invalid --edits-json payload",
					Suggestion: `Provide an object like: {"edits":[{"old_str":"from","new_str":"to"}]}`,
					Details:    map[string]string{"error": err.Error()},
					Err:        err,
				}
			}
			payload = string(b)
		}

		edits, err := editsvc.ParseEditsJSON(strings.TrimSpace(payload))
		if err != nil {
			return nil, false, err
		}
		return edits, true, nil
	}

	hasOld := hasAnyArg(args, "old-str", "old_str")
	hasNew := hasAnyArg(args, "new-str", "new_str")
	if !hasOld || !hasNew {
		return nil, false, &editsvc.Error{
			Code:       editsvc.CodeInvalidInput,
			Message:    "requires old_str and new_str when --edits-json is not provided",
			Suggestion: "Usage: rvn edit <reference> <old_str> <new_str> or --edits-json",
		}
	}

	return []editsvc.EditSpec{{
		OldStr: toString(normalized["old-str"]),
		NewStr: toString(normalized["new-str"]),
	}}, false, nil
}

func mapDirectEditServiceError(err error) (string, bool) {
	if svcErr, ok := editsvc.AsError(err); ok {
		var details map[string]interface{}
		if len(svcErr.Details) > 0 {
			details = make(map[string]interface{}, len(svcErr.Details))
			for k, v := range svcErr.Details {
				details[k] = v
			}
		}
		return errorEnvelope(string(svcErr.Code), svcErr.Message, svcErr.Suggestion, details), true
	}
	return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
}

func mapDirectTraitServiceError(err error) (string, bool) {
	var validationErr *traitsvc.ValueValidationError
	if errors.As(err, &validationErr) {
		return errorEnvelope("VALIDATION_FAILED", validationErr.Error(), validationErr.Suggestion(), nil), true
	}
	if svcErr, ok := traitsvc.AsError(err); ok {
		return errorEnvelope(string(svcErr.Code), svcErr.Message, svcErr.Suggestion, svcErr.Details), true
	}
	return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
}

func extractTraitUpdateIDs(args map[string]interface{}, stdinMode bool) []string {
	collected := make([]string, 0)

	appendIDs := func(v interface{}) {
		switch val := v.(type) {
		case string:
			for _, line := range strings.Split(val, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if strings.Contains(line, ",") && !strings.Contains(line, "\t") {
					for _, part := range strings.Split(line, ",") {
						part = strings.TrimSpace(part)
						if part == "" {
							continue
						}
						collected = append(collected, extractIDFromPipeLine(part))
					}
					continue
				}
				collected = append(collected, extractIDFromPipeLine(line))
			}
		default:
			for _, raw := range stringSliceValues(v) {
				id := extractIDFromPipeLine(raw)
				if strings.TrimSpace(id) == "" {
					continue
				}
				collected = append(collected, id)
			}
		}
	}

	appendIDs(args["trait-ids"])
	appendIDs(args["trait_ids"])
	appendIDs(args["ids"])
	if stdinMode {
		appendIDs(args["trait-id"])
		appendIDs(args["trait_id"])
	}

	seen := make(map[string]struct{}, len(collected))
	ids := make([]string, 0, len(collected))
	for _, id := range collected {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if !strings.Contains(id, ":trait:") {
			continue
		}
		ids = append(ids, id)
	}

	return ids
}
