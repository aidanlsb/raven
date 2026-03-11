package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

type directWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Ref     string `json:"ref,omitempty"`
}

type resolvedReference struct {
	ObjectID     string
	FileObjectID string
	FilePath     string
	IsSection    bool
}

type directRefError struct {
	Code       string
	Message    string
	Suggestion string
}

func (e *directRefError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (s *Server) callToolDirect(name string, args map[string]interface{}) (string, bool, bool) {
	switch name {
	case "raven_new":
		out, isErr := s.callDirectNew(args)
		return out, isErr, true
	case "raven_upsert":
		out, isErr := s.callDirectUpsert(args)
		return out, isErr, true
	case "raven_add":
		out, isErr := s.callDirectAdd(args)
		return out, isErr, true
	case "raven_set":
		out, isErr := s.callDirectSet(args)
		return out, isErr, true
	case "raven_delete":
		out, isErr := s.callDirectDelete(args)
		return out, isErr, true
	case "raven_move":
		out, isErr := s.callDirectMove(args)
		return out, isErr, true
	default:
		return "", false, false
	}
}

func (s *Server) callDirectNew(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, sch, normalized, errOut, isErr := s.directContext(args)
	if isErr {
		return errOut, true
	}

	typeName := toString(normalized["type"])
	title := toString(normalized["title"])
	targetPath := toString(normalized["path"])
	templateID := toString(normalized["template"])
	if targetPath == "" {
		targetPath = title
	}

	fieldValues, err := parseKeyValueInput(normalized["field"])
	if err != nil {
		return errorEnvelope("INVALID_INPUT", "invalid --field payload", err.Error(), nil), true
	}

	typedFieldValues, err := parseTypedFieldValues(normalized["field-json"])
	if err != nil {
		return errorEnvelope("INVALID_INPUT", "invalid --field-json payload", "Provide a JSON object, e.g. --field-json '{\"status\":\"active\"}'", nil), true
	}

	result, err := objectsvc.Create(objectsvc.CreateRequest{
		VaultPath:        vaultPath,
		TypeName:         typeName,
		Title:            title,
		TargetPath:       targetPath,
		FieldValues:      fieldValues,
		TypedFieldValues: typedFieldValues,
		Schema:           sch,
		ObjectsRoot:      vaultCfg.GetObjectsRoot(),
		PagesRoot:        vaultCfg.GetPagesRoot(),
		TemplateDir:      vaultCfg.GetTemplateDirectory(),
		TemplateID:       templateID,
	})
	if err != nil {
		return mapDirectServiceError(err)
	}

	maybeDirectReindexFile(vaultPath, result.FilePath, vaultCfg)

	return successEnvelope(map[string]interface{}{
		"file":  result.RelativePath,
		"id":    vaultCfg.FilePathToObjectID(result.RelativePath),
		"title": title,
		"type":  typeName,
	}, nil), false
}

func (s *Server) callDirectUpsert(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, sch, normalized, errOut, isErr := s.directContext(args)
	if isErr {
		return errOut, true
	}

	typeName := toString(normalized["type"])
	title := toString(normalized["title"])
	targetPath := title
	if v := toString(normalized["path"]); v != "" {
		targetPath = v
	}

	fieldValues, err := parseKeyValueInput(normalized["field"])
	if err != nil {
		return errorEnvelope("INVALID_INPUT", "invalid --field payload", err.Error(), nil), true
	}

	typedFieldValues, err := parseTypedFieldValues(normalized["field-json"])
	if err != nil {
		return errorEnvelope("INVALID_INPUT", "invalid --field-json payload", "Provide a JSON object, e.g. --field-json '{\"status\":\"active\"}'", nil), true
	}

	contentVal, hasContent := normalized["content"]
	content := toString(contentVal)

	result, err := objectsvc.Upsert(objectsvc.UpsertRequest{
		VaultPath:        vaultPath,
		TypeName:         typeName,
		Title:            title,
		TargetPath:       targetPath,
		ReplaceBody:      hasContent,
		Content:          content,
		FieldValues:      fieldValues,
		TypedFieldValues: typedFieldValues,
		Schema:           sch,
		ObjectsRoot:      vaultCfg.GetObjectsRoot(),
		PagesRoot:        vaultCfg.GetPagesRoot(),
		TemplateDir:      vaultCfg.GetTemplateDirectory(),
	})
	if err != nil {
		return mapDirectServiceError(err)
	}

	if result.Status == "created" || result.Status == "updated" {
		maybeDirectReindexFile(vaultPath, result.FilePath, vaultCfg)
	}

	return successEnvelope(map[string]interface{}{
		"status": result.Status,
		"id":     vaultCfg.FilePathToObjectID(result.RelativePath),
		"file":   result.RelativePath,
		"type":   typeName,
		"title":  title,
	}, warningMessagesToDirectWarnings(result.WarningMessages, "UNKNOWN_FIELD")), false
}

func (s *Server) callDirectAdd(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, sch, normalized, errOut, isErr := s.directContext(args)
	if isErr {
		return errOut, true
	}

	text := toString(normalized["text"])
	if strings.TrimSpace(text) == "" {
		return errorEnvelope("MISSING_ARGUMENT", "requires text argument", "Usage: rvn add <text>", nil), true
	}
	line := text
	headingSpec := strings.TrimSpace(toString(normalized["heading"]))

	stdinMode := boolValue(normalized["stdin"])
	objectIDs := extractAddObjectIDs(normalized, stdinMode)
	if stdinMode || len(objectIDs) > 0 {
		return s.callDirectAddBulk(vaultPath, vaultCfg, sch, objectIDs, line, headingSpec, normalized)
	}

	parseOpts := parseOptionsFromVaultConfig(vaultCfg)
	captureCfg := vaultCfg.GetCaptureConfig()

	var destPath string
	var isDailyNote bool
	var targetObjectID string
	var fileObjectID string

	if toRef := strings.TrimSpace(toString(normalized["to"])); toRef != "" {
		resolved, daily, err := resolveAddDestination(vaultPath, vaultCfg, sch, toRef)
		if err != nil {
			var refErr *directRefError
			if errors.As(err, &refErr) {
				return errorEnvelope(refErr.Code, refErr.Message, refErr.Suggestion, nil), true
			}
			return errorEnvelope("REF_NOT_FOUND", err.Error(), "Check the destination reference and run 'rvn reindex' if needed", nil), true
		}
		destPath = resolved.FilePath
		targetObjectID = resolved.ObjectID
		fileObjectID = resolved.FileObjectID
		isDailyNote = daily
	} else if captureCfg.Destination == "daily" {
		today := vault.FormatDateISO(time.Now())
		destPath = vaultCfg.DailyNotePath(vaultPath, today)
		fileObjectID = vaultCfg.DailyNoteID(today)
		isDailyNote = true
	} else {
		destPath = filepath.Join(vaultPath, captureCfg.Destination)
		fileObjectID = vaultCfg.FilePathToObjectID(captureCfg.Destination)
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			return errorEnvelope("FILE_NOT_FOUND", fmt.Sprintf("Configured capture destination '%s' does not exist", captureCfg.Destination), "Create the file first or change capture.destination in raven.yaml", nil), true
		}
	}

	if err := paths.ValidateWithinVault(vaultPath, destPath); err != nil {
		if errors.Is(err, paths.ErrPathOutsideVault) {
			return errorEnvelope("FILE_OUTSIDE_VAULT", fmt.Sprintf("cannot capture outside vault: %s", destPath), "", nil), true
		}
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	if headingSpec != "" {
		if targetObjectID != "" && strings.Contains(targetObjectID, "#") {
			return errorEnvelope("INVALID_INPUT", "cannot combine --heading with a section reference in --to", "Use either --to <file#section> or --heading", nil), true
		}
		resolvedTarget, err := objectsvc.ResolveAddHeadingTarget(vaultPath, destPath, fileObjectID, headingSpec, parseOpts)
		if err != nil {
			return errorEnvelope("REF_NOT_FOUND", err.Error(), "Use an existing section slug/id or heading text", nil), true
		}
		targetObjectID = resolvedTarget
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return errorEnvelope("FILE_WRITE_ERROR", err.Error(), "", nil), true
	}

	if err := objectsvc.AppendToFile(vaultPath, destPath, line, captureCfg, vaultCfg, isDailyNote, targetObjectID, parseOpts); err != nil {
		return errorEnvelope("FILE_WRITE_ERROR", err.Error(), "", nil), true
	}

	maybeDirectReindexFile(vaultPath, destPath, vaultCfg)
	relPath, _ := filepath.Rel(vaultPath, destPath)
	return successEnvelope(map[string]interface{}{
		"file":    relPath,
		"line":    objectsvc.FileLineCount(destPath),
		"content": line,
	}, nil), false
}

func (s *Server) callDirectAddBulk(
	vaultPath string,
	vaultCfg *config.VaultConfig,
	_ *schema.Schema,
	objectIDs []string,
	line string,
	headingSpec string,
	normalized map[string]interface{},
) (string, bool) {
	if len(objectIDs) == 0 {
		return errorEnvelope("MISSING_ARGUMENT", "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line", nil), true
	}

	confirm := boolValue(normalized["confirm"])
	parseOpts := parseOptionsFromVaultConfig(vaultCfg)
	request := objectsvc.AddBulkRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		ObjectIDs:    objectIDs,
		Line:         line,
		HeadingSpec:  headingSpec,
		ParseOptions: parseOpts,
	}

	if !confirm {
		previewResult, err := objectsvc.PreviewAddBulk(request)
		if err != nil {
			return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
		}
		items := make([]map[string]interface{}, 0, len(previewResult.Items))
		skipped := make([]map[string]interface{}, 0, len(previewResult.Skipped))
		for _, item := range previewResult.Items {
			items = append(items, map[string]interface{}{
				"id":      item.ID,
				"action":  item.Action,
				"details": item.Details,
			})
		}
		for _, result := range previewResult.Skipped {
			skipped = append(skipped, map[string]interface{}{
				"id":     result.ID,
				"status": result.Status,
				"reason": result.Reason,
			})
		}

		return successEnvelope(map[string]interface{}{
			"preview":  true,
			"action":   "add",
			"items":    items,
			"skipped":  skipped,
			"total":    previewResult.Total,
			"warnings": nil,
			"content":  line,
		}, nil), false
	}

	bulkResult, err := objectsvc.ApplyAddBulk(request, func(filePath string) {
		maybeDirectReindexFile(vaultPath, filePath, vaultCfg)
	})
	if err != nil {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}
	results := make([]map[string]interface{}, 0, len(bulkResult.Results))
	for _, result := range bulkResult.Results {
		entry := map[string]interface{}{
			"id":     result.ID,
			"status": result.Status,
		}
		if strings.TrimSpace(result.Reason) != "" {
			entry["reason"] = result.Reason
		}
		results = append(results, entry)
	}

	return successEnvelope(map[string]interface{}{
		"ok":      bulkResult.Errors == 0,
		"action":  bulkResult.Action,
		"results": results,
		"total":   bulkResult.Total,
		"skipped": bulkResult.Skipped,
		"errors":  bulkResult.Errors,
		"added":   bulkResult.Added,
		"content": line,
	}, nil), false
}

