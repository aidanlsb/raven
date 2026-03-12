package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemasvc"
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
	case "raven_search":
		out, isErr := s.callDirectSearch(args)
		return out, isErr, true
	case "raven_backlinks":
		out, isErr := s.callDirectBacklinks(args)
		return out, isErr, true
	case "raven_outlinks":
		out, isErr := s.callDirectOutlinks(args)
		return out, isErr, true
	case "raven_resolve":
		out, isErr := s.callDirectResolve(args)
		return out, isErr, true
	case "raven_query":
		out, isErr := s.callDirectQuery(args)
		return out, isErr, true
	case "raven_schema_add_type":
		out, isErr := s.callDirectSchemaAddType(args)
		return out, isErr, true
	case "raven_schema_add_trait":
		out, isErr := s.callDirectSchemaAddTrait(args)
		return out, isErr, true
	case "raven_schema_add_field":
		out, isErr := s.callDirectSchemaAddField(args)
		return out, isErr, true
	case "raven_schema_validate":
		out, isErr := s.callDirectSchemaValidate(args)
		return out, isErr, true
	case "raven_schema_update_type":
		out, isErr := s.callDirectSchemaUpdateType(args)
		return out, isErr, true
	case "raven_schema_update_trait":
		out, isErr := s.callDirectSchemaUpdateTrait(args)
		return out, isErr, true
	case "raven_schema_update_field":
		out, isErr := s.callDirectSchemaUpdateField(args)
		return out, isErr, true
	case "raven_schema_remove_type":
		out, isErr := s.callDirectSchemaRemoveType(args)
		return out, isErr, true
	case "raven_schema_remove_trait":
		out, isErr := s.callDirectSchemaRemoveTrait(args)
		return out, isErr, true
	case "raven_schema_remove_field":
		out, isErr := s.callDirectSchemaRemoveField(args)
		return out, isErr, true
	case "raven_schema_rename_field":
		out, isErr := s.callDirectSchemaRenameField(args)
		return out, isErr, true
	case "raven_schema_rename_type":
		out, isErr := s.callDirectSchemaRenameType(args)
		return out, isErr, true
	case "raven_schema_template_list":
		out, isErr := s.callDirectSchemaTemplateList(args)
		return out, isErr, true
	case "raven_schema_template_get":
		out, isErr := s.callDirectSchemaTemplateGet(args)
		return out, isErr, true
	case "raven_schema_template_set":
		out, isErr := s.callDirectSchemaTemplateSet(args)
		return out, isErr, true
	case "raven_schema_template_remove":
		out, isErr := s.callDirectSchemaTemplateRemove(args)
		return out, isErr, true
	case "raven_schema_type_template_list":
		out, isErr := s.callDirectSchemaTypeTemplateList(args)
		return out, isErr, true
	case "raven_schema_type_template_set":
		out, isErr := s.callDirectSchemaTypeTemplateSet(args)
		return out, isErr, true
	case "raven_schema_type_template_remove":
		out, isErr := s.callDirectSchemaTypeTemplateRemove(args)
		return out, isErr, true
	case "raven_schema_type_template_default":
		out, isErr := s.callDirectSchemaTypeTemplateDefault(args)
		return out, isErr, true
	case "raven_schema_core_template_list":
		out, isErr := s.callDirectSchemaCoreTemplateList(args)
		return out, isErr, true
	case "raven_schema_core_template_set":
		out, isErr := s.callDirectSchemaCoreTemplateSet(args)
		return out, isErr, true
	case "raven_schema_core_template_remove":
		out, isErr := s.callDirectSchemaCoreTemplateRemove(args)
		return out, isErr, true
	case "raven_schema_core_template_default":
		out, isErr := s.callDirectSchemaCoreTemplateDefault(args)
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
	request := objectsvc.SetBulkRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		Schema:       sch,
		ObjectIDs:    objectIDs,
		Updates:      updates,
		ParseOptions: parseOpts,
	}

	if !confirm {
		previewResult, err := objectsvc.PreviewSetBulk(request)
		if err != nil {
			return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
		}
		items := make([]map[string]interface{}, 0, len(previewResult.Items))
		skipped := make([]map[string]interface{}, 0, len(previewResult.Skipped))
		for _, item := range previewResult.Items {
			items = append(items, map[string]interface{}{
				"id":      item.ID,
				"action":  item.Action,
				"changes": item.Changes,
			})
		}
		for _, result := range previewResult.Skipped {
			skipped = append(skipped, map[string]interface{}{
				"id":     result.ID,
				"status": result.Status,
				"reason": result.Reason,
			})
		}

		data := map[string]interface{}{
			"preview":  true,
			"action":   "set",
			"items":    items,
			"skipped":  skipped,
			"total":    previewResult.Total,
			"warnings": nil,
			"fields":   updates,
		}
		return successEnvelope(data, nil), false
	}

	summaryResult, err := objectsvc.ApplySetBulk(request, func(filePath string) {
		maybeDirectReindexFile(vaultPath, filePath, vaultCfg)
	})
	if err != nil {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}
	results := make([]map[string]interface{}, 0, len(summaryResult.Results))
	for _, result := range summaryResult.Results {
		entry := map[string]interface{}{
			"id":     result.ID,
			"status": result.Status,
		}
		if strings.TrimSpace(result.Reason) != "" {
			entry["reason"] = result.Reason
		}
		results = append(results, entry)
	}

	data := map[string]interface{}{
		"ok":       summaryResult.Errors == 0,
		"action":   summaryResult.Action,
		"results":  results,
		"total":    summaryResult.Total,
		"skipped":  summaryResult.Skipped,
		"errors":   summaryResult.Errors,
		"modified": summaryResult.Modified,
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
	request := objectsvc.DeleteBulkRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		ObjectIDs:   objectIDs,
		Behavior:    deletionCfg.Behavior,
		TrashDir:    deletionCfg.TrashDir,
	}

	confirm := boolValue(normalized["confirm"])
	if !confirm {
		previewResult, err := objectsvc.PreviewDeleteBulk(request)
		if err != nil {
			return errorEnvelope("DATABASE_ERROR", "failed to open index database", "Run 'rvn reindex' to rebuild the database", nil), true
		}
		items := make([]map[string]interface{}, 0, len(previewResult.Items))
		skipped := make([]map[string]interface{}, 0, len(previewResult.Skipped))
		for _, item := range previewResult.Items {
			entry := map[string]interface{}{
				"id":      item.ID,
				"action":  item.Action,
				"changes": item.Changes,
			}
			if strings.TrimSpace(item.Details) != "" {
				entry["details"] = item.Details
			}
			items = append(items, entry)
		}
		for _, result := range previewResult.Skipped {
			skipped = append(skipped, map[string]interface{}{
				"id":     result.ID,
				"status": result.Status,
				"reason": result.Reason,
			})
		}

		data := map[string]interface{}{
			"preview":  true,
			"action":   "delete",
			"items":    items,
			"skipped":  skipped,
			"total":    previewResult.Total,
			"warnings": warnings,
			"behavior": previewResult.Behavior,
		}
		return successEnvelope(data, nil), false
	}

	summaryResult, err := objectsvc.ApplyDeleteBulk(request)
	if err != nil {
		return errorEnvelope("DATABASE_ERROR", "failed to open index database", "Run 'rvn reindex' to rebuild the database", nil), true
	}
	results := make([]map[string]interface{}, 0, len(summaryResult.Results))
	for _, result := range summaryResult.Results {
		entry := map[string]interface{}{
			"id":     result.ID,
			"status": result.Status,
		}
		if strings.TrimSpace(result.Reason) != "" {
			entry["reason"] = result.Reason
		}
		results = append(results, entry)
	}
	for _, warningMsg := range summaryResult.WarningMessages {
		warnings = append(warnings, directWarning{
			Code:    "INDEX_UPDATE_FAILED",
			Message: warningMsg,
			Ref:     "Run 'rvn reindex' to rebuild the database",
		})
	}

	data := map[string]interface{}{
		"ok":       summaryResult.Errors == 0,
		"action":   summaryResult.Action,
		"results":  results,
		"total":    summaryResult.Total,
		"skipped":  summaryResult.Skipped,
		"errors":   summaryResult.Errors,
		"deleted":  summaryResult.Deleted,
		"behavior": summaryResult.Behavior,
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
	request := objectsvc.MoveBulkRequest{
		VaultPath:      vaultPath,
		VaultConfig:    vaultCfg,
		Schema:         sch,
		ObjectIDs:      objectIDs,
		DestinationDir: destination,
		UpdateRefs:     updateRefs,
		ParseOptions:   parseOptionsFromVaultConfig(vaultCfg),
	}

	if !confirm {
		previewResult, err := objectsvc.PreviewMoveBulk(request)
		if err != nil {
			return errorEnvelope("INVALID_INPUT", err.Error(), "Example: rvn move --stdin archive/projects/", nil), true
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

		data := map[string]interface{}{
			"preview":     true,
			"action":      previewResult.Action,
			"items":       items,
			"skipped":     skipped,
			"total":       previewResult.Total,
			"warnings":    warnings,
			"destination": previewResult.Destination,
		}
		return successEnvelope(data, nil), false
	}

	summaryResult, err := objectsvc.ApplyMoveBulk(request)
	if err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), "Example: rvn move --stdin archive/projects/", nil), true
	}
	results := make([]map[string]interface{}, 0, len(summaryResult.Results))
	for _, result := range summaryResult.Results {
		entry := map[string]interface{}{
			"id":     result.ID,
			"status": result.Status,
		}
		if strings.TrimSpace(result.Reason) != "" {
			entry["reason"] = result.Reason
		}
		if strings.TrimSpace(result.Details) != "" {
			entry["details"] = result.Details
		}
		results = append(results, entry)
	}
	for _, warningMessage := range summaryResult.WarningMessages {
		warnings = append(warnings, directWarning{
			Code:    "INDEX_UPDATE_FAILED",
			Message: warningMessage,
			Ref:     "Run 'rvn reindex' to rebuild the database",
		})
	}

	data := map[string]interface{}{
		"ok":          summaryResult.Errors == 0,
		"action":      summaryResult.Action,
		"results":     results,
		"total":       summaryResult.Total,
		"skipped":     summaryResult.Skipped,
		"errors":      summaryResult.Errors,
		"moved":       summaryResult.Moved,
		"destination": summaryResult.Destination,
	}
	return successEnvelope(data, warnings), false
}

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

	readsvc.SaveSearchResults(vaultPath, queryStr, results)

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
	readsvc.SaveBacklinksResults(vaultPath, resolved.ObjectID, links)

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
	readsvc.SaveOutlinksResults(vaultPath, resolved.ObjectID, links)

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
		readsvc.SaveObjectQueryResults(vaultPath, resolvedQuery, result.Objects)
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
	readsvc.SaveTraitQueryResults(vaultPath, resolvedQuery, result.Traits)
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

