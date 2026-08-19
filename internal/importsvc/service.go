package importsvc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/slugs"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type Code = codes.ErrorCode

const (
	CodeInvalidInput  Code = codes.ErrInvalidInput
	CodeTypeNotFound  Code = codes.ErrTypeNotFound
	CodeSchemaInvalid Code = codes.ErrSchemaInvalid
	CodeConfigInvalid Code = codes.ErrConfigInvalid
)

func newError(code Code, msg string, err error) *svcerr.Error {
	return &svcerr.Error{Code: code, Message: msg, Err: err}
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
	Results         []ResultItem
	WarningMessages []string
	ChangeSet       mutation.ChangeSet
	VaultConfig     *config.VaultConfig
}

func Run(rt *vaultruntime.Runtime, req RunRequest) (*RunResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		message := "vault path is required"
		if rt == nil {
			message = "vault runtime is required"
		}
		return nil, newError(CodeInvalidInput, message, err)
	}
	vaultPath := strings.TrimSpace(rt.VaultPath)
	if req.MappingConfig == nil {
		return nil, newError(CodeInvalidInput, "mapping config is required", nil)
	}
	if len(req.Items) == 0 {
		return nil, newError(CodeInvalidInput, "no items to import", nil)
	}

	if rt.SchemaLoadErr != nil {
		return nil, newError(CodeSchemaInvalid, rt.SchemaLoadErr.Error(), rt.SchemaLoadErr)
	}
	if rt.Schema == nil {
		return nil, newError(CodeSchemaInvalid, "schema runtime is required", nil)
	}
	if rt.VaultCfg == nil {
		return nil, newError(CodeConfigInvalid, "vault config runtime is required", nil)
	}
	sch := rt.Schema
	vaultCfg := rt.VaultCfg
	if err := ValidateMappingTypes(req.MappingConfig, sch); err != nil {
		return nil, err
	}

	result := &RunResult{
		Results:         make([]ResultItem, 0, len(req.Items)),
		WarningMessages: []string{},
		ChangeSet:       mutation.NewChangeSet(),
		VaultConfig:     vaultCfg,
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

		targetName := importTargetName(matchValue)
		targetPath := pages.ResolveTargetPathWithRoots(targetName, itemCfg.TypeName, sch, objectsRoot, pagesRoot)
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
				File:   slugs.PathSlug(targetPath) + ".md",
			})
			continue
		}

		itemResult, warnMsgs, changes := applyObject(applyObjectRequest{
			VaultPath:   vaultPath,
			Exists:      exists,
			TargetName:  targetName,
			TargetPath:  targetPath,
			TypeName:    itemCfg.TypeName,
			Fields:      mapped,
			Content:     contentValue,
			Schema:      sch,
			VaultConfig: vaultCfg,
			ObjectsRoot: objectsRoot,
			PagesRoot:   pagesRoot,
			TemplateDir: templateDir,
			Runtime:     rt,
		})
		result.Results = append(result.Results, itemResult)
		result.WarningMessages = append(result.WarningMessages, warnMsgs...)
		result.ChangeSet.Merge(changes)
	}

	return result, nil
}

func importTargetName(matchValue string) string {
	return slugs.ComponentSlug(matchValue)
}

type applyObjectRequest struct {
	VaultPath   string
	Exists      bool
	TargetName  string
	TargetPath  string
	TypeName    string
	Fields      map[string]interface{}
	Content     string
	Schema      *schema.Schema
	VaultConfig *config.VaultConfig
	ObjectsRoot string
	PagesRoot   string
	TemplateDir string
	Runtime     *vaultruntime.Runtime
}

// applyObject creates or updates a single imported object by routing through
// objectsvc.Upsert. Sharing that primitive keeps import aligned with the rest of
// Raven on protected/excluded-path safeguards, field validation, and write
// semantics, instead of reimplementing them here.
//
// Import keeps one body nuance that upsert does not model: on create it appends
// the content field after any template body, while on update it replaces the
// body. So the body is only handed to upsert for replacement on update; on
// create the object is written without body content and the content is appended
// afterwards.
func applyObject(req applyObjectRequest) (ResultItem, []string, mutation.ChangeSet) {
	fieldValues := fieldsToSchemaValues(req.Fields)
	delete(fieldValues, "type")

	replaceBody := req.Exists && req.Content != ""

	result, err := objectsvc.Upsert(objectsvc.UpsertRequest{
		VaultPath:   req.VaultPath,
		TypeName:    req.TypeName,
		TargetPath:  req.TargetName,
		ReplaceBody: replaceBody,
		Content:     req.Content,
		FieldValues: fieldValues,
		VaultConfig: req.VaultConfig,
		Schema:      req.Schema,
		ObjectsRoot: req.ObjectsRoot,
		PagesRoot:   req.PagesRoot,
		TemplateDir: req.TemplateDir,
		Runtime:     req.Runtime,
	})
	if err != nil {
		return mutationErrorResult(req.TargetPath, err), nil, mutation.ChangeSet{}
	}

	if !req.Exists && req.Content != "" {
		if err := appendContentToFile(result.FilePath, req.Content); err != nil {
			return ResultItem{
				ID:     req.TargetPath,
				Action: "error",
				Reason: fmt.Sprintf("failed to write content: %v", err),
			}, result.WarningMessages, result.ChangeSet
		}
	}

	return ResultItem{
		ID:     req.VaultConfig.FilePathToObjectID(result.RelativePath),
		Action: importActionForUpsertStatus(result.Status),
		File:   result.RelativePath,
	}, result.WarningMessages, result.ChangeSet
}

// importActionForUpsertStatus maps an upsert status onto the action vocabulary
// used in import results. Import historically reports every existing-object
// write as "updated", so an unchanged upsert is reported the same way.
func importActionForUpsertStatus(status string) string {
	if status == "created" {
		return "created"
	}
	return "updated"
}

func fieldsToSchemaValues(fields map[string]interface{}) map[string]fieldvalue.FieldValue {
	values := make(map[string]fieldvalue.FieldValue, len(fields))
	for key, value := range fields {
		values[key] = parser.FieldValueFromYAML(value)
	}
	return values
}

func mutationErrorResult(id string, err error) ResultItem {
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

	if svcErr, ok := svcerr.AsError(err); ok {
		return ResultItem{
			ID:      id,
			Action:  "error",
			Reason:  svcErr.Error(),
			Code:    string(svcErr.Code),
			Details: svcErr.Details,
		}
	}

	return ResultItem{
		ID:     id,
		Action: "error",
		Reason: err.Error(),
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