func (s *Server) callDirectSet(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, sch, normalized, errOut, isErr := s.directContext(args)
	if isErr {
		return errOut, true
	}

	updates, err := parseKeyValueInput(normalized["fields"])
	if err != nil {
		return errorEnvelope("INVALID_INPUT", "invalid fields payload", err.Error(), nil), true
	}

	typedUpdates, err := parseTypedFieldValues(normalized["fields-json"])
	if err != nil {
		return errorEnvelope("INVALID_INPUT", "invalid --fields-json payload", "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'", nil), true
	}

	stdinMode := boolValue(normalized["stdin"])
	objectIDs := extractSetObjectIDs(normalized, stdinMode)
	explicitFieldsJSON := hasAnyArg(args, "fields-json", "fields_json")
	if stdinMode || len(objectIDs) > 0 {
		if explicitFieldsJSON {
			return errorEnvelope("INVALID_INPUT", "--fields-json is not supported with --stdin", "Use positional field=value updates when using --stdin", nil), true
		}
		if len(updates) == 0 {
			return errorEnvelope("MISSING_ARGUMENT", "no fields to set", "Usage: rvn set --stdin field=value...", nil), true
		}
		return s.callDirectSetBulk(vaultPath, vaultCfg, sch, objectIDs, updates, normalized)
	}

	reference := toString(normalized["object_id"])
	if strings.TrimSpace(reference) == "" {
		return errorEnvelope("MISSING_ARGUMENT", "requires object-id", "Usage: rvn set <object-id> field=value...", nil), true
	}

	if len(updates) == 0 && len(typedUpdates) == 0 {
		return errorEnvelope("MISSING_ARGUMENT", "no fields to set", "Usage: rvn set <object-id> field=value... or --fields-json '{...}'", nil), true
	}

	resolved, err := resolveReference(vaultPath, vaultCfg, sch, reference)
	if err != nil {
		var refErr *directRefError
		if errors.As(err, &refErr) {
			return errorEnvelope(refErr.Code, refErr.Message, refErr.Suggestion, nil), true
		}
		return errorEnvelope("REF_NOT_FOUND", err.Error(), "Check the object reference and run 'rvn reindex' if needed", nil), true
	}

	if resolved.IsSection {
		serviceResult, err := objectsvc.SetEmbeddedObject(objectsvc.SetEmbeddedObjectRequest{
			VaultPath:      vaultPath,
			FilePath:       resolved.FilePath,
			ObjectID:       resolved.ObjectID,
			Updates:        updates,
			TypedUpdates:   typedUpdates,
			Schema:         sch,
			AllowedFields:  map[string]bool{"alias": true, "id": true},
			DocumentParser: parseOptionsFromVaultConfig(vaultCfg),
		})
		if err != nil {
			return mapDirectServiceError(err)
		}

		maybeDirectReindexFile(vaultPath, resolved.FilePath, vaultCfg)

		relPath, _ := filepath.Rel(vaultPath, resolved.FilePath)
		return successEnvelope(map[string]interface{}{
			"file":           relPath,
			"object_id":      resolved.ObjectID,
			"type":           serviceResult.ObjectType,
			"embedded":       true,
			"updated_fields": serviceResult.ResolvedUpdates,
		}, warningMessagesToDirectWarnings(serviceResult.WarningMessages, "UNKNOWN_FIELD")), false
	}

	serviceResult, err := objectsvc.SetObjectFile(objectsvc.SetObjectFileRequest{
		FilePath:      resolved.FilePath,
		ObjectID:      resolved.ObjectID,
		Updates:       updates,
		TypedUpdates:  typedUpdates,
		Schema:        sch,
		AllowedFields: map[string]bool{"alias": true},
	})
	if err != nil {
		return mapDirectServiceError(err)
	}

	maybeDirectReindexFile(vaultPath, resolved.FilePath, vaultCfg)
	relPath, _ := filepath.Rel(vaultPath, resolved.FilePath)

	return successEnvelope(map[string]interface{}{
		"file":           relPath,
		"object_id":      resolved.ObjectID,
		"type":           serviceResult.ObjectType,
		"updated_fields": serviceResult.ResolvedUpdates,
	}, warningMessagesToDirectWarnings(serviceResult.WarningMessages, "UNKNOWN_FIELD")), false
}

