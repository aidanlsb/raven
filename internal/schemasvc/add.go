package schemasvc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
)

type AddTypeRequest struct {
	VaultPath     string
	TypeName      string
	DefaultPath   string
	NameField     string
	Description   string
	RequireSchema bool
}

type AddTypeResult struct {
	Name             string
	DefaultPath      string
	Description      string
	NameField        string
	AutoCreatedField string
}

type AddTraitRequest struct {
	VaultPath string
	TraitName string
	TraitType string
	Values    string
	Default   string
}

type AddTraitResult struct {
	Name   string
	Type   string
	Values []string
}

type AddFieldRequest struct {
	VaultPath   string
	TypeName    string
	FieldName   string
	FieldType   string
	Required    bool
	Default     string
	Values      string
	Target      string
	Description string
}

type AddFieldResult struct {
	TypeName    string
	FieldName   string
	FieldType   string
	Required    bool
	Description string
}

type FieldTypeValidation struct {
	Valid      bool
	BaseType   string
	IsArray    bool
	Error      string
	Suggestion string
	Examples   []string
	ValidTypes []string
	TargetHint string
}

var validFieldTypes = map[string]bool{
	"string":   true,
	"number":   true,
	"url":      true,
	"date":     true,
	"datetime": true,
	"bool":     true,
	"enum":     true,
	"ref":      true,
}

func AddType(req AddTypeRequest) (*AddTypeResult, error) {
	typeName := strings.TrimSpace(req.TypeName)
	if typeName == "" {
		return nil, newError(ErrorInvalidInput, "type name cannot be empty", "", nil, nil)
	}

	sch, err := schema.Load(req.VaultPath)
	if err != nil {
		return nil, newError(ErrorSchemaNotFound, err.Error(), "Run 'rvn init' first", nil, err)
	}

	if _, exists := sch.Types[typeName]; exists {
		return nil, newError(ErrorObjectExists, fmt.Sprintf("type '%s' already exists", typeName), "", nil, nil)
	}
	if schema.IsBuiltinType(typeName) {
		return nil, newError(ErrorInvalidInput, fmt.Sprintf("'%s' is a built-in type", typeName), "Choose a different name", nil, nil)
	}

	defaultPath := strings.TrimSpace(req.DefaultPath)
	if defaultPath == "" {
		defaultPath = normalizeDirRoot(typeName)
	}

	schemaDoc, err := readSchemaDoc(req.VaultPath)
	if err != nil {
		return nil, err
	}

	typesNode := ensureMapNode(schemaDoc, "types")
	newType := make(map[string]interface{})
	newType["default_path"] = defaultPath

	description := strings.TrimSpace(req.Description)
	if description != "" {
		newType["description"] = description
	}

	nameField := strings.TrimSpace(req.NameField)
	autoCreatedField := ""
	if nameField != "" {
		newType["name_field"] = nameField
		fields := make(map[string]interface{})
		fields[nameField] = map[string]interface{}{
			"type":     "string",
			"required": true,
		}
		newType["fields"] = fields
		autoCreatedField = nameField
	}

	typesNode[typeName] = newType
	if err := writeSchemaDoc(req.VaultPath, schemaDoc); err != nil {
		return nil, err
	}

	return &AddTypeResult{
		Name:             typeName,
		DefaultPath:      defaultPath,
		Description:      description,
		NameField:        nameField,
		AutoCreatedField: autoCreatedField,
	}, nil
}

func AddTrait(req AddTraitRequest) (*AddTraitResult, error) {
	traitName := strings.TrimSpace(req.TraitName)
	if traitName == "" {
		return nil, newError(ErrorInvalidInput, "trait name cannot be empty", "", nil, nil)
	}

	sch, err := schema.Load(req.VaultPath)
	if err != nil {
		return nil, newError(ErrorSchemaNotFound, err.Error(), "Run 'rvn init' first", nil, err)
	}
	if _, exists := sch.Traits[traitName]; exists {
		return nil, newError(ErrorObjectExists, fmt.Sprintf("trait '%s' already exists", traitName), "", nil, nil)
	}

	traitType := strings.TrimSpace(req.TraitType)
	if traitType == "" {
		traitType = "string"
	}
	normalizedDefault := strings.TrimSpace(req.Default)
	trimmedValues := splitCommaValues(req.Values)

	schemaDoc, err := readSchemaDoc(req.VaultPath)
	if err != nil {
		return nil, err
	}

	traitsNode := ensureMapNode(schemaDoc, "traits")
	newTrait := make(map[string]interface{})
	newTrait["type"] = traitType
	if len(trimmedValues) > 0 {
		newTrait["values"] = trimmedValues
	}
	if normalizedDefault != "" {
		if traitType == "bool" || traitType == "boolean" {
			if normalizedDefault == "true" {
				newTrait["default"] = true
			} else if normalizedDefault == "false" {
				newTrait["default"] = false
			} else {
				newTrait["default"] = normalizedDefault
			}
		} else {
			newTrait["default"] = normalizedDefault
		}
	}
	traitsNode[traitName] = newTrait

	if err := writeSchemaDoc(req.VaultPath, schemaDoc); err != nil {
		return nil, err
	}

	result := &AddTraitResult{
		Name: traitName,
		Type: traitType,
	}
	result.Values = trimmedValues
	return result, nil
}

