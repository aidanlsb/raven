package mcp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

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

	serviceResult, err := objectsvc.SetByReference(objectsvc.SetByReferenceRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		Schema:       sch,
		Reference:    reference,
		Updates:      updates,
		TypedUpdates: typedUpdates,
		ParseOptions: parseOptionsFromVaultConfig(vaultCfg),
	})
	if err != nil {
		return mapDirectServiceError(err)
	}

	maybeDirectReindexFile(vaultPath, serviceResult.FilePath, vaultCfg)

	data := map[string]interface{}{
		"file":           serviceResult.RelativePath,
		"object_id":      serviceResult.ObjectID,
		"type":           serviceResult.ObjectType,
		"updated_fields": serviceResult.ResolvedUpdates,
	}
	if serviceResult.Embedded {
		data["embedded"] = true
	}

	return successEnvelope(data, warningMessagesToDirectWarnings(serviceResult.WarningMessages, "UNKNOWN_FIELD")), false
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

	deletionCfg := vaultCfg.GetDeletionConfig()
	serviceResult, err := objectsvc.DeleteByReference(objectsvc.DeleteByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		Schema:      sch,
		Reference:   reference,
		Behavior:    deletionCfg.Behavior,
		TrashDir:    deletionCfg.TrashDir,
	})
	if err != nil {
		return mapDirectServiceError(err)
	}

	warnings := make([]directWarning, 0)
	if len(serviceResult.Backlinks) > 0 {
		backlinkIDs := make([]string, 0, len(serviceResult.Backlinks))
		for _, bl := range serviceResult.Backlinks {
			backlinkIDs = append(backlinkIDs, bl.SourceID)
		}
		warnings = append(warnings, directWarning{
			Code:    "HAS_BACKLINKS",
			Message: fmt.Sprintf("Object is referenced by %d other objects", len(serviceResult.Backlinks)),
			Ref:     strings.Join(backlinkIDs, ", "),
		})
	}
	for _, warningMsg := range serviceResult.WarningMessages {
		warnings = append(warnings, directWarning{
			Code:    "INDEX_UPDATE_FAILED",
			Message: warningMsg,
			Ref:     "Run 'rvn reindex' to rebuild the database",
		})
	}

	data := map[string]interface{}{
		"deleted":  serviceResult.ObjectID,
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

	serviceResult, err := objectsvc.MoveByReference(objectsvc.MoveByReferenceRequest{
		VaultPath:      vaultPath,
		VaultConfig:    vaultCfg,
		Schema:         sch,
		Reference:      source,
		Destination:    destination,
		UpdateRefs:     boolValueDefault(normalized["update-refs"], true),
		SkipTypeCheck:  boolValue(normalized["skip-type-check"]),
		ParseOptions:   parseOptionsFromVaultConfig(vaultCfg),
		FailOnIndexErr: false,
	})
	if err != nil {
		return mapDirectServiceError(err)
	}

	warnings := make([]directWarning, 0)
	if serviceResult.NeedsConfirm && serviceResult.TypeMismatch != nil {
		mismatch := serviceResult.TypeMismatch
		warnings = append(warnings, directWarning{
			Code: "TYPE_DIRECTORY_MISMATCH",
			Message: fmt.Sprintf("Moving to '%s/' which is the default directory for type '%s', but file has type '%s'",
				mismatch.DestinationDir, mismatch.ExpectedType, mismatch.ActualType),
			Ref: fmt.Sprintf("Use --skip-type-check to proceed, or change the file's type to '%s'", mismatch.ExpectedType),
		})
		return successEnvelope(map[string]interface{}{
			"source":        serviceResult.SourceID,
			"destination":   serviceResult.DestinationID,
			"needs_confirm": true,
			"reason":        serviceResult.Reason,
		}, warnings), false
	}

	for _, warningMessage := range serviceResult.WarningMessages {
		warnings = append(warnings, directWarning{
			Code:    "INDEX_UPDATE_FAILED",
			Message: warningMessage,
			Ref:     "Run 'rvn reindex' to rebuild the database",
		})
	}

	data := map[string]interface{}{
		"source":      serviceResult.SourceID,
		"destination": serviceResult.DestinationID,
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