func (s *Server) callDirectSchemaValidate(args map[string]interface{}) (string, bool) {
	_ = args
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}

	result, err := schemasvc.Validate(vaultPath)
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return successEnvelope(map[string]interface{}{
		"valid":  result.Valid,
		"issues": result.Issues,
		"types":  result.Types,
		"traits": result.Traits,
	}, nil), false
}

func (s *Server) callDirectSchemaAddType(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	typeName := strings.TrimSpace(toString(normalized["name"]))
	defaultPath := toString(normalized["default-path"])
	nameField := toString(normalized["name-field"])
	description := toString(normalized["description"])

	result, err := schemasvc.AddType(schemasvc.AddTypeRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		DefaultPath: defaultPath,
		NameField:   nameField,
		Description: description,
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	data := map[string]interface{}{
		"added":        "type",
		"name":         result.Name,
		"default_path": result.DefaultPath,
	}
	if result.Description != "" {
		data["description"] = result.Description
	}
	if result.NameField != "" {
		data["name_field"] = result.NameField
		data["auto_created_field"] = result.AutoCreatedField
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectSchemaAddTrait(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	result, err := schemasvc.AddTrait(schemasvc.AddTraitRequest{
		VaultPath: vaultPath,
		TraitName: strings.TrimSpace(toString(normalized["name"])),
		TraitType: toString(normalized["type"]),
		Values:    toString(normalized["values"]),
		Default:   toString(normalized["default"]),
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	data := map[string]interface{}{
		"added": "trait",
		"name":  result.Name,
		"type":  result.Type,
	}
	if len(result.Values) > 0 {
		data["values"] = result.Values
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectSchemaAddField(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	result, err := schemasvc.AddField(schemasvc.AddFieldRequest{
		VaultPath:   vaultPath,
		TypeName:    strings.TrimSpace(toString(normalized["type_name"])),
		FieldName:   strings.TrimSpace(toString(normalized["field_name"])),
		FieldType:   toString(normalized["type"]),
		Required:    boolValue(normalized["required"]),
		Default:     toString(normalized["default"]),
		Values:      toString(normalized["values"]),
		Target:      toString(normalized["target"]),
		Description: toString(normalized["description"]),
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	data := map[string]interface{}{
		"added":      "field",
		"type":       result.TypeName,
		"field":      result.FieldName,
		"field_type": result.FieldType,
		"required":   result.Required,
	}
	if result.Description != "" {
		data["description"] = result.Description
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectSchemaUpdateType(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	typeName := strings.TrimSpace(toString(normalized["name"]))
	result, err := schemasvc.UpdateType(schemasvc.UpdateTypeRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		DefaultPath: toString(normalized["default-path"]),
		NameField:   toString(normalized["name-field"]),
		Description: toString(normalized["description"]),
		AddTrait:    toString(normalized["add-trait"]),
		RemoveTrait: toString(normalized["remove-trait"]),
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return successEnvelope(map[string]interface{}{
		"updated": "type",
		"name":    typeName,
		"changes": result.Changes,
	}, nil), false
}

func (s *Server) callDirectSchemaUpdateTrait(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	traitName := strings.TrimSpace(toString(normalized["name"]))
	result, err := schemasvc.UpdateTrait(schemasvc.UpdateTraitRequest{
		VaultPath: vaultPath,
		TraitName: traitName,
		TraitType: toString(normalized["type"]),
		Values:    toString(normalized["values"]),
		Default:   toString(normalized["default"]),
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return successEnvelope(map[string]interface{}{
		"updated": "trait",
		"name":    traitName,
		"changes": result.Changes,
	}, nil), false
}

func (s *Server) callDirectSchemaUpdateField(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	typeName := strings.TrimSpace(toString(normalized["type_name"]))
	fieldName := strings.TrimSpace(toString(normalized["field_name"]))
	result, err := schemasvc.UpdateField(schemasvc.UpdateFieldRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		FieldName:   fieldName,
		FieldType:   toString(normalized["type"]),
		Required:    toString(normalized["required"]),
		Default:     toString(normalized["default"]),
		Values:      toString(normalized["values"]),
		Target:      toString(normalized["target"]),
		Description: toString(normalized["description"]),
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return successEnvelope(map[string]interface{}{
		"updated": "field",
		"type":    typeName,
		"field":   fieldName,
		"changes": result.Changes,
	}, nil), false
}

func (s *Server) callDirectSchemaRemoveType(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	typeName := strings.TrimSpace(toString(normalized["name"]))
	result, err := schemasvc.RemoveType(schemasvc.RemoveTypeRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		Force:       boolValue(normalized["force"]),
		Interactive: false,
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return successEnvelope(map[string]interface{}{
		"removed": "type",
		"name":    typeName,
	}, schemaWarningsToDirect(result.Warnings)), false
}

func (s *Server) callDirectSchemaRemoveTrait(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	traitName := strings.TrimSpace(toString(normalized["name"]))
	result, err := schemasvc.RemoveTrait(schemasvc.RemoveTraitRequest{
		VaultPath:   vaultPath,
		TraitName:   traitName,
		Force:       boolValue(normalized["force"]),
		Interactive: false,
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return successEnvelope(map[string]interface{}{
		"removed": "trait",
		"name":    traitName,
	}, schemaWarningsToDirect(result.Warnings)), false
}

func (s *Server) callDirectSchemaRemoveField(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	typeName := strings.TrimSpace(toString(normalized["type_name"]))
	fieldName := strings.TrimSpace(toString(normalized["field_name"]))
	_, err = schemasvc.RemoveField(schemasvc.RemoveFieldRequest{
		VaultPath: vaultPath,
		TypeName:  typeName,
		FieldName: fieldName,
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return successEnvelope(map[string]interface{}{
		"removed": "field",
		"type":    typeName,
		"field":   fieldName,
	}, nil), false
}

func (s *Server) callDirectSchemaRenameField(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	result, err := schemasvc.RenameField(schemasvc.RenameFieldRequest{
		VaultPath: vaultPath,
		TypeName:  strings.TrimSpace(toString(normalized["type_name"])),
		OldField:  strings.TrimSpace(toString(normalized["old_field"])),
		NewField:  strings.TrimSpace(toString(normalized["new_field"])),
		Confirm:   boolValue(normalized["confirm"]),
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	if result.Preview {
		return successEnvelope(map[string]interface{}{
			"preview":       true,
			"type":          result.TypeName,
			"old_field":     result.OldField,
			"new_field":     result.NewField,
			"total_changes": result.TotalChanges,
			"changes":       result.Changes,
			"hint":          result.Hint,
		}, nil), false
	}

	return successEnvelope(map[string]interface{}{
		"renamed":         true,
		"type":            result.TypeName,
		"old_field":       result.OldField,
		"new_field":       result.NewField,
		"changes_applied": result.ChangesApplied,
		"hint":            result.Hint,
	}, nil), false
}

func (s *Server) callDirectSchemaRenameType(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)

	result, err := schemasvc.RenameType(schemasvc.RenameTypeRequest{
		VaultPath:         vaultPath,
		OldName:           strings.TrimSpace(toString(normalized["old_name"])),
		NewName:           strings.TrimSpace(toString(normalized["new_name"])),
		Confirm:           boolValue(normalized["confirm"]),
		RenameDefaultPath: boolValue(normalized["rename-default-path"]),
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	if result.Preview {
		data := map[string]interface{}{
			"preview":       true,
			"old_name":      result.OldName,
			"new_name":      result.NewName,
			"total_changes": result.TotalChanges,
			"changes":       result.Changes,
			"hint":          result.Hint,
		}
		if result.DefaultPathRenameAvailable {
			data["default_path_rename_available"] = true
			data["default_path_old"] = result.DefaultPathOld
			data["default_path_new"] = result.DefaultPathNew
			data["optional_total_changes"] = result.OptionalTotalChanges
			data["optional_changes"] = result.OptionalChanges
			data["files_to_move"] = result.FilesToMove
		}
		return successEnvelope(data, nil), false
	}

	data := map[string]interface{}{
		"renamed":         true,
		"old_name":        result.OldName,
		"new_name":        result.NewName,
		"changes_applied": result.ChangesApplied,
		"hint":            result.Hint,
	}
	if result.DefaultPathRenameAvailable {
		data["default_path_rename_available"] = true
		data["default_path_renamed"] = result.DefaultPathRenamed
		data["default_path_old"] = result.DefaultPathOld
		data["default_path_new"] = result.DefaultPathNew
		data["files_moved"] = result.FilesMoved
		data["reference_files_updated"] = result.ReferenceFilesUpdated
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectSchemaTemplateList(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}

	items, err := schemasvc.ListTemplates(vaultPath)
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}
	return successEnvelope(map[string]interface{}{"templates": items}, nil), false
}

func (s *Server) callDirectSchemaTemplateGet(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}

	normalized := normalizeArgs(args)
	templateID := strings.TrimSpace(toString(normalized["template_id"]))
	item, err := schemasvc.GetTemplate(vaultPath, templateID)
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}
	return successEnvelope(map[string]interface{}{
		"id":          item.ID,
		"file":        item.File,
		"description": item.Description,
	}, nil), false
}

func (s *Server) callDirectSchemaTemplateSet(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}

	normalized := normalizeArgs(args)
	templateID := strings.TrimSpace(toString(normalized["template_id"]))
	description := toString(normalized["description"])
	item, err := schemasvc.SetTemplate(schemasvc.SetTemplateRequest{
		VaultPath:   vaultPath,
		TemplateID:  templateID,
		File:        toString(normalized["file"]),
		Description: description,
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}
	return successEnvelope(map[string]interface{}{
		"id":          item.ID,
		"file":        item.File,
		"description": strings.TrimSpace(description),
	}, nil), false
}

func (s *Server) callDirectSchemaTemplateRemove(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}

	normalized := normalizeArgs(args)
	templateID := strings.TrimSpace(toString(normalized["template_id"]))
	if err := schemasvc.RemoveTemplate(vaultPath, templateID); err != nil {
		return mapDirectSchemaServiceError(err)
	}
	return successEnvelope(map[string]interface{}{
		"removed": true,
		"id":      templateID,
	}, nil), false
}

func (s *Server) callDirectSchemaTypeTemplateList(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)
	typeName := strings.TrimSpace(toString(normalized["type_name"]))

	state, err := schemasvc.ListTypeTemplates(vaultPath, typeName)
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}
	return successEnvelope(map[string]interface{}{
		"type":             typeName,
		"templates":        state.Templates,
		"default_template": state.DefaultTemplate,
	}, nil), false
}

func (s *Server) callDirectSchemaTypeTemplateSet(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)
	typeName := strings.TrimSpace(toString(normalized["type_name"]))
	templateID := strings.TrimSpace(toString(normalized["template_id"]))

	result, err := schemasvc.AddTypeTemplate(vaultPath, typeName, templateID)
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}
	if result.AlreadySet {
		return successEnvelope(map[string]interface{}{
			"type":          typeName,
			"template_id":   templateID,
			"already_set":   true,
			"default_match": result.DefaultMatch,
		}, nil), false
	}

	return successEnvelope(map[string]interface{}{
		"type":        typeName,
		"template_id": templateID,
	}, nil), false
}

func (s *Server) callDirectSchemaTypeTemplateRemove(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)
	typeName := strings.TrimSpace(toString(normalized["type_name"]))
	templateID := strings.TrimSpace(toString(normalized["template_id"]))

	if err := schemasvc.RemoveTypeTemplate(vaultPath, typeName, templateID); err != nil {
		return mapDirectSchemaServiceError(err)
	}
	return successEnvelope(map[string]interface{}{
		"type":        typeName,
		"template_id": templateID,
		"removed":     true,
	}, nil), false
}

func (s *Server) callDirectSchemaTypeTemplateDefault(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)
	typeName := strings.TrimSpace(toString(normalized["type_name"]))
	clearDefault := boolValue(normalized["clear"])
	templateID := strings.TrimSpace(toString(normalized["template_id"]))

	newDefault, err := schemasvc.SetTypeDefaultTemplate(vaultPath, typeName, templateID, clearDefault)
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}
	return successEnvelope(map[string]interface{}{
		"type":             typeName,
		"default_template": newDefault,
	}, nil), false
}

func (s *Server) callDirectSchemaCoreTemplateList(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)
	coreType := strings.TrimSpace(toString(normalized["core_type"]))

	state, err := schemasvc.ListCoreTemplates(vaultPath, coreType)
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}
	return successEnvelope(map[string]interface{}{
		"core_type":        coreType,
		"templates":        state.Templates,
		"default_template": state.DefaultTemplate,
	}, nil), false
}

func (s *Server) callDirectSchemaCoreTemplateSet(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)
	coreType := strings.TrimSpace(toString(normalized["core_type"]))
	templateID := strings.TrimSpace(toString(normalized["template_id"]))

	result, err := schemasvc.AddCoreTemplate(vaultPath, coreType, templateID)
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}
	if result.AlreadySet {
		return successEnvelope(map[string]interface{}{
			"core_type":     coreType,
			"template_id":   templateID,
			"already_set":   true,
			"default_match": result.DefaultMatch,
		}, nil), false
	}

	return successEnvelope(map[string]interface{}{
		"core_type":   coreType,
		"template_id": templateID,
	}, nil), false
}

func (s *Server) callDirectSchemaCoreTemplateRemove(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)
	coreType := strings.TrimSpace(toString(normalized["core_type"]))
	templateID := strings.TrimSpace(toString(normalized["template_id"]))

	if err := schemasvc.RemoveCoreTemplate(vaultPath, coreType, templateID); err != nil {
		return mapDirectSchemaServiceError(err)
	}
	return successEnvelope(map[string]interface{}{
		"core_type":   coreType,
		"template_id": templateID,
		"removed":     true,
	}, nil), false
}

func (s *Server) callDirectSchemaCoreTemplateDefault(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)
	coreType := strings.TrimSpace(toString(normalized["core_type"]))
	clearDefault := boolValue(normalized["clear"])
	templateID := strings.TrimSpace(toString(normalized["template_id"]))

	newDefault, err := schemasvc.SetCoreDefaultTemplate(vaultPath, coreType, templateID, clearDefault)
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}
	return successEnvelope(map[string]interface{}{
		"core_type":        coreType,
		"default_template": newDefault,
	}, nil), false
}

func mapDirectResolveError(err error, reference string) (string, bool) {
	var ambiguous *readsvc.AmbiguousRefError
	if errors.As(err, &ambiguous) {
		return errorEnvelope("REF_AMBIGUOUS", ambiguous.Error(), "Use a full object ID/path to disambiguate", nil), true
	}

	var notFound *readsvc.RefNotFoundError
	if errors.As(err, &notFound) {
		return errorEnvelope("REF_NOT_FOUND", notFound.Error(), "Check the object reference and run 'rvn reindex' if needed", nil), true
	}

	return errorEnvelope("REF_NOT_FOUND", fmt.Sprintf("reference '%s' not found", reference), "Check the object reference and run 'rvn reindex' if needed", nil), true
}

func formatSearchMatches(results []model.SearchMatch) []map[string]interface{} {
	formatted := make([]map[string]interface{}, len(results))
	for i, result := range results {
		formatted[i] = map[string]interface{}{
			"object_id": result.ObjectID,
			"title":     result.Title,
			"file_path": result.FilePath,
			"snippet":   result.Snippet,
			"rank":      result.Rank,
		}
	}
	return formatted
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

func mapDirectSchemaServiceError(err error) (string, bool) {
	var svcErr *schemasvc.Error
	if errors.As(err, &svcErr) {
		return errorEnvelope(string(svcErr.Code), svcErr.Message, svcErr.Suggestion, svcErr.Details), true
	}
	return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
}

func schemaWarningsToDirect(warnings []schemasvc.Warning) []directWarning {
	if len(warnings) == 0 {
		return nil
	}
	direct := make([]directWarning, 0, len(warnings))
	for _, warning := range warnings {
		direct = append(direct, directWarning{
			Code:    warning.Code,
			Message: warning.Message,
		})
	}
	return direct
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

func intValueDefault(v interface{}, defaultValue int) int {
	if v == nil {
		return defaultValue
	}

	switch val := v.(type) {
	case int:
		return val
	case int8:
		return int(val)
	case int16:
		return int(val)
	case int32:
		return int(val)
	case int64:
		return int(val)
	case uint:
		return int(val)
	case uint8:
		return int(val)
	case uint16:
		return int(val)
	case uint32:
		return int(val)
	case uint64:
		return int(val)
	case float32:
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			return defaultValue
		}
		return int(val)
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return defaultValue
		}
		return int(val)
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return int(i)
		}
		if f, err := val.Float64(); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return int(f)
		}
		return defaultValue
	case string:
		if strings.TrimSpace(val) == "" {
			return defaultValue
		}
		if i, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return i
		}
		return defaultValue
	default:
		return defaultValue
	}
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

func hasAnyArg(args map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		if _, ok := args[key]; ok {
			return true
		}
	}
	return false
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