func (s *Server) callDirectSetBulk(
	vaultPath string,
	vaultCfg *config.VaultConfig,
	sch *schema.Schema,
	objectIDs []string,
	updates map[string]string,
	normalized map[string]interface{},
) (string, bool) {
	if len(objectIDs) == 0 {
		return errorEnvelope("MISSING_ARGUMENT", "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line", nil), true
	}

	confirm := boolValue(normalized["confirm"])
	parseOpts := parseOptionsFromVaultConfig(vaultCfg)

	if !confirm {
		items := make([]map[string]interface{}, 0, len(objectIDs))
		var skipped []map[string]interface{}

		for _, id := range objectIDs {
			if strings.Contains(id, "#") {
				item, skip := previewSetBulkEmbedded(vaultPath, id, updates, sch, vaultCfg, parseOpts)
				if skip != nil {
					skipped = append(skipped, skip)
					continue
				}
				items = append(items, item)
				continue
			}

			filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
			if err != nil {
				skipped = append(skipped, map[string]interface{}{"id": id, "status": "skipped", "reason": "object not found"})
				continue
			}

			content, err := os.ReadFile(filePath)
			if err != nil {
				skipped = append(skipped, map[string]interface{}{"id": id, "status": "skipped", "reason": fmt.Sprintf("read error: %v", err)})
				continue
			}

			fm, err := parser.ParseFrontmatter(string(content))
			if err != nil || fm == nil {
				skipped = append(skipped, map[string]interface{}{"id": id, "status": "skipped", "reason": "no frontmatter"})
				continue
			}

			objectType := fm.ObjectType
			if objectType == "" {
				objectType = "page"
			}

			if unknownErr := fieldmutation.DetectUnknownFieldMutationByNames(objectType, sch, mapKeys(updates), map[string]bool{"alias": true}); unknownErr != nil {
				skipped = append(skipped, map[string]interface{}{"id": id, "status": "skipped", "reason": unknownErr.Error()})
				continue
			}

			_, resolvedUpdates, _, err := fieldmutation.PrepareValidatedFrontmatterMutation(
				string(content),
				fm,
				objectType,
				updates,
				sch,
				map[string]bool{"alias": true},
			)
			if err != nil {
				var validationErr *fieldmutation.ValidationError
				if errors.As(err, &validationErr) {
					skipped = append(skipped, map[string]interface{}{"id": id, "status": "skipped", "reason": validationErr.Error()})
				} else {
					skipped = append(skipped, map[string]interface{}{"id": id, "status": "skipped", "reason": fmt.Sprintf("validation error: %v", err)})
				}
				continue
			}

			changes := make(map[string]string)
			for field, resolvedVal := range resolvedUpdates {
				oldVal := "<unset>"
				if fm.Fields != nil {
					if v, ok := fm.Fields[field]; ok {
						oldVal = fmt.Sprintf("%v", v)
					}
				}
				changes[field] = fmt.Sprintf("%s (was: %s)", resolvedVal, oldVal)
			}

			items = append(items, map[string]interface{}{
				"id":      id,
				"action":  "set",
				"changes": changes,
			})
		}

		data := map[string]interface{}{
			"preview":  true,
			"action":   "set",
			"items":    items,
			"skipped":  skipped,
			"total":    len(objectIDs),
			"warnings": nil,
			"fields":   updates,
		}
		return successEnvelope(data, nil), false
	}

	results := make([]map[string]interface{}, 0, len(objectIDs))
	modifiedCount := 0
	skippedCount := 0
	errorCount := 0

	for _, id := range objectIDs {
		result := map[string]interface{}{"id": id}

		if strings.Contains(id, "#") {
			err := applySetBulkEmbedded(vaultPath, id, updates, sch, vaultCfg, parseOpts)
			if err != nil {
				result["status"] = "error"
				result["reason"] = err.Error()
				errorCount++
			} else {
				result["status"] = "modified"
				modifiedCount++
			}
			results = append(results, result)
			continue
		}

		filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
		if err != nil {
			result["status"] = "skipped"
			result["reason"] = "object not found"
			skippedCount++
			results = append(results, result)
			continue
		}

		_, err = objectsvc.SetObjectFile(objectsvc.SetObjectFileRequest{
			FilePath:      filePath,
			ObjectID:      id,
			Updates:       updates,
			Schema:        sch,
			AllowedFields: map[string]bool{"alias": true},
		})
		if err != nil {
			result["status"] = "error"
			result["reason"] = setBulkReasonFromError(err)
			errorCount++
			results = append(results, result)
			continue
		}

		maybeDirectReindexFile(vaultPath, filePath, vaultCfg)
		result["status"] = "modified"
		modifiedCount++
		results = append(results, result)
	}

	data := map[string]interface{}{
		"ok":       errorCount == 0,
		"action":   "set",
		"results":  results,
		"total":    len(results),
		"skipped":  skippedCount,
		"errors":   errorCount,
		"modified": modifiedCount,
		"fields":   updates,
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectDelete(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, _, normalized, errOut, isErr := s.directContext(args)
	if isErr {
		return errOut, true
	}

	stdinMode := boolValue(normalized["stdin"])
	objectIDs, embeddedIDs := extractDeleteObjectIDs(normalized, stdinMode)
	if stdinMode || len(objectIDs) > 0 {
		return s.callDirectDeleteBulk(vaultPath, vaultCfg, objectIDs, embeddedIDs, normalized)
	}

	reference := toString(normalized["object_id"])
	if strings.TrimSpace(reference) == "" {
		return errorEnvelope("MISSING_ARGUMENT", "requires object-id argument", "Usage: rvn delete <object-id>", nil), true
	}

	// Ignore "force" in direct mode: MCP/JSON is non-interactive by default.
	_ = normalized["force"]

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return errorEnvelope("SCHEMA_INVALID", "failed to load schema", "Fix schema.yaml and try again", nil), true
	}

	resolved, err := resolveReference(vaultPath, vaultCfg, sch, reference)
	if err != nil {
		var refErr *directRefError
		if errors.As(err, &refErr) {
			return errorEnvelope(refErr.Code, refErr.Message, refErr.Suggestion, nil), true
		}
		return errorEnvelope("REF_NOT_FOUND", err.Error(), "Check the object reference and run 'rvn reindex' if needed", nil), true
	}

	var warnings []directWarning

	db, err := index.Open(vaultPath)
	if err != nil {
		return errorEnvelope("DATABASE_ERROR", "failed to open index database", "Run 'rvn reindex' to rebuild the database", nil), true
	}
	defer db.Close()

	backlinks, err := db.Backlinks(resolved.ObjectID)
	if err == nil && len(backlinks) > 0 {
		backlinkIDs := make([]string, 0, len(backlinks))
		for _, bl := range backlinks {
			backlinkIDs = append(backlinkIDs, bl.SourceID)
		}
		warnings = append(warnings, directWarning{
			Code:    "HAS_BACKLINKS",
			Message: fmt.Sprintf("Object is referenced by %d other objects", len(backlinks)),
			Ref:     strings.Join(backlinkIDs, ", "),
		})
	}

	deletionCfg := vaultCfg.GetDeletionConfig()
	serviceResult, err := objectsvc.DeleteFile(objectsvc.DeleteFileRequest{
		VaultPath: vaultPath,
		FilePath:  resolved.FilePath,
		Behavior:  deletionCfg.Behavior,
		TrashDir:  deletionCfg.TrashDir,
	})
	if err != nil {
		return mapDirectServiceError(err)
	}

	if err := db.RemoveDocument(resolved.FileObjectID); err != nil {
		warningMsg := fmt.Sprintf("Failed to remove deleted object from index: %v", err)
		if errors.Is(err, index.ErrObjectNotFound) {
			warningMsg = "Object not found in index; consider running 'rvn reindex'"
		}
		warnings = append(warnings, directWarning{
			Code:    "INDEX_UPDATE_FAILED",
			Message: warningMsg,
			Ref:     "Run 'rvn reindex' to rebuild the database",
		})
	}

	data := map[string]interface{}{
		"deleted":  resolved.ObjectID,
		"behavior": serviceResult.Behavior,
	}
	if serviceResult.TrashPath != "" {
		relDest, _ := filepath.Rel(vaultPath, serviceResult.TrashPath)
		data["trash_path"] = relDest
	}

	return successEnvelope(data, warnings), false
}

func (s *Server) callDirectDeleteBulk(
	vaultPath string,
	vaultCfg *config.VaultConfig,
	objectIDs []string,
	embeddedIDs []string,
	normalized map[string]interface{},
) (string, bool) {
	if len(objectIDs) == 0 && len(embeddedIDs) == 0 {
		return errorEnvelope("MISSING_ARGUMENT", "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line", nil), true
	}

	var warnings []directWarning
	if len(embeddedIDs) > 0 {
		warnings = append(warnings, directWarning{
			Code:    "embedded_skipped",
			Message: fmt.Sprintf("Skipped %d embedded object(s) - bulk operations only support file-level objects", len(embeddedIDs)),
			Ref:     strings.Join(embeddedIDs, ", "),
		})
	}

	deletionCfg := vaultCfg.GetDeletionConfig()

	db, err := index.Open(vaultPath)
	if err != nil {
		return errorEnvelope("DATABASE_ERROR", "failed to open index database", "Run 'rvn reindex' to rebuild the database", nil), true
	}
	defer db.Close()

	confirm := boolValue(normalized["confirm"])
	if !confirm {
		items := make([]map[string]interface{}, 0, len(objectIDs))
		var skipped []map[string]interface{}

		for _, id := range objectIDs {
			objectID := vaultCfg.FilePathToObjectID(id)
			filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
			if err != nil {
				skipped = append(skipped, map[string]interface{}{
					"id":     id,
					"status": "skipped",
					"reason": "object not found",
				})
				continue
			}

			details := ""
			backlinks, _ := db.Backlinks(objectID)
			if len(backlinks) > 0 {
				details = fmt.Sprintf("⚠ referenced by %d objects", len(backlinks))
			}

			item := map[string]interface{}{
				"id":     id,
				"action": "delete",
				"changes": map[string]string{
					"behavior": "permanent deletion",
				},
			}
			if details != "" {
				item["details"] = details
			}
			if deletionCfg.Behavior == "trash" {
				item["changes"] = map[string]string{"behavior": fmt.Sprintf("move to %s/", deletionCfg.TrashDir)}
			}

			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				skipped = append(skipped, map[string]interface{}{
					"id":     id,
					"status": "skipped",
					"reason": "file not found",
				})
				continue
			}

			items = append(items, item)
		}

		data := map[string]interface{}{
			"preview":  true,
			"action":   "delete",
			"items":    items,
			"skipped":  skipped,
			"total":    len(objectIDs),
			"warnings": warnings,
			"behavior": deletionCfg.Behavior,
		}
		return successEnvelope(data, nil), false
	}

	results := make([]map[string]interface{}, 0, len(objectIDs))
	deletedCount := 0
	skippedCount := 0
	errorCount := 0

	for _, id := range objectIDs {
		result := map[string]interface{}{
			"id": id,
		}

		objectID := vaultCfg.FilePathToObjectID(id)
		filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
		if err != nil {
			result["status"] = "skipped"
			result["reason"] = "object not found"
			skippedCount++
			results = append(results, result)
			continue
		}

		_, err = objectsvc.DeleteFile(objectsvc.DeleteFileRequest{
			VaultPath: vaultPath,
			FilePath:  filePath,
			Behavior:  deletionCfg.Behavior,
			TrashDir:  deletionCfg.TrashDir,
		})
		if err != nil {
			result["status"] = "error"
			var svcErr *objectsvc.Error
			if errors.As(err, &svcErr) {
				result["reason"] = svcErr.Message
			} else {
				result["reason"] = fmt.Sprintf("delete failed: %v", err)
			}
			errorCount++
			results = append(results, result)
			continue
		}

		if err := db.RemoveDocument(objectID); err != nil {
			warningMsg := fmt.Sprintf("Failed to remove deleted object from index: %v", err)
			if errors.Is(err, index.ErrObjectNotFound) {
				warningMsg = "Object not found in index; consider running 'rvn reindex'"
			}
			warnings = append(warnings, directWarning{
				Code:    "INDEX_UPDATE_FAILED",
				Message: warningMsg,
				Ref:     "Run 'rvn reindex' to rebuild the database",
			})
		}

		result["status"] = "deleted"
		deletedCount++
		results = append(results, result)
	}

	data := map[string]interface{}{
		"ok":       errorCount == 0,
		"action":   "delete",
		"results":  results,
		"total":    len(results),
		"skipped":  skippedCount,
		"errors":   errorCount,
		"deleted":  deletedCount,
		"behavior": deletionCfg.Behavior,
	}
	return successEnvelope(data, warnings), false
}

