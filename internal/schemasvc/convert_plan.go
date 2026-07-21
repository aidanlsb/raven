package schemasvc

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemadoc"
)

// ValueConvertChange describes one logical schema or vault-data mutation.
// Vault file changes are appended by schemamigrate to the schema-only changes
// produced here.
type ValueConvertChange struct {
	FilePath    string `json:"file_path"`
	ChangeType  string `json:"change_type"`
	Description string `json:"description"`
	Line        int    `json:"line,omitempty"`
}

type ConvertTraitPlanRequest struct {
	SchemaDoc  *schemadoc.Document
	TraitName  string
	TargetType schema.FieldType
	SetType    bool
	NewValues  []string
	NewDefault interface{}
	HasDefault bool
	SourceType schema.FieldType
}

type ConvertFieldPlanRequest struct {
	SchemaDoc  *schemadoc.Document
	TypeName   string
	FieldName  string
	TargetType schema.FieldType
	SetType    bool
	NewValues  []string
	NewDefault interface{}
	HasDefault bool
	SourceType schema.FieldType
}

type ValueConvertPlan struct {
	SchemaYAML      []byte
	Changes         []ValueConvertChange
	SchemaMutations int
}

// BuildTraitConvertPlan transforms one trait definition without reading or
// writing Markdown files.
func BuildTraitConvertPlan(req ConvertTraitPlanRequest) (*ValueConvertPlan, error) {
	if req.SchemaDoc == nil {
		return nil, newError(ErrorInternal, "schema document is required", "", nil, nil)
	}

	traits, ok := req.SchemaDoc.Root()["traits"].(map[string]interface{})
	if !ok {
		return nil, newError(ErrorSchemaInvalid, "traits section not found", "", nil, nil)
	}
	rawTrait, ok := traits[req.TraitName]
	if !ok {
		return nil, newError(ErrorTraitNotFound, fmt.Sprintf("trait '%s' not found", req.TraitName), "", nil, nil)
	}
	traitNode, ok := rawTrait.(map[string]interface{})
	if !ok {
		return nil, newError(ErrorSchemaInvalid, fmt.Sprintf("trait '%s' has invalid definition", req.TraitName), "", nil, nil)
	}

	plan := &ValueConvertPlan{Changes: make([]ValueConvertChange, 0)}
	applyDefinitionConversion(
		plan,
		traitNode,
		fmt.Sprintf("trait '%s'", req.TraitName),
		req.SourceType,
		req.TargetType,
		req.SetType,
		req.NewValues,
		req.NewDefault,
		req.HasDefault,
	)

	output, err := req.SchemaDoc.Marshal()
	if err != nil {
		return nil, MapSchemaDocError(err, "", ErrorSchemaInvalid)
	}
	plan.SchemaYAML = output
	return plan, nil
}

// BuildFieldConvertPlan transforms one field definition without reading or
// writing Markdown files.
func BuildFieldConvertPlan(req ConvertFieldPlanRequest) (*ValueConvertPlan, error) {
	if req.SchemaDoc == nil {
		return nil, newError(ErrorInternal, "schema document is required", "", nil, nil)
	}

	types, ok := req.SchemaDoc.Root()["types"].(map[string]interface{})
	if !ok {
		return nil, newError(ErrorSchemaInvalid, "types section not found", "", nil, nil)
	}
	rawType, ok := types[req.TypeName]
	if !ok {
		return nil, newError(ErrorTypeNotFound, fmt.Sprintf("type '%s' not found", req.TypeName), "", nil, nil)
	}
	typeNode, ok := rawType.(map[string]interface{})
	if !ok {
		return nil, newError(ErrorSchemaInvalid, fmt.Sprintf("type '%s' has invalid definition", req.TypeName), "", nil, nil)
	}
	fields, ok := typeNode["fields"].(map[string]interface{})
	if !ok {
		return nil, newError(ErrorFieldNotFound, fmt.Sprintf("type '%s' has no fields", req.TypeName), "", nil, nil)
	}
	rawField, ok := fields[req.FieldName]
	if !ok {
		return nil, newError(ErrorFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", req.FieldName, req.TypeName), "", nil, nil)
	}
	fieldNode, ok := rawField.(map[string]interface{})
	if !ok {
		return nil, newError(ErrorSchemaInvalid, fmt.Sprintf("field '%s.%s' has invalid definition", req.TypeName, req.FieldName), "", nil, nil)
	}

	plan := &ValueConvertPlan{Changes: make([]ValueConvertChange, 0)}
	applyDefinitionConversion(
		plan,
		fieldNode,
		fmt.Sprintf("field '%s.%s'", req.TypeName, req.FieldName),
		req.SourceType,
		req.TargetType,
		req.SetType,
		req.NewValues,
		req.NewDefault,
		req.HasDefault,
	)

	if !isRefType(req.TargetType) {
		if _, exists := fieldNode["target"]; exists {
			delete(fieldNode, "target")
			plan.addSchemaChange("schema_target", fmt.Sprintf("remove reference target from field '%s.%s'", req.TypeName, req.FieldName))
		}
	}

	output, err := req.SchemaDoc.Marshal()
	if err != nil {
		return nil, MapSchemaDocError(err, "", ErrorSchemaInvalid)
	}
	plan.SchemaYAML = output
	return plan, nil
}

func applyDefinitionConversion(
	plan *ValueConvertPlan,
	node map[string]interface{},
	label string,
	sourceType, targetType schema.FieldType,
	setType bool,
	newValues []string,
	newDefault interface{},
	hasDefault bool,
) {
	if setType {
		current, _ := node["type"].(string)
		if current != string(targetType) {
			node["type"] = string(targetType)
			plan.addSchemaChange("schema_type", fmt.Sprintf("change %s type from '%s' to '%s'", label, sourceType, targetType))
		}
	}

	if isEnumType(targetType) {
		if !stringSliceEqual(node["values"], newValues) {
			node["values"] = append([]string(nil), newValues...)
			plan.addSchemaChange("schema_values", fmt.Sprintf("replace allowed values for %s with [%s]", label, strings.Join(newValues, ", ")))
		}
	} else if _, exists := node["values"]; exists {
		delete(node, "values")
		plan.addSchemaChange("schema_values", fmt.Sprintf("remove allowed values from %s", label))
	}

	if hasDefault && !reflect.DeepEqual(node["default"], newDefault) {
		node["default"] = newDefault
		plan.addSchemaChange("schema_default", fmt.Sprintf("convert default value for %s", label))
	}
}

func (p *ValueConvertPlan) addSchemaChange(changeType, description string) {
	p.Changes = append(p.Changes, ValueConvertChange{
		FilePath:    "schema.yaml",
		ChangeType:  changeType,
		Description: description,
	})
	p.SchemaMutations++
}

func isEnumType(fieldType schema.FieldType) bool {
	return fieldType == schema.FieldTypeEnum || fieldType == schema.FieldTypeEnumArray
}

func isRefType(fieldType schema.FieldType) bool {
	return fieldType == schema.FieldTypeRef || fieldType == schema.FieldTypeRefArray
}

func stringSliceEqual(raw interface{}, expected []string) bool {
	switch values := raw.(type) {
	case []string:
		return reflect.DeepEqual(values, expected)
	case []interface{}:
		if len(values) != len(expected) {
			return false
		}
		for i, value := range values {
			if value != expected[i] {
				return false
			}
		}
		return true
	default:
		return len(expected) == 0 && raw == nil
	}
}
