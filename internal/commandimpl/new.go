package commandimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
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

	data := map[string]interface{}{
		"file":  result.RelativePath,
		"id":    vaultCfg.FilePathToObjectID(result.RelativePath),
		"title": title,
		"type":  typeName,
	}
	missingData, missingWarnings := missingRefEnvelope(vaultPath, vaultCfg, sch, result.RelativePath)
	data = mergeDataFields(data, missingData)
	warnings = appendCommandWarnings(warnings, missingWarnings)

	return commandexec.SuccessWithWarnings(data, warnings, nil)
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

	return commandexec.Failure(codes.ErrInternal, err.Error(), nil, "")
}

func mapServiceCode(code objectsvc.ErrorCode) codes.ErrorCode {
	if codes.IsErrorCode(string(code)) {
		return code
	}
	return codes.ErrInternal
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

	content, hasContent, contentErr := upsertBodyContent(req)
	if contentErr != nil {
		return *contentErr
	}

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

	warnings := warningMessagesToCommandWarnings(result.WarningMessages, codes.WarnUnknownField)
	data := map[string]interface{}{
		"status": result.Status,
		"id":     vaultCfg.FilePathToObjectID(result.RelativePath),
		"file":   result.RelativePath,
		"type":   typeName,
		"title":  title,
	}
	if result.Status == "created" || result.Status == "updated" {
		warnings = appendCommandWarnings(warnings, autoReindexWarnings(vaultPath, vaultCfg, result.FilePath))
		missingData, missingWarnings := missingRefEnvelope(vaultPath, vaultCfg, sch, result.RelativePath)
		data = mergeDataFields(data, missingData)
		warnings = appendCommandWarnings(warnings, missingWarnings)
	}

	return commandexec.SuccessWithWarnings(
		data,
		warnings,
		nil,
	)
}

func upsertBodyContent(req commandexec.Request) (string, bool, *commandexec.Result) {
	_, hasContent := req.Args["content"]
	content := stringArg(req.Args, "content")

	_, hasContentFile := req.Args["content-file"]
	contentFile := strings.TrimSpace(stringArg(req.Args, "content-file"))
	if hasContent && hasContentFile {
		result := commandexec.Failure(
			codes.ErrInvalidInput,
			"--content and --content-file are mutually exclusive",
			nil,
			"Use only one body input mode",
		)
		return "", false, &result
	}
	if !hasContentFile {
		return content, hasContent, nil
	}
	if contentFile == "" {
		result := commandexec.Failure(
			codes.ErrInvalidInput,
			"--content-file requires a path or '-'",
			nil,
			"Provide a file path or use --content-file - to read from stdin",
		)
		return "", false, &result
	}
	if contentFile == "-" {
		return string(req.Stdin), true, nil
	}

	data, err := os.ReadFile(contentFile)
	if err != nil {
		result := commandexec.Failure(
			codes.ErrFileRead,
			"failed to read --content-file",
			map[string]interface{}{"path": contentFile},
			err.Error(),
		)
		return "", false, &result
	}
	return string(data), true, nil
}

func warningMessagesToCommandWarnings(messages []string, code codes.WarningCode) []commandexec.Warning {
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