func AddField(req AddFieldRequest) (*AddFieldResult, error) {
	typeName := strings.TrimSpace(req.TypeName)
	fieldName := strings.TrimSpace(req.FieldName)
	if typeName == "" || fieldName == "" {
		return nil, newError(ErrorInvalidInput, "type and field names are required", "", nil, nil)
	}

	sch, err := schema.Load(req.VaultPath)
	if err != nil {
		return nil, newError(ErrorSchemaNotFound, err.Error(), "Run 'rvn init' first", nil, err)
	}

	typeDef, exists := sch.Types[typeName]
	if !exists {
		return nil, newError(
			ErrorTypeNotFound,
			fmt.Sprintf("type '%s' not found", typeName),
			"Add the type first with 'rvn schema add type'",
			nil,
			nil,
		)
	}
	if schema.IsBuiltinType(typeName) {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("cannot add fields to built-in type '%s'", typeName),
			"Built-in types (page, section, date) have fixed definitions. Use traits for additional metadata.",
			nil,
			nil,
		)
	}
	if typeDef.Fields != nil {
		if _, exists := typeDef.Fields[fieldName]; exists {
			return nil, newError(ErrorObjectExists, fmt.Sprintf("field '%s' already exists on type '%s'", fieldName, typeName), "", nil, nil)
		}
	}

	validation := validateFieldTypeSpec(req.FieldType, req.Target, req.Values, sch)
	if !validation.Valid {
		details := map[string]interface{}{
			"field_type":  req.FieldType,
			"valid_types": validation.ValidTypes,
		}
		if len(validation.Examples) > 0 {
			details["examples"] = validation.Examples
		}
		if validation.TargetHint != "" {
			details["target_hint"] = validation.TargetHint
		}
		return nil, newError(ErrorInvalidInput, validation.Error, validation.Suggestion, details, nil)
	}

	fieldType := validation.BaseType
	if fieldType == "" {
		fieldType = "string"
	}
	if validation.IsArray {
		fieldType += "[]"
	}

	schemaDoc, typesNode, err := readSchemaDocWithTypes(req.VaultPath)
	if err != nil {
		return nil, err
	}
	typeNode := ensureMapNode(typesNode, typeName)
	fieldsNode := ensureMapNode(typeNode, "fields")

	newField := make(map[string]interface{})
	newField["type"] = fieldType
	if req.Required {
		newField["required"] = true
	}
	if strings.TrimSpace(req.Default) != "" {
		newField["default"] = req.Default
	}
	if strings.TrimSpace(req.Values) != "" {
		newField["values"] = strings.Split(req.Values, ",")
	}
	if strings.TrimSpace(req.Target) != "" {
		newField["target"] = req.Target
	}
	if strings.TrimSpace(req.Description) != "" {
		newField["description"] = req.Description
	}
	fieldsNode[fieldName] = newField

	if err := writeSchemaDoc(req.VaultPath, schemaDoc); err != nil {
		return nil, err
	}

	return &AddFieldResult{
		TypeName:    typeName,
		FieldName:   fieldName,
		FieldType:   fieldType,
		Required:    req.Required,
		Description: req.Description,
	}, nil
}

func normalizeFieldTypeAlias(baseType string) string {
	switch strings.ToLower(baseType) {
	case "boolean":
		return "bool"
	default:
		return strings.ToLower(baseType)
	}
}