func (s *Server) callDirectMove(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, sch, normalized, errOut, isErr := s.directContext(args)
	if isErr {
		return errOut, true
	}

	source := toString(normalized["source"])
	destination := toString(normalized["destination"])
	stdinMode := boolValue(normalized["stdin"])
	objectIDs, embeddedIDs := extractMoveObjectIDs(normalized)
	if stdinMode || len(objectIDs) > 0 {
		if strings.TrimSpace(destination) == "" {
			// CLI bulk mode accepts one positional destination; MCP clients may send it as source.
			destination = source
		}
		return s.callDirectMoveBulk(vaultPath, vaultCfg, sch, destination, objectIDs, embeddedIDs, normalized)
	}

	if strings.TrimSpace(source) == "" || strings.TrimSpace(destination) == "" {
		return errorEnvelope("MISSING_ARGUMENT", "requires source and destination arguments", "Usage: rvn move <source> <destination>", nil), true
	}

	originalDestination := destination
	destinationIsDirectory := strings.HasSuffix(originalDestination, "/") || strings.HasSuffix(originalDestination, "\\")
	updateRefs := boolValueDefault(normalized["update-refs"], true)
	skipTypeCheck := boolValue(normalized["skip-type-check"])

	sourceResult, err := resolveReference(vaultPath, vaultCfg, sch, source)
	if err != nil {
		var refErr *directRefError
		if errors.As(err, &refErr) {
			return errorEnvelope(refErr.Code, refErr.Message, refErr.Suggestion, nil), true
		}
		return errorEnvelope("REF_NOT_FOUND", err.Error(), "Check the reference and run 'rvn reindex' if needed", nil), true
	}
	sourceFile := sourceResult.FilePath

	if err := paths.ValidateWithinVault(vaultPath, sourceFile); err != nil {
		return errorEnvelope("VALIDATION_FAILED", "source path is outside vault", "Files can only be moved within the vault", nil), true
	}

	sourceRelPath, err := filepath.Rel(vaultPath, sourceFile)
	if err != nil {
		return errorEnvelope("INTERNAL_ERROR", "failed to resolve source path", "", nil), true
	}
	sourceID := vaultCfg.FilePathToObjectID(sourceRelPath)

	if destinationIsDirectory {
		sourceBase := strings.TrimSuffix(filepath.Base(sourceRelPath), ".md")
		if strings.TrimSpace(sourceBase) == "" {
			return errorEnvelope("INVALID_INPUT", "source has an invalid filename", "Use an explicit destination file path", nil), true
		}
		destination = filepath.ToSlash(filepath.Join(originalDestination, sourceBase))
	}

	destination = paths.EnsureMDExtension(destination)
	destinationBase := strings.TrimSuffix(filepath.Base(destination), ".md")
	if strings.TrimSpace(destinationBase) == "" {
		return errorEnvelope("INVALID_INPUT", "destination has an empty filename", "Use a non-empty destination filename or a directory ending with /", nil), true
	}

	destPath := destination
	if vaultCfg.HasDirectoriesConfig() {
		destPath = vaultCfg.ResolveReferenceToFilePath(strings.TrimSuffix(destination, ".md"))
	}
	destFile := filepath.Join(vaultPath, destPath)

	if err := paths.ValidateWithinVault(vaultPath, destFile); err != nil {
		return errorEnvelope("VALIDATION_FAILED", "destination path is outside vault", "Files can only be moved within the vault", nil), true
	}

	relDest, _ := filepath.Rel(vaultPath, destFile)
	if strings.HasPrefix(relDest, ".raven") || strings.HasPrefix(relDest, ".trash") {
		return errorEnvelope("VALIDATION_FAILED", "cannot move to system directory", "Destination cannot be in .raven or .trash directories", nil), true
	}

	if _, err := os.Stat(destFile); err == nil {
		return errorEnvelope("VALIDATION_FAILED", fmt.Sprintf("Destination '%s' already exists", destination), "Choose a different destination or delete the existing file first", nil), true
	}

	parseOpts := parseOptionsFromVaultConfig(vaultCfg)
	content, err := os.ReadFile(sourceFile)
	if err != nil {
		return errorEnvelope("FILE_READ_ERROR", err.Error(), "", nil), true
	}
	doc, err := parser.ParseDocumentWithOptions(string(content), sourceFile, vaultPath, parseOpts)
	if err != nil {
		return errorEnvelope("INTERNAL_ERROR", fmt.Sprintf("failed to parse source file: %v", err), "", nil), true
	}

	fileType := ""
	if len(doc.Objects) > 0 {
		fileType = doc.Objects[0].ObjectType
	}

	var warnings []directWarning
	destDir := filepath.Dir(relDest)
	mismatchType := ""
	for typeName, typeDef := range sch.Types {
		if typeDef.DefaultPath == "" {
			continue
		}
		defaultPath := strings.TrimSuffix(typeDef.DefaultPath, "/")
		if destDir == defaultPath && typeName != fileType {
			mismatchType = typeName
			break
		}
	}

	if mismatchType != "" && !skipTypeCheck {
		warnings = append(warnings, directWarning{
			Code: "TYPE_DIRECTORY_MISMATCH",
			Message: fmt.Sprintf("Moving to '%s/' which is the default directory for type '%s', but file has type '%s'",
				destDir, mismatchType, fileType),
			Ref: fmt.Sprintf("Use --skip-type-check to proceed, or change the file's type to '%s'", mismatchType),
		})
		return successEnvelope(map[string]interface{}{
			"source":        sourceID,
			"destination":   vaultCfg.FilePathToObjectID(destPath),
			"needs_confirm": true,
			"reason":        fmt.Sprintf("Type mismatch: file is '%s' but destination is default path for '%s'", fileType, mismatchType),
		}, warnings), false
	}

	serviceResult, err := objectsvc.MoveFile(objectsvc.MoveFileRequest{
		VaultPath:         vaultPath,
		SourceFile:        sourceFile,
		DestinationFile:   destFile,
		SourceObjectID:    sourceID,
		DestinationObject: vaultCfg.FilePathToObjectID(destPath),
		UpdateRefs:        updateRefs,
		FailOnIndexError:  false,
		VaultConfig:       vaultCfg,
		Schema:            sch,
		ParseOptions:      parseOpts,
	})
	if err != nil {
		return mapDirectServiceError(err)
	}

	for _, warningMessage := range serviceResult.WarningMessages {
		warnings = append(warnings, directWarning{
			Code:    "INDEX_UPDATE_FAILED",
			Message: warningMessage,
			Ref:     "Run 'rvn reindex' to rebuild the database",
		})
	}

	data := map[string]interface{}{
		"source":      sourceID,
		"destination": vaultCfg.FilePathToObjectID(destPath),
	}
	if len(serviceResult.UpdatedRefs) > 0 {
		data["updated_refs"] = serviceResult.UpdatedRefs
	}

	return successEnvelope(data, warnings), false
}

