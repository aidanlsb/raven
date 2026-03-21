package objectsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
)

type ReclassifyRequest struct {
	VaultPath   string
	VaultConfig *config.VaultConfig
	Schema      *schema.Schema

	ObjectRef string
	ObjectID  string
	FilePath  string

	NewTypeName string
	FieldValues map[string]string

	NoMove     bool
	UpdateRefs bool
	Force      bool

	ParseOptions *parser.ParseOptions
}

type ReclassifyByReferenceRequest struct {
	VaultPath   string
	VaultConfig *config.VaultConfig
	Schema      *schema.Schema

	Reference string

	NewTypeName string
	FieldValues map[string]string

	NoMove     bool
	UpdateRefs bool
	Force      bool

	ParseOptions *parser.ParseOptions
}

type ReclassifyResult struct {
	ObjectID      string   `json:"object_id"`
	OldType       string   `json:"old_type"`
	NewType       string   `json:"new_type"`
	File          string   `json:"file"`
	Moved         bool     `json:"moved,omitempty"`
	OldPath       string   `json:"old_path,omitempty"`
	NewPath       string   `json:"new_path,omitempty"`
	UpdatedRefs   []string `json:"updated_refs,omitempty"`
	AddedFields   []string `json:"added_fields,omitempty"`
	DroppedFields []string `json:"dropped_fields,omitempty"`
	NeedsConfirm  bool     `json:"needs_confirm,omitempty"`
	Reason        string   `json:"reason,omitempty"`

	ChangedFilePath string   `json:"-"`
	WarningMessages []string `json:"-"`
}