func splitCommaValues(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func validateFieldTypeSpec(fieldType, target, values string, sch *schema.Schema) FieldTypeValidation {
	result := FieldTypeValidation{
		ValidTypes: []string{"string", "number", "url", "date", "datetime", "bool", "enum", "ref"},
	}

	if fieldType == "" {
		fieldType = "string"
	}

	isArray := strings.HasSuffix(fieldType, "[]")
	baseType := normalizeFieldTypeAlias(strings.TrimSuffix(fieldType, "[]"))
	result.BaseType = baseType
	result.IsArray = isArray

	if sch != nil {
		if _, isSchemaType := sch.Types[baseType]; isSchemaType && !validFieldTypes[baseType] {
			result.Error = fmt.Sprintf("'%s' is a type name, not a field type", baseType)
			result.Suggestion = fmt.Sprintf("To reference objects of type '%s', use --type ref --target %s", baseType, baseType)
			if isArray {
				result.Examples = []string{
					fmt.Sprintf("--type ref[] --target %s  (array of %s references)", baseType, baseType),
				}
			} else {
				result.Examples = []string{
					fmt.Sprintf("--type ref --target %s  (single %s reference)", baseType, baseType),
					fmt.Sprintf("--type ref[] --target %s  (array of %s references)", baseType, baseType),
				}
			}
			return result
		}

		cleanType := strings.TrimSuffix(baseType, "[]")
		if _, isSchemaType := sch.Types[cleanType]; isSchemaType && !validFieldTypes[cleanType] {
			result.Error = fmt.Sprintf("'%s' is a type name, not a field type", cleanType)
			result.Suggestion = fmt.Sprintf("To reference an array of '%s' objects, use --type ref[] --target %s", cleanType, cleanType)
			result.Examples = []string{
				fmt.Sprintf("--type ref[] --target %s", cleanType),
			}
			return result
		}
	}

	if !validFieldTypes[baseType] {
		result.Error = fmt.Sprintf("'%s' is not a valid field type", fieldType)
		result.Suggestion = "Valid types: string, number, url, date, datetime, bool, enum, ref (add [] suffix for arrays)"
		result.Examples = []string{
			"--type string        (text)",
			"--type string[]      (array of text, e.g., tags)",
			"--type url           (web link)",
			"--type ref --target person   (reference to a person)",
			"--type ref[] --target person (array of person references)",
			"--type enum --values a,b,c   (single choice from list)",
		}
		return result
	}

	if baseType == "ref" && target == "" {
		result.Error = "ref fields require --target to specify which type they reference"
		result.Suggestion = "Add --target <type_name> to specify the referenced type"
		if sch != nil && len(sch.Types) > 0 {
			typeNames := make([]string, 0, len(sch.Types))
			for name := range sch.Types {
				typeNames = append(typeNames, name)
			}
			sort.Strings(typeNames)
			if len(typeNames) > 3 {
				typeNames = typeNames[:3]
			}
			result.Examples = []string{}
			for _, t := range typeNames {
				if isArray {
					result.Examples = append(result.Examples, fmt.Sprintf("--type ref[] --target %s", t))
				} else {
					result.Examples = append(result.Examples, fmt.Sprintf("--type ref --target %s", t))
				}
			}
		}
		result.TargetHint = "Available types can be listed with 'rvn schema types'"
		return result
	}

	if baseType == "enum" && values == "" {
		result.Error = "enum fields require --values to specify allowed values"
		result.Suggestion = "Add --values with comma-separated allowed values"
		result.Examples = []string{
			"--type enum --values active,paused,done",
			"--type enum[] --values red,green,blue  (allows multiple selections)",
		}
		return result
	}

	if target != "" && baseType != "ref" {
		result.Error = fmt.Sprintf("--target is only valid for ref fields, but type is '%s'", fieldType)
		result.Suggestion = "Either change --type to ref (or ref[]) or remove --target"
		result.Examples = []string{
			fmt.Sprintf("--type ref --target %s  (single reference)", target),
			fmt.Sprintf("--type ref[] --target %s  (array of references)", target),
		}
		return result
	}

	if target != "" && sch != nil {
		if _, exists := sch.Types[target]; !exists {
			if !schema.IsBuiltinType(target) {
				result.Error = fmt.Sprintf("target type '%s' does not exist in schema", target)
				result.Suggestion = fmt.Sprintf("Either create the type first with 'rvn schema add type %s' or use an existing type", target)
				if len(sch.Types) > 0 {
					typeNames := make([]string, 0, len(sch.Types))
					for name := range sch.Types {
						typeNames = append(typeNames, name)
					}
					sort.Strings(typeNames)
					result.TargetHint = fmt.Sprintf("Existing types: %s", strings.Join(typeNames, ", "))
				}
				return result
			}
		}
	}

	result.Valid = true
	return result
}

func normalizeDirRoot(root string) string {
	root = strings.TrimSpace(root)
	root = strings.Trim(root, "/")
	if root == "" {
		return ""
	}
	return root + "/"
}