func (s *Server) callDirectMoveBulk(
	vaultPath string,
	vaultCfg *config.VaultConfig,
	sch *schema.Schema,
	destination string,
	objectIDs []string,
	embeddedIDs []string,
	normalized map[string]interface{},
) (string, bool) {
	if strings.TrimSpace(destination) == "" {
		return errorEnvelope("MISSING_ARGUMENT", "no destination provided", "Usage: rvn move --stdin <destination-directory/>", nil), true
	}
	if !strings.HasSuffix(destination, "/") {
		return errorEnvelope("INVALID_INPUT", "destination must be a directory (end with /)", "Example: rvn move --stdin archive/projects/", nil), true
	}
	if len(objectIDs) == 0 {
		return errorEnvelope("MISSING_ARGUMENT", "no object IDs provided via stdin", "Provide object IDs via object_ids (array/string) when using raven_move bulk mode", nil), true
	}

	var warnings []directWarning
	if len(embeddedIDs) > 0 {
		warnings = append(warnings, directWarning{
			Code:    "embedded_skipped",
			Message: fmt.Sprintf("Skipped %d embedded object(s) - bulk operations only support file-level objects", len(embeddedIDs)),
			Ref:     strings.Join(embeddedIDs, ", "),
		})
	}

	confirm := boolValue(normalized["confirm"])
	updateRefs := boolValueDefault(normalized["update-refs"], true)
	parseOpts := parseOptionsFromVaultConfig(vaultCfg)

	if !confirm {
		items := make([]map[string]interface{}, 0, len(objectIDs))
		var skipped []map[string]interface{}

		for _, id := range objectIDs {
			sourceFile, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
			if err != nil {
				skipped = append(skipped, map[string]interface{}{
					"id":     id,
					"status": "skipped",
					"reason": "object not found",
				})
				continue
			}

			filename := filepath.Base(sourceFile)
			destPath := filepath.Join(destination, filename)
			fullDestPath := filepath.Join(vaultPath, destPath)
			if _, err := os.Stat(fullDestPath); err == nil {
				skipped = append(skipped, map[string]interface{}{
					"id":     id,
					"status": "skipped",
					"reason": fmt.Sprintf("destination already exists: %s", destPath),
				})
				continue
			}

			items = append(items, map[string]interface{}{
				"id":      id,
				"action":  "move",
				"details": fmt.Sprintf("→ %s", destPath),
			})
		}

		data := map[string]interface{}{
			"preview":     true,
			"action":      "move",
			"items":       items,
			"skipped":     skipped,
			"total":       len(objectIDs),
			"warnings":    warnings,
			"destination": destination,
		}
		return successEnvelope(data, nil), false
	}

	results := make([]map[string]interface{}, 0, len(objectIDs))
	movedCount := 0
	skippedCount := 0
	errorCount := 0

	for _, id := range objectIDs {
		result := map[string]interface{}{
			"id": id,
		}

		sourceFile, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
		if err != nil {
			result["status"] = "skipped"
			result["reason"] = "object not found"
			skippedCount++
			results = append(results, result)
			continue
		}

		filename := filepath.Base(sourceFile)
		destPath := filepath.Join(destination, filename)
		fullDestPath := filepath.Join(vaultPath, destPath)

		if _, err := os.Stat(fullDestPath); err == nil {
			result["status"] = "skipped"
			result["reason"] = fmt.Sprintf("destination already exists: %s", destPath)
			skippedCount++
			results = append(results, result)
			continue
		}

		relSource, _ := filepath.Rel(vaultPath, sourceFile)
		sourceID := vaultCfg.FilePathToObjectID(relSource)
		destID := vaultCfg.FilePathToObjectID(destPath)

		serviceResult, err := objectsvc.MoveFile(objectsvc.MoveFileRequest{
			VaultPath:         vaultPath,
			SourceFile:        sourceFile,
			DestinationFile:   fullDestPath,
			SourceObjectID:    sourceID,
			DestinationObject: destID,
			UpdateRefs:        updateRefs,
			FailOnIndexError:  true,
			VaultConfig:       vaultCfg,
			Schema:            sch,
			ParseOptions:      parseOpts,
		})
		if err != nil {
			result["status"] = "error"
			var svcErr *objectsvc.Error
			if errors.As(err, &svcErr) {
				result["reason"] = svcErr.Message
			} else {
				result["reason"] = fmt.Sprintf("move failed: %v", err)
			}
			errorCount++
			results = append(results, result)
			continue
		}

		for _, warningMessage := range serviceResult.WarningMessages {
			warnings = append(warnings, directWarning{
				Code:    "INDEX_UPDATE_FAILED",
				Message: warningMessage,
				Ref:     "Run 'rvn reindex' to rebuild the database",
			})
		}

		result["status"] = "moved"
		result["details"] = destPath
		movedCount++
		results = append(results, result)
	}

	data := map[string]interface{}{
		"ok":          errorCount == 0,
		"action":      "move",
		"results":     results,
		"total":       len(results),
		"skipped":     skippedCount,
		"errors":      errorCount,
		"moved":       movedCount,
		"destination": destination,
	}
	return successEnvelope(data, warnings), false
}

