package commandimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

// HandleNew executes the canonical `new` command.
func HandleNew(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return commandexec.Failure("SCHEMA_INVALID", "failed to load schema", nil, "Fix schema.yaml and try again")
	}

	typeName := strings.TrimSpace(stringArg(req.Args, "type"))
	title := strings.TrimSpace(stringArg(req.Args, "title"))
	targetPath := strings.TrimSpace(stringArg(req.Args, "path"))
	if targetPath == "" {
		targetPath = title
	}

	fieldValues, err := parseKeyValueArgs(req.Args["field"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", "invalid --field payload", nil, err.Error())
	}

	typedFieldValues, err := parseTypedFieldValues(req.Args["field-json"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", "invalid --field-json payload", nil, "Provide a JSON object, e.g. --field-json '{\"status\":\"active\"}'")
	}

	result, err := objectsvc.Create(objectsvc.CreateRequest{
		VaultPath:        vaultPath,
		TypeName:         typeName,
		Title:            title,
		TargetPath:       targetPath,
		FieldValues:      fieldValues,
		TypedFieldValues: typedFieldValues,
		VaultConfig:      vaultCfg,
		Schema:           sch,
		ObjectsRoot:      vaultCfg.GetObjectsRoot(),
		PagesRoot:        vaultCfg.GetPagesRoot(),
		TemplateDir:      vaultCfg.GetTemplateDirectory(),
		TemplateID:       stringArg(req.Args, "template"),
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	maybeReindexFile(vaultPath, result.FilePath, vaultCfg)

	return commandexec.Success(map[string]interface{}{
		"file":  result.RelativePath,
		"id":    vaultCfg.FilePathToObjectID(result.RelativePath),
		"title": title,
		"type":  typeName,
	}, nil)
}

func mapContentMutationError(err error) commandexec.Result {
	var svcErr *objectsvc.Error
	if errors.As(err, &svcErr) {
		return commandexec.Failure(mapServiceCode(svcErr.Code), svcErr.Message, svcErr.Details, svcErr.Suggestion)
	}

	var unknownErr *fieldmutation.UnknownFieldMutationError
	if errors.As(err, &unknownErr) {
		return commandexec.Failure("UNKNOWN_FIELD", unknownErr.Error(), unknownErr.Details(), unknownErr.Suggestion())
	}

	var validationErr *fieldmutation.ValidationError
	if errors.As(err, &validationErr) {
		return commandexec.Failure("VALIDATION_FAILED", validationErr.Error(), nil, validationErr.Suggestion())
	}

	return commandexec.Failure("UNEXPECTED", err.Error(), nil, "")
}

func mapServiceCode(code objectsvc.ErrorCode) string {
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

// HandleUpsert executes the canonical `upsert` command.
func HandleUpsert(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return commandexec.Failure("SCHEMA_INVALID", "failed to load schema", nil, "Fix schema.yaml and try again")
	}

	typeName := strings.TrimSpace(stringArg(req.Args, "type"))
	title := strings.TrimSpace(stringArg(req.Args, "title"))
	targetPath := strings.TrimSpace(stringArg(req.Args, "path"))
	if targetPath == "" {
		targetPath = title
	}

	fieldValues, err := parseKeyValueArgs(req.Args["field"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", "invalid --field payload", nil, err.Error())
	}

	typedFieldValues, err := parseTypedFieldValues(req.Args["field-json"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", "invalid --field-json payload", nil, "Provide a JSON object, e.g. --field-json '{\"status\":\"active\"}'")
	}

	_, hasContent := req.Args["content"]
	content := stringArg(req.Args, "content")

	result, err := objectsvc.Upsert(objectsvc.UpsertRequest{
		VaultPath:        vaultPath,
		TypeName:         typeName,
		Title:            title,
		TargetPath:       targetPath,
		ReplaceBody:      hasContent,
		Content:          content,
		FieldValues:      fieldValues,
		TypedFieldValues: typedFieldValues,
		VaultConfig:      vaultCfg,
		Schema:           sch,
		ObjectsRoot:      vaultCfg.GetObjectsRoot(),
		PagesRoot:        vaultCfg.GetPagesRoot(),
		TemplateDir:      vaultCfg.GetTemplateDirectory(),
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	if result.Status == "created" || result.Status == "updated" {
		maybeReindexFile(vaultPath, result.FilePath, vaultCfg)
	}

	return commandexec.SuccessWithWarnings(
		map[string]interface{}{
			"status": result.Status,
			"id":     vaultCfg.FilePathToObjectID(result.RelativePath),
			"file":   result.RelativePath,
			"type":   typeName,
			"title":  title,
		},
		warningMessagesToCommandWarnings(result.WarningMessages, "UNKNOWN_FIELD"),
		nil,
	)
}

func warningMessagesToCommandWarnings(messages []string, code string) []commandexec.Warning {
	if len(messages) == 0 {
		return nil
	}

	warnings := make([]commandexec.Warning, 0, len(messages))
	for _, message := range messages {
		message = strings.TrimSpace(message)
		if message == "" {
			continue
		}
		warnings = append(warnings, commandexec.Warning{
			Code:    code,
			Message: message,
		})
	}
	return warnings
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	switch v := args[key].(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func boolArg(args map[string]any, key string) bool {
	if args == nil {
		return false
	}
	value, ok := args[key]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

func intArg(args map[string]any, key string) (int, bool) {
	if args == nil {
		return 0, false
	}
	value, ok := args[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	default:
		return 0, false
	}
}

func parseKeyValueArgs(raw any) (map[string]string, error) {
	if raw == nil {
		return map[string]string{}, nil
	}

	out := make(map[string]string)
	for _, pair := range keyValuePairs(raw) {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("use --field key=value")
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}

func keyValuePairs(v any) []string {
	switch val := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for key := range val {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		values := make([]string, 0, len(keys))
		for _, key := range keys {
			values = append(values, fmt.Sprintf("%s=%v", key, val[key]))
		}
		return values
	case map[string]string:
		keys := make([]string, 0, len(val))
		for key := range val {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		values := make([]string, 0, len(keys))
		for _, key := range keys {
			values = append(values, key+"="+val[key])
		}
		return values
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return nil
		}
		return []string{s}
	case []string:
		values := make([]string, 0, len(val))
		for _, item := range val {
			item = strings.TrimSpace(item)
			if item != "" {
				values = append(values, item)
			}
		}
		return values
	case []interface{}:
		values := make([]string, 0, len(val))
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != "" {
				values = append(values, s)
			}
		}
		return values
	default:
		return nil
	}
}

func parseTypedFieldValues(raw any) (map[string]schema.FieldValue, error) {
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

func maybeReindexFile(vaultPath, filePath string, vaultCfg *config.VaultConfig) {
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