func Reclassify(req ReclassifyRequest) (*ReclassifyResult, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(ErrorInvalidInput, "vault path is required", "", nil, nil)
	}
	if req.VaultConfig == nil {
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}
	if req.Schema == nil {
		return nil, newError(ErrorValidationFailed, "schema is required", "Fix schema.yaml and try again", nil, nil)
	}
	if strings.TrimSpace(req.FilePath) == "" {
		return nil, newError(ErrorInvalidInput, "file path is required", "", nil, nil)
	}
	if strings.TrimSpace(req.ObjectID) == "" {
		return nil, newError(ErrorInvalidInput, "object ID is required", "", nil, nil)
	}
	if strings.TrimSpace(req.NewTypeName) == "" {
		return nil, newError(ErrorInvalidInput, "new type is required", "Usage: rvn reclassify <object> <new-type>", nil, nil)
	}

	contentBytes, err := os.ReadFile(req.FilePath)
	if err != nil {
		return nil, newError(ErrorFileRead, "failed to read file", "", nil, err)
	}
	content := string(contentBytes)

	fm, err := parser.ParseFrontmatter(content)
	if err != nil {
		return nil, newError(ErrorInvalidInput, "failed to parse frontmatter", "The file must have YAML frontmatter (---) to reclassify", nil, err)
	}
	if fm == nil {
		return nil, newError(ErrorInvalidInput, "file has no frontmatter", "The file must have YAML frontmatter (---) to reclassify", nil, nil)
	}

	oldType := fm.ObjectType
	if oldType == "" {
		oldType = "page"
	}

	if req.NewTypeName == oldType {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("object is already type '%s'", oldType),
			"Specify a different target type",
			nil,
			nil,
		)
	}

	if schema.IsBuiltinType(req.NewTypeName) {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("cannot reclassify to built-in type '%s'", req.NewTypeName),
			"Built-in types (page, section, date) cannot be used as reclassify targets",
			nil,
			nil,
		)
	}

	newTypeDef, typeExists := req.Schema.Types[req.NewTypeName]
	if !typeExists {
		var typeNames []string
		for name := range req.Schema.Types {
			typeNames = append(typeNames, name)
		}
		sort.Strings(typeNames)
		return nil, newError(
			ErrorTypeNotFound,
			fmt.Sprintf("type '%s' not found", req.NewTypeName),
			fmt.Sprintf("Available types: %s", strings.Join(typeNames, ", ")),
			map[string]interface{}{"available_types": typeNames},
			nil,
		)
	}

	fieldValues := make(map[string]string, len(req.FieldValues))
	for k, v := range req.FieldValues {
		fieldValues[k] = v
	}

	addedFields, missingFieldNames, missingFieldDetails := resolveRequiredFieldsForReclassify(fm, newTypeDef, fieldValues)
	if len(missingFieldNames) > 0 {
		objectRef := req.ObjectRef
		if strings.TrimSpace(objectRef) == "" {
			objectRef = req.ObjectID
		}

		details := map[string]interface{}{
			"missing_fields": missingFieldDetails,
			"object_id":      req.ObjectID,
			"old_type":       oldType,
			"new_type":       req.NewTypeName,
			"retry_with": map[string]interface{}{
				"object":   objectRef,
				"new_type": req.NewTypeName,
				"field":    buildReclassifyFieldTemplate(missingFieldNames),
			},
		}

		return nil, newError(
			ErrorRequiredField,
			fmt.Sprintf("Missing required fields for type '%s': %s", req.NewTypeName, strings.Join(missingFieldNames, ", ")),
			fmt.Sprintf("Retry with: field: {%s}", buildFieldTemplateExample(missingFieldNames)),
			details,
			nil,
		)
	}

	droppedFields := collectDroppedFieldsForReclassify(fm, newTypeDef)

	relPath, err := filepath.Rel(req.VaultPath, req.FilePath)
	if err != nil {
		relPath = req.FilePath
	}

	result := &ReclassifyResult{
		ObjectID:      req.ObjectID,
		OldType:       oldType,
		NewType:       req.NewTypeName,
		File:          relPath,
		AddedFields:   addedFields,
		DroppedFields: droppedFields,
	}

	if len(droppedFields) > 0 && !req.Force {
		result.NeedsConfirm = true
		result.Reason = fmt.Sprintf("Fields not defined on type '%s' will be dropped: %s", req.NewTypeName, strings.Join(droppedFields, ", "))
		return result, nil
	}

	newContent, err := updateFrontmatterForReclassify(content, req.NewTypeName, fieldValues, droppedFields)
	if err != nil {
		return nil, newError(ErrorFileWrite, "failed to update frontmatter", "", nil, err)
	}

	if err := atomicfile.WriteFile(req.FilePath, []byte(newContent), 0o644); err != nil {
		return nil, newError(ErrorFileWrite, "failed to write file", "", nil, err)
	}

	result.ChangedFilePath = req.FilePath

	if !req.NoMove && newTypeDef != nil && strings.TrimSpace(newTypeDef.DefaultPath) != "" {
		defaultDir := strings.TrimSuffix(newTypeDef.DefaultPath, "/")
		currentDir := filepath.Dir(relPath)

		currentObjDir := req.VaultConfig.FilePathToObjectID(currentDir)
		if currentObjDir == "" {
			currentObjDir = currentDir
		}

		if currentObjDir != defaultDir {
			filename := filepath.Base(relPath)
			destRelPath := req.VaultConfig.ResolveReferenceToFilePath(
				strings.TrimSuffix(filepath.Join(defaultDir, strings.TrimSuffix(filename, ".md")), ".md"),
			)
			destRelPath = paths.EnsureMDExtension(destRelPath)
			destAbsPath := filepath.Join(req.VaultPath, destRelPath)

			if _, err := os.Stat(destAbsPath); os.IsNotExist(err) {
				destinationObjectID := req.VaultConfig.FilePathToObjectID(destRelPath)

				moveResult, err := MoveFile(MoveFileRequest{
					VaultPath:         req.VaultPath,
					SourceFile:        req.FilePath,
					DestinationFile:   destAbsPath,
					SourceObjectID:    req.ObjectID,
					DestinationObject: destinationObjectID,
					UpdateRefs:        req.UpdateRefs,
					FailOnIndexError:  false,
					VaultConfig:       req.VaultConfig,
					Schema:            req.Schema,
					ParseOptions:      req.ParseOptions,
				})
				if err != nil {
					return nil, err
				}

				result.Moved = true
				result.OldPath = relPath
				result.NewPath = destRelPath
				result.File = destRelPath
				result.ObjectID = destinationObjectID
				result.UpdatedRefs = moveResult.UpdatedRefs
				result.WarningMessages = append(result.WarningMessages, moveResult.WarningMessages...)
				result.ChangedFilePath = destAbsPath
			}
		}
	}

	return result, nil
}

