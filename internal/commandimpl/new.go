package commandimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/objectsvc"
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
	allFieldValues := mergeFieldInputs(fieldValues, typedFieldValues)

	result, err := objectsvc.Create(objectsvc.CreateRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		Title:       title,
		TargetPath:  targetPath,
		FieldValues: allFieldValues,
		VaultConfig: vaultCfg,
		Schema:      sch,
		ObjectsRoot: vaultCfg.GetObjectsRoot(),
		PagesRoot:   vaultCfg.GetPagesRoot(),
		TemplateDir: vaultCfg.GetTemplateDirectory(),
		TemplateID:  stringArg(req.Args, "template"),
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	warnings := autoReindexWarnings(vaultPath, vaultCfg, result.FilePath)

	return commandexec.SuccessWithWarnings(map[string]interface{}{
		"file":  result.RelativePath,
		"id":    vaultCfg.FilePathToObjectID(result.RelativePath),
		"title": title,
		"type":  typeName,
	}, warnings, nil)
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
	case objectsvc.ErrorFileNotFound:
		return "FILE_NOT_FOUND"
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
	case objectsvc.ErrorUnexpected:
		return "INTERNAL_ERROR"
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
	allFieldValues := mergeFieldInputs(fieldValues, typedFieldValues)

	_, hasContent := req.Args["content"]
	content := stringArg(req.Args, "content")

	result, err := objectsvc.Upsert(objectsvc.UpsertRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		Title:       title,
		TargetPath:  targetPath,
		ReplaceBody: hasContent,
		Content:     content,
		FieldValues: allFieldValues,
		VaultConfig: vaultCfg,
		Schema:      sch,
		ObjectsRoot: vaultCfg.GetObjectsRoot(),
		PagesRoot:   vaultCfg.GetPagesRoot(),
		TemplateDir: vaultCfg.GetTemplateDirectory(),
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	warnings := warningMessagesToCommandWarnings(result.WarningMessages, "UNKNOWN_FIELD")
	if result.Status == "created" || result.Status == "updated" {
		warnings = appendCommandWarnings(warnings, autoReindexWarnings(vaultPath, vaultCfg, result.FilePath))
	}

	return commandexec.SuccessWithWarnings(
		map[string]interface{}{
			"status": result.Status,
			"id":     vaultCfg.FilePathToObjectID(result.RelativePath),
			"file":   result.RelativePath,
			"type":   typeName,
			"title":  title,
		},
		warnings,
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

func commaStringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	switch v := args[key].(type) {
	case []string:
		return strings.Join(v, ",")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if item == nil {
				continue
			}
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, ",")
	default:
		return stringArg(args, key)
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
