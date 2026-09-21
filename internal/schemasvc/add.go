package schemasvc

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type AddTypeRequest struct {
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

func AddType(rt *vaultruntime.Runtime, req AddTypeRequest) (*AddTypeResult, error) {
	typeName := strings.TrimSpace(req.TypeName)
	if typeName == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "type name cannot be empty")
	}

	if schema.IsBuiltinType(typeName) {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type", typeName)).WithSuggestion("Choose a different name")
	}

	defaultPath := strings.TrimSpace(req.DefaultPath)
	if defaultPath == "" {
		defaultPath = normalizeDirRoot(typeName)
	}

	description := strings.TrimSpace(req.Description)
	nameField := strings.TrimSpace(req.NameField)
	autoCreatedField := ""
	err := editRuntimeSchema(rt, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		if _, exists := doc.Schema().Types[typeName]; exists {
			return svcerr.New(codes.ErrObjectExists, fmt.Sprintf("type '%s' already exists", typeName))
		}

		typesNode := schemadoc.EnsureMap(doc.Root(), "types")
		newType := map[string]interface{}{
			"default_path": defaultPath,
		}
		if description != "" {
			newType["description"] = description
		}
		if nameField != "" {
			newType["name_field"] = nameField
			newType["fields"] = map[string]interface{}{
				nameField: map[string]interface{}{
					"type":     "string",
					"required": true,
				},
			}
			autoCreatedField = nameField
		}
		typesNode[typeName] = newType
		return nil
	})
	if err != nil {
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

func AddTrait(rt *vaultruntime.Runtime, req AddTraitRequest) (*AddTraitResult, error) {
	traitName := strings.TrimSpace(req.TraitName)
	if traitName == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "trait name cannot be empty")
	}

	traitType := normalizeTraitTypeInput(req.TraitType)
	trimmedValues := splitCommaValues(req.Values)

	err := editRuntimeSchema(rt, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		if _, exists := doc.Schema().Traits[traitName]; exists {
			return svcerr.New(codes.ErrObjectExists, fmt.Sprintf("trait '%s' already exists", traitName))
		}

		traitsNode := schemadoc.EnsureMap(doc.Root(), "traits")
		newTrait := map[string]interface{}{
			"type": traitType,
		}
		if len(trimmedValues) > 0 {
			newTrait["values"] = trimmedValues
		}
		if normalizedDefault, ok := normalizeTraitDefaultValue(traitType, req.Default); ok {
			newTrait["default"] = normalizedDefault
		}
		traitsNode[traitName] = newTrait
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := &AddTraitResult{
		Name: traitName,
		Type: traitType,
	}
	result.Values = trimmedValues
	return result, nil
}

func AddField(rt *vaultruntime.Runtime, req AddFieldRequest) (*AddFieldResult, error) {
	typeName := strings.TrimSpace(req.TypeName)
	fieldName := strings.TrimSpace(req.FieldName)
	if typeName == "" || fieldName == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "type and field names are required")
	}

	if schema.IsBuiltinType(typeName) {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("cannot add fields to built-in type '%s'", typeName)).WithSuggestion("Built-in types (page, section, date) have fixed definitions. Use traits for additional metadata.")
	}
	trimmedTarget := strings.TrimSpace(req.Target)
	trimmedValues := splitCommaValues(req.Values)
	fieldType := ""
	err := editRuntimeSchema(rt, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		sch := doc.Schema()
		typeDef, exists := sch.Types[typeName]
		if !exists {
			return svcerr.New(codes.ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName)).WithSuggestion("Add the type first with 'rvn schema add type'")
		}
		if typeDef.Fields != nil {
			if _, exists := typeDef.Fields[fieldName]; exists {
				return svcerr.New(codes.ErrObjectExists, fmt.Sprintf("field '%s' already exists on type '%s'", fieldName, typeName))
			}
		}

		validation := ValidateFieldTypeSpec(req.FieldType, trimmedTarget, strings.Join(trimmedValues, ","), sch)
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
			return svcerr.New(codes.ErrInvalidInput, validation.Error).WithSuggestion(validation.Suggestion).WithDetails(details)
		}

		fieldType = validation.BaseType
		if fieldType == "" {
			fieldType = "string"
		}
		if validation.IsArray {
			fieldType += "[]"
		}

		typesNode, ok := doc.Root()["types"].(map[string]interface{})
		if !ok {
			return svcerr.New(codes.ErrSchemaInvalid, "types section not found")
		}
		typeNode := schemadoc.EnsureMap(typesNode, typeName)
		fieldsNode := schemadoc.EnsureMap(typeNode, "fields")

		newField := map[string]interface{}{
			"type": fieldType,
		}
		if req.Required {
			newField["required"] = true
		}
		if strings.TrimSpace(req.Default) != "" {
			newField["default"] = req.Default
		}
		if len(trimmedValues) > 0 {
			newField["values"] = trimmedValues
		}
		if trimmedTarget != "" {
			newField["target"] = trimmedTarget
		}
		if strings.TrimSpace(req.Description) != "" {
			newField["description"] = req.Description
		}
		fieldsNode[fieldName] = newField
		return nil
	})

	if err != nil {
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

func normalizeTraitTypeInput(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "string"
	}
	return trimmed
}

func normalizeTraitDefaultValue(traitType, raw string) (interface{}, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false
	}
	if traitType == "bool" || traitType == "boolean" {
		if trimmed == "true" {
			return true, true
		}
		if trimmed == "false" {
			return false, true
		}
	}
	return trimmed, true
}

func normalizeDirRoot(root string) string {
	root = strings.TrimSpace(root)
	root = strings.Trim(root, "/")
	if root == "" {
		return ""
	}
	return root + "/"
}
