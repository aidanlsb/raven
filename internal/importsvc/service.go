package importsvc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

type Code string

const (
	CodeInvalidInput  Code = "INVALID_INPUT"
	CodeTypeNotFound  Code = "TYPE_NOT_FOUND"
	CodeSchemaInvalid Code = "SCHEMA_INVALID"
	CodeConfigInvalid Code = "CONFIG_INVALID"
)

type Error struct {
	Code    Code
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newError(code Code, msg string, err error) *Error {
	return &Error{Code: code, Message: msg, Err: err}
}

func AsError(err error) (*Error, bool) {
	var svcErr *Error
	if errors.As(err, &svcErr) {
		return svcErr, true
	}
	return nil, false
}

type MappingConfig struct {
	Type         string            `yaml:"type"`
	Key          string            `yaml:"key"`
	Map          map[string]string `yaml:"map"`
	ContentField string            `yaml:"content_field"`

	TypeField string                 `yaml:"type_field"`
	Types     map[string]TypeMapping `yaml:"types"`
}

type TypeMapping struct {
	Type         string            `yaml:"type"`
	Key          string            `yaml:"key"`
	Map          map[string]string `yaml:"map"`
	ContentField string            `yaml:"content_field"`
}

type BuildMappingConfigRequest struct {
	MappingFilePath string
	CLIType         string
	MapFlags        []string
	Key             string
	ContentField    string
}

func BuildMappingConfig(req BuildMappingConfigRequest) (*MappingConfig, error) {
	cfg := &MappingConfig{
		Map: make(map[string]string),
	}

	if strings.TrimSpace(req.MappingFilePath) != "" {
		data, err := os.ReadFile(req.MappingFilePath)
		if err != nil {
			return nil, newError(CodeInvalidInput, fmt.Sprintf("failed to read mapping file: %v", err), err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, newError(CodeInvalidInput, fmt.Sprintf("failed to parse mapping file: %v", err), err)
		}
	}

	if strings.TrimSpace(req.CLIType) != "" {
		cfg.Type = req.CLIType
	}

	for _, m := range req.MapFlags {
		parts := strings.SplitN(m, "=", 2)
		if len(parts) != 2 {
			return nil, newError(CodeInvalidInput, fmt.Sprintf("invalid --map format: %q (expected key=value)", m), nil)
		}
		cfg.Map[parts[0]] = parts[1]
	}

	if strings.TrimSpace(req.Key) != "" {
		cfg.Key = req.Key
	}
	if strings.TrimSpace(req.ContentField) != "" {
		cfg.ContentField = req.ContentField
	}

	if strings.TrimSpace(cfg.Type) == "" && strings.TrimSpace(cfg.TypeField) == "" {
		return nil, newError(CodeInvalidInput, "no type specified: provide a type argument or use a mapping file with 'type' or 'type_field'", nil)
	}

	return cfg, nil
}

func ReadJSONInput(filePath string, stdin io.Reader) ([]map[string]interface{}, error) {
	var data []byte
	var err error

	if strings.TrimSpace(filePath) != "" {
		data, err = os.ReadFile(filePath)
		if err != nil {
			return nil, newError(CodeInvalidInput, fmt.Sprintf("failed to read file %s: %v", filePath, err), err)
		}
	} else {
		data, err = io.ReadAll(stdin)
		if err != nil {
			return nil, newError(CodeInvalidInput, fmt.Sprintf("failed to read stdin: %v", err), err)
		}
	}

	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return nil, newError(CodeInvalidInput, "empty input", nil)
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(data, &items); err == nil {
		return items, nil
	}

	var single map[string]interface{}
	if err := json.Unmarshal(data, &single); err == nil {
		return []map[string]interface{}{single}, nil
	}

	return nil, newError(CodeInvalidInput, "input is not valid JSON (expected array or object)", nil)
}

func ValidateMappingTypes(cfg *MappingConfig, sch *schema.Schema) error {
	if cfg.Type != "" {
		if _, ok := sch.Types[cfg.Type]; !ok && !schema.IsBuiltinType(cfg.Type) {
			return newError(CodeTypeNotFound, fmt.Sprintf("type '%s' not found in schema", cfg.Type), nil)
		}
	}

	for sourceName, typeMapping := range cfg.Types {
		if _, ok := sch.Types[typeMapping.Type]; !ok && !schema.IsBuiltinType(typeMapping.Type) {
			return newError(CodeTypeNotFound, fmt.Sprintf("type '%s' (mapped from '%s') not found in schema", typeMapping.Type, sourceName), nil)
		}
	}

	return nil
}

type ItemConfig struct {
	TypeName     string
	FieldMap     map[string]string
	MatchKey     string
	ContentField string
}

func ResolveItemMapping(item map[string]interface{}, cfg *MappingConfig, sch *schema.Schema) (*ItemConfig, error) {
	result := &ItemConfig{}

	if cfg.TypeField != "" {
		sourceType, ok := item[cfg.TypeField]
		if !ok {
			return nil, fmt.Errorf("missing type field '%s'", cfg.TypeField)
		}
		sourceTypeStr, ok := sourceType.(string)
		if !ok {
			return nil, fmt.Errorf("type field '%s' is not a string", cfg.TypeField)
		}
		typeMapping, ok := cfg.Types[sourceTypeStr]
		if !ok {
			return nil, fmt.Errorf("no mapping for source type '%s'", sourceTypeStr)
		}
		result.TypeName = typeMapping.Type
		result.FieldMap = typeMapping.Map
		result.MatchKey = typeMapping.Key
		result.ContentField = typeMapping.ContentField
	} else {
		result.TypeName = cfg.Type
		result.FieldMap = cfg.Map
		result.MatchKey = cfg.Key
		result.ContentField = cfg.ContentField
	}

	if result.MatchKey == "" {
		if typeDef, ok := sch.Types[result.TypeName]; ok && typeDef != nil && typeDef.NameField != "" {
			result.MatchKey = typeDef.NameField
		}
	}
	if result.MatchKey == "" {
		return nil, fmt.Errorf("no match key: set --key or configure name_field for type '%s'", result.TypeName)
	}
	if result.FieldMap == nil {
		result.FieldMap = make(map[string]string)
	}

	return result, nil
}

func ApplyFieldMappings(item map[string]interface{}, fieldMap map[string]string) map[string]interface{} {
	result := make(map[string]interface{}, len(item))
	for inputKey, value := range item {
		if schemaField, ok := fieldMap[inputKey]; ok {
			result[schemaField] = value
		} else {
			result[inputKey] = value
		}
	}
	return result
}

func MatchKeyValue(mapped map[string]interface{}, matchKey string) (string, bool) {
	val, ok := mapped[matchKey]
	if !ok {
		return "", false
	}

	switch v := val.(type) {
	case string:
		if v == "" {
			return "", false
		}
		return v, true
	case float64:
		return fmt.Sprintf("%v", v), true
	default:
		return fmt.Sprintf("%v", v), true
	}
}

func ExtractContentField(mapped map[string]interface{}, contentField string) string {
	val, ok := mapped[contentField]
	if !ok {
		return ""
	}
	delete(mapped, contentField)

	switch v := val.(type) {
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

func FieldsToStringMap(fields map[string]interface{}, _ string) map[string]string {
	result := make(map[string]string, len(fields))
	for k, v := range fields {
		if k == "type" {
			continue
		}
		switch val := v.(type) {
		case string:
			result[k] = val
		case float64:
			if val == float64(int64(val)) {
				result[k] = fmt.Sprintf("%d", int64(val))
			} else {
				result[k] = fmt.Sprintf("%v", val)
			}
		case bool:
			result[k] = fmt.Sprintf("%v", val)
		case []interface{}:
			var parts []string
			for _, item := range val {
				parts = append(parts, fmt.Sprintf("%v", item))
			}
			result[k] = "[" + strings.Join(parts, ", ") + "]"
		case nil:
		default:
			result[k] = fmt.Sprintf("%v", val)
		}
	}
	return result
}

func ReplaceBodyContent(fileContent, newBody string) string {
	lines := strings.Split(fileContent, "\n")
	_, endLine, ok := parser.FrontmatterBounds(lines)
	if !ok || endLine == -1 {
		return newBody
	}

	var result strings.Builder
	for i := 0; i <= endLine; i++ {
		result.WriteString(lines[i])
		result.WriteString("\n")
	}
	result.WriteString("\n")
	result.WriteString(newBody)
	if !strings.HasSuffix(newBody, "\n") {
		result.WriteString("\n")
	}
	return result.String()
}

type ResultItem struct {
	ID      string                 `json:"id"`
	Action  string                 `json:"action"`
	File    string                 `json:"file,omitempty"`
	Reason  string                 `json:"reason,omitempty"`
	Code    string                 `json:"code,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

type RunRequest struct {
	VaultPath     string
	MappingConfig *MappingConfig
	Items         []map[string]interface{}
	DryRun        bool
	CreateOnly    bool
	UpdateOnly    bool
}

type RunResult struct {
	Results          []ResultItem
	WarningMessages  []string
	ChangedFilePaths []string
	VaultConfig      *config.VaultConfig
}

func Run(req RunRequest) (*RunResult, error) {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return nil, newError(CodeInvalidInput, "vault path is required", nil)
	}
	if req.MappingConfig == nil {
		return nil, newError(CodeInvalidInput, "mapping config is required", nil)
	}
	if len(req.Items) == 0 {
		return nil, newError(CodeInvalidInput, "no items to import", nil)
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return nil, newError(CodeSchemaInvalid, err.Error(), err)
	}
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return nil, newError(CodeConfigInvalid, err.Error(), err)
	}
	if err := ValidateMappingTypes(req.MappingConfig, sch); err != nil {
		return nil, err
	}

	result := &RunResult{
		Results:          make([]ResultItem, 0, len(req.Items)),
		WarningMessages:  []string{},
		ChangedFilePaths: []string{},
		VaultConfig:      vaultCfg,
	}

	objectsRoot := vaultCfg.GetObjectsRoot()
	pagesRoot := vaultCfg.GetPagesRoot()
	templateDir := vaultCfg.GetTemplateDirectory()

	for i, item := range req.Items {
		itemCfg, err := ResolveItemMapping(item, req.MappingConfig, sch)
		if err != nil {
			result.Results = append(result.Results, ResultItem{
				ID:     fmt.Sprintf("item[%d]", i),
				Action: "skipped",
				Reason: err.Error(),
			})
			continue
		}

		mapped := ApplyFieldMappings(item, itemCfg.FieldMap)
		contentValue := ""
		if itemCfg.ContentField != "" {
			contentValue = ExtractContentField(mapped, itemCfg.ContentField)
		}

		matchValue, ok := MatchKeyValue(mapped, itemCfg.MatchKey)
		if !ok {
			result.Results = append(result.Results, ResultItem{
				ID:     fmt.Sprintf("item[%d]", i),
				Action: "skipped",
				Reason: fmt.Sprintf("missing match key '%s'", itemCfg.MatchKey),
			})
			continue
		}

		targetPath := pages.ResolveTargetPathWithRoots(matchValue, itemCfg.TypeName, sch, objectsRoot, pagesRoot)
		exists := pages.Exists(vaultPath, targetPath)

		if exists && req.CreateOnly {
			result.Results = append(result.Results, ResultItem{
				ID:     targetPath,
				Action: "skipped",
				Reason: "already exists (--create-only)",
			})
			continue
		}
		if !exists && req.UpdateOnly {
			result.Results = append(result.Results, ResultItem{
				ID:     targetPath,
				Action: "skipped",
				Reason: "does not exist (--update-only)",
			})
			continue
		}

		if req.DryRun {
			action := "create"
			if exists {
				action = "update"
			}
			result.Results = append(result.Results, ResultItem{
				ID:     targetPath,
				Action: action,
				File:   pages.SlugifyPath(targetPath) + ".md",
			})
			continue
		}

		if exists {
			itemResult, warnMsgs, filePath := updateObject(vaultPath, targetPath, itemCfg.TypeName, mapped, contentValue, sch, vaultCfg)
			result.Results = append(result.Results, itemResult)
			result.WarningMessages = append(result.WarningMessages, warnMsgs...)
			if filePath != "" {
				result.ChangedFilePaths = append(result.ChangedFilePaths, filePath)
			}
			continue
		}

		itemResult, warnMsgs, filePath := createObject(vaultPath, matchValue, targetPath, itemCfg.TypeName, mapped, contentValue, sch, vaultCfg, templateDir, objectsRoot, pagesRoot)
		result.Results = append(result.Results, itemResult)
		result.WarningMessages = append(result.WarningMessages, warnMsgs...)
		if filePath != "" {
			result.ChangedFilePaths = append(result.ChangedFilePaths, filePath)
		}
	}

	return result, nil
}

func createObject(
	vaultPath, title, resolvedTargetPath, typeName string,
	fields map[string]interface{},
	content string,
	sch *schema.Schema,
	vaultCfg *config.VaultConfig,
	templateDir, objectsRoot, pagesRoot string,
) (ResultItem, []string, string) {
	warnings := []string{}

	typedFields := fieldsToSchemaValues(fields)
	delete(typedFields, "type")
	validatedTypedFields, warningMessages, err := fieldmutation.PrepareValidatedFieldMutationValues(
		typeName,
		nil,
		typedFields,
		sch,
		map[string]bool{"type": true, "alias": true},
		&fieldmutation.RefValidationContext{
			VaultPath:   vaultPath,
			VaultConfig: vaultCfg,
		},
	)
	if err != nil {
		return mutationErrorResult(resolvedTargetPath, err, ""), warnings, ""
	}
	warnings = append(warnings, warningMessages...)

	createResult, err := pages.Create(pages.CreateOptions{
		VaultPath:         vaultPath,
		TypeName:          typeName,
		Title:             title,
		TargetPath:        title,
		Fields:            validatedTypedFields,
		Schema:            sch,
		TemplateDir:       templateDir,
		ProtectedPrefixes: vaultCfg.ProtectedPrefixes,
		ObjectsRoot:       objectsRoot,
		PagesRoot:         pagesRoot,
	})
	if err != nil {
		return ResultItem{ID: resolvedTargetPath, Action: "error", Reason: err.Error()}, warnings, ""
	}

	if content != "" {
		if err := appendContentToFile(createResult.FilePath, content); err != nil {
			return ResultItem{ID: resolvedTargetPath, Action: "error", Reason: fmt.Sprintf("failed to write content: %v", err)}, warnings, ""
		}
	}

	return ResultItem{
		ID:     vaultCfg.FilePathToObjectID(createResult.RelativePath),
		Action: "created",
		File:   createResult.RelativePath,
	}, warnings, createResult.FilePath
}

func updateObject(
	vaultPath, targetPath, typeName string,
	fields map[string]interface{},
	newContent string,
	sch *schema.Schema,
	vaultCfg *config.VaultConfig,
) (ResultItem, []string, string) {
	warnings := []string{}

	slugPath := pages.SlugifyPath(targetPath)
	filePath := filepath.Join(vaultPath, slugPath)
	if !strings.HasSuffix(filePath, ".md") {
		filePath += ".md"
	}

	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return ResultItem{ID: targetPath, Action: "error", Reason: fmt.Sprintf("read error: %v", err)}, warnings, ""
	}

	fm, err := parser.ParseFrontmatter(string(fileData))
	if err != nil || fm == nil {
		return ResultItem{ID: targetPath, Action: "error", Reason: "failed to parse frontmatter"}, warnings, ""
	}

	typedUpdates := fieldsToSchemaValues(fields)
	delete(typedUpdates, "type")

	updatedFile, warningMessages, err := fieldmutation.PrepareValidatedFrontmatterMutationValues(
		string(fileData),
		fm,
		typeName,
		typedUpdates,
		sch,
		map[string]bool{"type": true, "alias": true},
		&fieldmutation.RefValidationContext{
			VaultPath:   vaultPath,
			VaultConfig: vaultCfg,
		},
	)
	if err != nil {
		return mutationErrorResult(targetPath, err, "update error"), warnings, ""
	}
	warnings = append(warnings, warningMessages...)

	if newContent != "" {
		updatedFile = ReplaceBodyContent(updatedFile, newContent)
	}

	if err := atomicfile.WriteFile(filePath, []byte(updatedFile), 0o644); err != nil {
		return ResultItem{ID: targetPath, Action: "error", Reason: fmt.Sprintf("write error: %v", err)}, warnings, ""
	}

	relPath, _ := filepath.Rel(vaultPath, filePath)
	return ResultItem{
		ID:     vaultCfg.FilePathToObjectID(relPath),
		Action: "updated",
		File:   relPath,
	}, warnings, filePath
}

func fieldsToSchemaValues(fields map[string]interface{}) map[string]schema.FieldValue {
	values := make(map[string]schema.FieldValue, len(fields))
	for key, value := range fields {
		values[key] = parser.FieldValueFromYAML(value)
	}
	return values
}

func mutationErrorResult(id string, err error, fallbackPrefix string) ResultItem {
	var unknownErr *fieldmutation.UnknownFieldMutationError
	if errors.As(err, &unknownErr) {
		return ResultItem{
			ID:      id,
			Action:  "error",
			Reason:  unknownErr.Error(),
			Code:    "UNKNOWN_FIELD",
			Details: unknownErr.Details(),
		}
	}

	var validationErr *fieldmutation.ValidationError
	if errors.As(err, &validationErr) {
		return ResultItem{
			ID:     id,
			Action: "error",
			Reason: validationErr.Error(),
			Code:   "VALIDATION_FAILED",
		}
	}

	reason := err.Error()
	if strings.TrimSpace(fallbackPrefix) != "" {
		reason = fmt.Sprintf("%s: %v", fallbackPrefix, err)
	}
	return ResultItem{
		ID:     id,
		Action: "error",
		Reason: reason,
	}
}

func appendContentToFile(filePath, content string) error {
	existing, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var result strings.Builder
	result.Write(existing)

	existingStr := string(existing)
	if !strings.HasSuffix(existingStr, "\n\n") {
		if strings.HasSuffix(existingStr, "\n") {
			result.WriteString("\n")
		} else {
			result.WriteString("\n\n")
		}
	}

	result.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		result.WriteString("\n")
	}

	return atomicfile.WriteFile(filePath, []byte(result.String()), 0o644)
}
