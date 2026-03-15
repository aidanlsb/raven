package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemapayload"
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
	return schemapayload.MapWarnings(warnings, func(code, message string) directWarning {
		return directWarning{
			Code:    code,
			Message: message,
		}
	})
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
	case objectsvc.ErrorRefNotFound:
		return "REF_NOT_FOUND"
	case objectsvc.ErrorRefAmbiguous:
		return "REF_AMBIGUOUS"
	case objectsvc.ErrorDatabase:
		return "DATABASE_ERROR"
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