func ReclassifyByReference(req ReclassifyByReferenceRequest) (*ReclassifyResult, error) {
	if strings.TrimSpace(req.Reference) == "" {
		return nil, newError(ErrorInvalidInput, "reference is required", "Usage: rvn reclassify <object> <new-type>", nil, nil)
	}

	resolved, err := resolveReferenceForMutation(req.VaultPath, req.VaultConfig, req.Schema, req.Reference)
	if err != nil {
		return nil, err
	}

	return Reclassify(ReclassifyRequest{
		VaultPath:    req.VaultPath,
		VaultConfig:  req.VaultConfig,
		Schema:       req.Schema,
		ObjectRef:    req.Reference,
		ObjectID:     resolved.ObjectID,
		FilePath:     resolved.FilePath,
		NewTypeName:  req.NewTypeName,
		FieldValues:  req.FieldValues,
		NoMove:       req.NoMove,
		UpdateRefs:   req.UpdateRefs,
		Force:        req.Force,
		ParseOptions: req.ParseOptions,
	})
}

func resolveRequiredFieldsForReclassify(
	fm *parser.Frontmatter,
	newTypeDef *schema.TypeDefinition,
	fieldValues map[string]string,
) ([]string, []string, []map[string]interface{}) {
	if newTypeDef == nil {
		return nil, nil, nil
	}

	var fieldNames []string
	for name := range newTypeDef.Fields {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)

	var addedFields []string
	var missingFieldNames []string
	var missingFieldDetails []map[string]interface{}

	for _, fieldName := range fieldNames {
		fieldDef := newTypeDef.Fields[fieldName]
		if fieldDef == nil || !fieldDef.Required {
			continue
		}

		if fm.Fields != nil {
			if _, ok := fm.Fields[fieldName]; ok {
				continue
			}
		}

		if _, ok := fieldValues[fieldName]; ok {
			addedFields = append(addedFields, fieldName)
			continue
		}

		if fieldDef.Default != nil {
			fieldValues[fieldName] = fmt.Sprintf("%v", fieldDef.Default)
			addedFields = append(addedFields, fieldName)
			continue
		}

		missingFieldNames = append(missingFieldNames, fieldName)
		detail := map[string]interface{}{
			"name":     fieldName,
			"type":     string(fieldDef.Type),
			"required": true,
		}
		if len(fieldDef.Values) > 0 {
			detail["values"] = fieldDef.Values
		}
		missingFieldDetails = append(missingFieldDetails, detail)
	}

	return addedFields, missingFieldNames, missingFieldDetails
}

func collectDroppedFieldsForReclassify(fm *parser.Frontmatter, newTypeDef *schema.TypeDefinition) []string {
	if fm == nil || fm.Fields == nil || newTypeDef == nil {
		return nil
	}

	var dropped []string
	for fieldName := range fm.Fields {
		if fieldName == "type" {
			continue
		}
		if _, ok := newTypeDef.Fields[fieldName]; !ok {
			dropped = append(dropped, fieldName)
		}
	}
	sort.Strings(dropped)
	return dropped
}

func updateFrontmatterForReclassify(content, newType string, fieldValues map[string]string, droppedFields []string) (string, error) {
	lines := strings.Split(content, "\n")

	startLine, endLine, ok := parser.FrontmatterBounds(lines)
	if !ok {
		return "", fmt.Errorf("no frontmatter found")
	}
	if endLine == -1 {
		return "", fmt.Errorf("unclosed frontmatter")
	}

	frontmatterContent := strings.Join(lines[startLine+1:endLine], "\n")
	var yamlData map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterContent), &yamlData); err != nil {
		return "", fmt.Errorf("failed to parse frontmatter: %w", err)
	}
	if yamlData == nil {
		yamlData = make(map[string]interface{})
	}

	yamlData["type"] = newType
	for key, value := range fieldValues {
		yamlData[key] = value
	}

	droppedSet := make(map[string]bool, len(droppedFields))
	for _, f := range droppedFields {
		droppedSet[f] = true
	}
	for key := range yamlData {
		if droppedSet[key] {
			delete(yamlData, key)
		}
	}

	newFrontmatter, err := yaml.Marshal(yamlData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	var result strings.Builder
	result.WriteString("---\n")
	result.Write(newFrontmatter)
	result.WriteString("---")
	if endLine+1 < len(lines) {
		result.WriteString("\n")
		result.WriteString(strings.Join(lines[endLine+1:], "\n"))
	}

	return result.String(), nil
}

func buildReclassifyFieldTemplate(missingFields []string) map[string]string {
	result := make(map[string]string, len(missingFields))
	for _, f := range missingFields {
		result[f] = "<value>"
	}
	return result
}