func (s *Server) directContext(args map[string]interface{}) (string, *config.VaultConfig, *schema.Schema, map[string]interface{}, string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return "", nil, nil, nil, errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return "", nil, nil, nil, errorEnvelope("CONFIG_INVALID", "failed to load vault config", "Fix raven.yaml and try again", nil), true
	}
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return "", nil, nil, nil, errorEnvelope("SCHEMA_INVALID", "failed to load schema", "Fix schema.yaml and try again", nil), true
	}

	normalized := normalizeArgs(args)
	return vaultPath, vaultCfg, sch, normalized, "", false
}

func parseKeyValueInput(raw interface{}) (map[string]string, error) {
	if raw == nil {
		return map[string]string{}, nil
	}
	out := make(map[string]string)
	for _, pair := range keyValuePairs(raw) {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid key=value pair: %s", pair)
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}

func parseTypedFieldValues(raw interface{}) (map[string]schema.FieldValue, error) {
	if raw == nil {
		return map[string]schema.FieldValue{}, nil
	}

	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return map[string]schema.FieldValue{}, nil
		}
		return fieldmutation.ParseFieldValuesJSON(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return fieldmutation.ParseFieldValuesJSON(string(b))
	}
}

func resolveReference(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, reference string) (*resolvedReference, error) {
	candidates := []string{reference}
	if !strings.HasSuffix(reference, ".md") {
		candidates = append(candidates, reference+".md")
	}

	for _, candidate := range candidates {
		fullPath := filepath.Join(vaultPath, candidate)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			objectID := strings.TrimSuffix(candidate, ".md")
			objectID = vaultCfg.FilePathToObjectID(objectID)
			return &resolvedReference{
				ObjectID:     objectID,
				FileObjectID: objectID,
				FilePath:     fullPath,
				IsSection:    false,
			}, nil
		}
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		return nil, &directRefError{
			Code:       "DATABASE_ERROR",
			Message:    fmt.Sprintf("failed to open database: %v", err),
			Suggestion: "Run 'rvn reindex' to rebuild the database",
		}
	}
	defer db.Close()

	res, err := db.Resolver(index.ResolverOptions{
		DailyDirectory: vaultCfg.GetDailyDirectory(),
		Schema:         sch,
	})
	if err != nil {
		return nil, &directRefError{
			Code:       "DATABASE_ERROR",
			Message:    fmt.Sprintf("failed to create resolver: %v", err),
			Suggestion: "Run 'rvn reindex' to rebuild the database",
		}
	}

	resolved := res.Resolve(reference)
	if resolved.Ambiguous {
		return nil, &directRefError{
			Code:       "REF_AMBIGUOUS",
			Message:    fmt.Sprintf("reference '%s' is ambiguous", reference),
			Suggestion: "Use a full object ID/path to disambiguate",
		}
	}
	if resolved.TargetID == "" {
		return nil, &directRefError{
			Code:       "REF_NOT_FOUND",
			Message:    fmt.Sprintf("reference '%s' not found", reference),
			Suggestion: "Check the object reference and run 'rvn reindex' if needed",
		}
	}

	fileObjectID := resolved.TargetID
	isSection := false
	if idx := strings.Index(fileObjectID, "#"); idx >= 0 {
		isSection = true
		fileObjectID = fileObjectID[:idx]
	}

	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileObjectID, vaultCfg)
	if err != nil {
		return nil, &directRefError{
			Code:       "REF_NOT_FOUND",
			Message:    fmt.Sprintf("resolved to '%s' but file not found", resolved.TargetID),
			Suggestion: "Run 'rvn reindex' if the index is stale",
		}
	}

	return &resolvedReference{
		ObjectID:     resolved.TargetID,
		FileObjectID: fileObjectID,
		FilePath:     filePath,
		IsSection:    isSection,
	}, nil
}

func maybeDirectReindexFile(vaultPath, filePath string, vaultCfg *config.VaultConfig) {
	if vaultCfg == nil || !vaultCfg.IsAutoReindexEnabled() {
		return
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return
	}

	doc, err := parser.ParseDocumentWithOptions(string(content), filePath, vaultPath, parseOptionsFromVaultConfig(vaultCfg))
	if err != nil {
		return
	}

	var mtime int64
	if st, err := os.Stat(filePath); err == nil {
		mtime = st.ModTime().Unix()
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		return
	}
	defer db.Close()
	db.SetDailyDirectory(vaultCfg.GetDailyDirectory())
	_ = db.IndexDocumentWithMtime(doc, sch, mtime)
}

func parseOptionsFromVaultConfig(vaultCfg *config.VaultConfig) *parser.ParseOptions {
	if vaultCfg == nil {
		return nil
	}
	return &parser.ParseOptions{
		ObjectsRoot: vaultCfg.GetObjectsRoot(),
		PagesRoot:   vaultCfg.GetPagesRoot(),
	}
}

func successEnvelope(data map[string]interface{}, warnings []directWarning) string {
	payload := map[string]interface{}{
		"ok":   true,
		"data": data,
	}
	if len(warnings) > 0 {
		payload["warnings"] = warnings
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func errorEnvelope(code, message, suggestion string, details map[string]interface{}) string {
	errPayload := map[string]interface{}{
		"code":    code,
		"message": message,
	}
	if suggestion != "" {
		errPayload["suggestion"] = suggestion
	}
	if len(details) > 0 {
		errPayload["details"] = details
	}

	payload := map[string]interface{}{
		"ok":    false,
		"error": errPayload,
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func mapDirectServiceError(err error) (string, bool) {
	var svcErr *objectsvc.Error
	if errors.As(err, &svcErr) {
		return errorEnvelope(mapServiceCodeToCLI(svcErr.Code), svcErr.Message, svcErr.Suggestion, svcErr.Details), true
	}

	var unknownErr *fieldmutation.UnknownFieldMutationError
	if errors.As(err, &unknownErr) {
		details := unknownErr.Details()
		return errorEnvelope("UNKNOWN_FIELD", unknownErr.Error(), unknownErr.Suggestion(), details), true
	}

	var validationErr *fieldmutation.ValidationError
	if errors.As(err, &validationErr) {
		return errorEnvelope("VALIDATION_FAILED", validationErr.Error(), validationErr.Suggestion(), nil), true
	}

	return errorEnvelope("UNEXPECTED", err.Error(), "", nil), true
}

func warningMessagesToDirectWarnings(messages []string, code string) []directWarning {
	if len(messages) == 0 {
		return nil
	}
	warnings := make([]directWarning, 0, len(messages))
	for _, message := range messages {
		warnings = append(warnings, directWarning{
			Code:    code,
			Message: message,
		})
	}
	return warnings
}

func mapServiceCodeToCLI(code objectsvc.ErrorCode) string {
	switch code {
	case objectsvc.ErrorTypeNotFound:
		return "TYPE_NOT_FOUND"
	case objectsvc.ErrorRequiredField:
		return "REQUIRED_FIELD_MISSING"
	case objectsvc.ErrorInvalidInput:
		return "INVALID_INPUT"
	case objectsvc.ErrorFileExists:
		return "FILE_EXISTS"
	case objectsvc.ErrorValidationFailed:
		return "VALIDATION_FAILED"
	case objectsvc.ErrorFileRead:
		return "FILE_READ_ERROR"
	case objectsvc.ErrorFileWrite:
		return "FILE_WRITE_ERROR"
	default:
		return "INTERNAL_ERROR"
	}
}

func boolValue(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		switch strings.ToLower(strings.TrimSpace(val)) {
		case "1", "true", "yes", "y", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func boolValueDefault(v interface{}, defaultValue bool) bool {
	if v == nil {
		return defaultValue
	}
	return boolValue(v)
}

func extractMoveObjectIDs(args map[string]interface{}) ([]string, []string) {
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

	appendIDs(args["object-ids"])
	appendIDs(args["object_ids"])
	appendIDs(args["object-id"])
	appendIDs(args["object_id"])
	appendIDs(args["ids"])

	// Allow `source` to be a list in MCP calls for explicit bulk payloads.
	switch args["source"].(type) {
	case []interface{}, []string:
		if boolValue(args["stdin"]) {
			appendIDs(args["source"])
		}
	case string:
		if boolValue(args["stdin"]) && strings.TrimSpace(toString(args["destination"])) != "" {
			appendIDs(args["source"])
		}
	}

	seen := make(map[string]struct{}, len(collected))
	ids := make([]string, 0, len(collected))
	embedded := make([]string, 0)
	for _, id := range collected {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if strings.Contains(id, "#") {
			embedded = append(embedded, id)
			continue
		}
		ids = append(ids, id)
	}

	return ids, embedded
}

func extractDeleteObjectIDs(args map[string]interface{}, stdinMode bool) ([]string, []string) {
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

	appendIDs(args["object-ids"])
	appendIDs(args["object_ids"])
	appendIDs(args["ids"])
	if stdinMode {
		appendIDs(args["object-id"])
		appendIDs(args["object_id"])
	}

	seen := make(map[string]struct{}, len(collected))
	ids := make([]string, 0, len(collected))
	embedded := make([]string, 0)
	for _, id := range collected {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if strings.Contains(id, "#") {
			embedded = append(embedded, id)
			continue
		}
		ids = append(ids, id)
	}

	return ids, embedded
}

func extractSetObjectIDs(args map[string]interface{}, stdinMode bool) []string {
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

	appendIDs(args["object-ids"])
	appendIDs(args["object_ids"])
	appendIDs(args["ids"])
	if stdinMode {
		appendIDs(args["object-id"])
		appendIDs(args["object_id"])
	}

	seen := make(map[string]struct{}, len(collected))
	ids := make([]string, 0, len(collected))
	for _, id := range collected {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	return ids
}

func mapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func hasAnyArg(args map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		if _, ok := args[key]; ok {
			return true
		}
	}
	return false
}

func previewSetBulkEmbedded(
	vaultPath string,
	id string,
	updates map[string]string,
	sch *schema.Schema,
	vaultCfg *config.VaultConfig,
	parseOpts *parser.ParseOptions,
) (map[string]interface{}, map[string]interface{}) {
	fileID, _, isEmbedded := paths.ParseEmbeddedID(id)
	if !isEmbedded {
		return nil, map[string]interface{}{"id": id, "status": "skipped", "reason": "invalid embedded ID format"}
	}

	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileID, vaultCfg)
	if err != nil {
		return nil, map[string]interface{}{"id": id, "status": "skipped", "reason": "parent file not found"}
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, map[string]interface{}{"id": id, "status": "skipped", "reason": fmt.Sprintf("read error: %v", err)}
	}

	doc, err := parser.ParseDocumentWithOptions(string(content), filePath, vaultPath, parseOpts)
	if err != nil {
		return nil, map[string]interface{}{"id": id, "status": "skipped", "reason": fmt.Sprintf("parse error: %v", err)}
	}

	var targetObj *parser.ParsedObject
	for _, obj := range doc.Objects {
		if obj.ID == id {
			targetObj = obj
			break
		}
	}
	if targetObj == nil {
		return nil, map[string]interface{}{"id": id, "status": "skipped", "reason": "embedded object not found"}
	}

	if unknownErr := fieldmutation.DetectUnknownFieldMutationByNames(targetObj.ObjectType, sch, mapKeys(updates), map[string]bool{"alias": true, "id": true}); unknownErr != nil {
		return nil, map[string]interface{}{"id": id, "status": "skipped", "reason": unknownErr.Error()}
	}

	_, resolvedUpdates, _, err := fieldmutation.PrepareValidatedFieldMutation(
		targetObj.ObjectType,
		targetObj.Fields,
		updates,
		sch,
		map[string]bool{"alias": true, "id": true},
	)
	if err != nil {
		var validationErr *fieldmutation.ValidationError
		if errors.As(err, &validationErr) {
			return nil, map[string]interface{}{"id": id, "status": "skipped", "reason": validationErr.Error()}
		}
		return nil, map[string]interface{}{"id": id, "status": "skipped", "reason": fmt.Sprintf("validation error: %v", err)}
	}

	changes := make(map[string]string)
	for field, resolvedVal := range resolvedUpdates {
		oldVal := "<unset>"
		if targetObj.Fields != nil {
			if v, ok := targetObj.Fields[field]; ok {
				if s, ok := v.AsString(); ok {
					oldVal = s
				} else if n, ok := v.AsNumber(); ok {
					oldVal = fmt.Sprintf("%v", n)
				} else if b, ok := v.AsBool(); ok {
					oldVal = fmt.Sprintf("%v", b)
				} else {
					oldVal = fmt.Sprintf("%v", v.Raw())
				}
			}
		}
		changes[field] = fmt.Sprintf("%s (was: %s)", resolvedVal, oldVal)
	}

	return map[string]interface{}{
		"id":      id,
		"action":  "set",
		"changes": changes,
	}, nil
}

func applySetBulkEmbedded(
	vaultPath string,
	id string,
	updates map[string]string,
	sch *schema.Schema,
	vaultCfg *config.VaultConfig,
	parseOpts *parser.ParseOptions,
) error {
	fileID, _, isEmbedded := paths.ParseEmbeddedID(id)
	if !isEmbedded {
		return fmt.Errorf("invalid embedded ID format")
	}

	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileID, vaultCfg)
	if err != nil {
		return fmt.Errorf("parent file not found: %w", err)
	}

	_, err = objectsvc.SetEmbeddedObject(objectsvc.SetEmbeddedObjectRequest{
		VaultPath:      vaultPath,
		FilePath:       filePath,
		ObjectID:       id,
		Updates:        updates,
		Schema:         sch,
		AllowedFields:  map[string]bool{"alias": true, "id": true},
		DocumentParser: parseOpts,
	})
	if err != nil {
		var svcErr *objectsvc.Error
		if errors.As(err, &svcErr) {
			return errors.New(svcErr.Message)
		}
		return err
	}

	maybeDirectReindexFile(vaultPath, filePath, vaultCfg)
	return nil
}

func setBulkReasonFromError(err error) string {
	var svcErr *objectsvc.Error
	var unknownErr *fieldmutation.UnknownFieldMutationError
	var validationErr *fieldmutation.ValidationError

	switch {
	case errors.As(err, &svcErr):
		return svcErr.Message
	case errors.As(err, &unknownErr):
		return unknownErr.Error()
	case errors.As(err, &validationErr):
		return validationErr.Error()
	default:
		return fmt.Sprintf("update error: %v", err)
	}
}

func extractAddObjectIDs(args map[string]interface{}, stdinMode bool) []string {
	return extractSetObjectIDs(args, stdinMode)
}

func resolveAddDestination(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, toRef string) (*resolvedReference, bool, error) {
	resolved, err := resolveReference(vaultPath, vaultCfg, sch, toRef)
	if err == nil {
		return resolved, isDailyNoteObjectID(resolved.FileObjectID, vaultCfg), nil
	}

	var refErr *directRefError
	if !errors.As(err, &refErr) || refErr.Code != "REF_NOT_FOUND" {
		return nil, false, err
	}

	dailyDir := "daily"
	if vaultCfg != nil && vaultCfg.GetDailyDirectory() != "" {
		dailyDir = vaultCfg.GetDailyDirectory()
	}

	dateStr := ""
	if resolvedDate, ok := resolveRelativeDateKeyword(toRef); ok {
		dateStr = resolvedDate
	} else if dates.IsValidDate(strings.TrimSpace(toRef)) {
		dateStr = strings.TrimSpace(toRef)
	} else if strings.HasPrefix(strings.TrimSpace(toRef), dailyDir+"/") {
		candidate := strings.TrimPrefix(strings.TrimSpace(toRef), dailyDir+"/")
		if dates.IsValidDate(candidate) {
			dateStr = candidate
		}
	}
	if dateStr == "" {
		return nil, false, err
	}

	fileID := vaultCfg.DailyNoteID(dateStr)
	return &resolvedReference{
		ObjectID:     fileID,
		FileObjectID: fileID,
		FilePath:     vaultCfg.DailyNotePath(vaultPath, dateStr),
		IsSection:    false,
	}, true, nil
}

func resolveRelativeDateKeyword(value string) (string, bool) {
	resolved, ok := dates.ResolveRelativeDateKeyword(value, time.Now(), time.Monday)
	if !ok || resolved.Kind != dates.RelativeDateInstant {
		return "", false
	}
	return resolved.Date.Format(dates.DateLayout), true
}

func isDailyNoteObjectID(objectID string, vaultCfg *config.VaultConfig) bool {
	if objectID == "" {
		return false
	}

	baseID := objectID
	if parts := strings.SplitN(objectID, "#", 2); len(parts) == 2 {
		baseID = parts[0]
	}

	dailyDir := "daily"
	if vaultCfg != nil && vaultCfg.GetDailyDirectory() != "" {
		dailyDir = vaultCfg.GetDailyDirectory()
	}
	if !strings.HasPrefix(baseID, dailyDir+"/") {
		return false
	}

	dateStr := strings.TrimPrefix(baseID, dailyDir+"/")
	return dates.IsValidDate(dateStr)
}

func extractIDFromPipeLine(line string) string {
	s := strings.TrimSpace(line)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "\t") {
		parts := strings.SplitN(s, "\t", 3)
		if len(parts) >= 2 {
			id := strings.TrimSpace(parts[1])
			if id != "" {
				return id
			}
		}
	}
	return s
}
