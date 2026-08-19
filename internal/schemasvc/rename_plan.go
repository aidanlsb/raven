package schemasvc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/svcerr"
)

type TypeRenameChange struct {
	FilePath    string `json:"file_path"`
	ChangeType  string `json:"change_type"`
	Description string `json:"description"`
	Line        int    `json:"line,omitempty"`
}

type FieldRenameChange struct {
	FilePath    string `json:"file_path"`
	ChangeType  string `json:"change_type"`
	Description string `json:"description"`
	Line        int    `json:"line,omitempty"`
}

// TypeRenamePlanRequest contains only the schema-document inputs needed to
// transform a type definition. Vault scanning and file migration are handled
// by schemamigrate.
type TypeRenamePlanRequest struct {
	SchemaDoc      *schemadoc.Document
	OldName        string
	NewName        string
	Description    string
	OldDefaultPath string
}

// TypeRenamePlan stages the schema.yaml variants and schema-level change
// records for a type rename. It intentionally contains no vault file moves or
// content rewrites.
type TypeRenamePlan struct {
	SchemaYAML                []byte
	SchemaYAMLWithDefaultPath []byte
	Changes                   []TypeRenameChange
	OptionalChanges           []TypeRenameChange
	CoreSchemaMutations       int
	DefaultPathMutation       bool
	DefaultPathOld            string
	DefaultPathNew            string
}

// FieldRenamePlanRequest contains only the schema-document inputs needed to
// transform a field definition.
type FieldRenamePlanRequest struct {
	SchemaDoc *schemadoc.Document
	TypeName  string
	OldField  string
	NewField  string
}

// FieldRenamePlan stages the schema.yaml update for a field rename. TemplateSpec
// tells the migration layer which legacy template file may need a content
// rewrite.
type FieldRenamePlan struct {
	SchemaYAML   []byte
	Changes      []FieldRenameChange
	TemplateSpec string
}

// BuildTypeRenamePlan transforms schema.yaml without reading or writing vault
// files. The migration layer combines this plan with frontmatter, reference,
// and optional directory-move plans.
func BuildTypeRenamePlan(req TypeRenamePlanRequest) (*TypeRenamePlan, error) {
	description := strings.TrimSpace(req.Description)
	plan := &TypeRenamePlan{
		Changes:         make([]TypeRenameChange, 0),
		OptionalChanges: make([]TypeRenameChange, 0),
	}

	if req.SchemaDoc == nil {
		return nil, svcerr.New(codes.ErrInternal, "schema document is required")
	}
	typesNode, ok := req.SchemaDoc.Root()["types"].(map[string]interface{})
	if !ok {
		return nil, svcerr.New(codes.ErrSchemaInvalid, "types section not found")
	}

	plan.Changes = append(plan.Changes, TypeRenameChange{
		FilePath:    "schema.yaml",
		ChangeType:  "schema_type",
		Description: fmt.Sprintf("rename type '%s' to '%s'", req.OldName, req.NewName),
	})
	if typeDef, exists := typesNode[req.OldName]; exists {
		typesNode[req.NewName] = typeDef
		delete(typesNode, req.OldName)
		plan.CoreSchemaMutations++
	}

	if description != "" {
		if typeDefMap, ok := typesNode[req.NewName].(map[string]interface{}); ok {
			if isClearSentinel(req.Description) {
				if _, hadDescription := typeDefMap["description"]; hadDescription {
					delete(typeDefMap, "description")
					plan.Changes = append(plan.Changes, TypeRenameChange{
						FilePath:    "schema.yaml",
						ChangeType:  "schema_description",
						Description: fmt.Sprintf("remove description from type '%s'", req.NewName),
					})
					plan.CoreSchemaMutations++
				}
			} else if current, _ := typeDefMap["description"].(string); current != req.Description {
				typeDefMap["description"] = req.Description
				plan.Changes = append(plan.Changes, TypeRenameChange{
					FilePath:    "schema.yaml",
					ChangeType:  "schema_description",
					Description: fmt.Sprintf("update description for type '%s'", req.NewName),
				})
				plan.CoreSchemaMutations++
			}
		}
	}

	for _, typeName := range sortedRenameKeys(typesNode) {
		typeMap, ok := typesNode[typeName].(map[string]interface{})
		if !ok {
			continue
		}
		fields, ok := typeMap["fields"].(map[string]interface{})
		if !ok {
			continue
		}
		for _, fieldName := range sortedRenameKeys(fields) {
			fieldMap, ok := fields[fieldName].(map[string]interface{})
			if !ok {
				continue
			}
			if target, ok := fieldMap["target"].(string); ok && target == req.OldName {
				fieldMap["target"] = req.NewName
				plan.Changes = append(plan.Changes, TypeRenameChange{
					FilePath:    "schema.yaml",
					ChangeType:  "schema_ref_target",
					Description: fmt.Sprintf("update field '%s.%s' target from '%s' to '%s'", typeName, fieldName, req.OldName, req.NewName),
				})
				plan.CoreSchemaMutations++
			}
		}
	}

	coreSchema, err := req.SchemaDoc.Marshal()
	if err != nil {
		return nil, MapSchemaDocError(err, "", codes.ErrSchemaNotFound)
	}
	plan.SchemaYAML = coreSchema

	if suggestedPath, ok := suggestRenamedDefaultPath(req.OldDefaultPath, req.OldName, req.NewName); ok {
		plan.DefaultPathOld = paths.NormalizeDirRoot(req.OldDefaultPath)
		plan.DefaultPathNew = suggestedPath
		plan.OptionalChanges = append(plan.OptionalChanges, TypeRenameChange{
			FilePath:    "schema.yaml",
			ChangeType:  "schema_default_path",
			Description: fmt.Sprintf("update default_path '%s' → '%s' for type '%s'", plan.DefaultPathOld, plan.DefaultPathNew, req.NewName),
		})

		if typeDefMap, ok := typesNode[req.NewName].(map[string]interface{}); ok {
			typeDefMap["default_path"] = plan.DefaultPathNew
			plan.DefaultPathMutation = true
		}
		withDefaultPath, err := req.SchemaDoc.Marshal()
		if err != nil {
			return nil, MapSchemaDocError(err, "", codes.ErrSchemaNotFound)
		}
		plan.SchemaYAMLWithDefaultPath = withDefaultPath
	}

	return plan, nil
}

// BuildFieldRenamePlan transforms schema.yaml without touching templates,
// saved queries, or Markdown objects.
func BuildFieldRenamePlan(req FieldRenamePlanRequest) (*FieldRenamePlan, error) {
	plan := &FieldRenamePlan{Changes: make([]FieldRenameChange, 0)}

	if req.SchemaDoc == nil {
		return nil, svcerr.New(codes.ErrInternal, "schema document is required")
	}

	types, ok := req.SchemaDoc.Root()["types"].(map[string]interface{})
	if !ok {
		return nil, svcerr.New(codes.ErrSchemaInvalid, "types section not found")
	}
	typeNodeAny, ok := types[req.TypeName]
	if !ok {
		return nil, svcerr.New(codes.ErrTypeNotFound, fmt.Sprintf("type '%s' not found", req.TypeName))
	}
	typeNode, ok := typeNodeAny.(map[string]interface{})
	if !ok {
		return nil, svcerr.New(codes.ErrSchemaInvalid, fmt.Sprintf("type '%s' has invalid definition", req.TypeName))
	}
	fieldsAny, ok := typeNode["fields"]
	if !ok {
		return nil, svcerr.New(codes.ErrFieldNotFound, fmt.Sprintf("type '%s' has no fields", req.TypeName))
	}
	fields, ok := fieldsAny.(map[string]interface{})
	if !ok {
		return nil, svcerr.New(codes.ErrSchemaInvalid, fmt.Sprintf("type '%s' fields are invalid", req.TypeName))
	}

	_, hasOld := fields[req.OldField]
	_, hasNew := fields[req.NewField]
	if hasOld && hasNew {
		return nil, svcerr.New(codes.ErrObjectExists, fmt.Sprintf("type '%s' already has both '%s' and '%s' fields", req.TypeName, req.OldField, req.NewField)).WithSuggestion("Choose a different new field name or remove one field first")
	}
	if hasNew {
		return nil, svcerr.New(codes.ErrObjectExists, fmt.Sprintf("field '%s' already exists on type '%s'", req.NewField, req.TypeName)).WithSuggestion("Choose a different new field name")
	}
	if !hasOld {
		return nil, svcerr.New(codes.ErrFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", req.OldField, req.TypeName))
	}

	fields[req.NewField] = fields[req.OldField]
	delete(fields, req.OldField)
	plan.Changes = append(plan.Changes, FieldRenameChange{
		FilePath:    "schema.yaml",
		ChangeType:  "schema_field",
		Description: fmt.Sprintf("rename field '%s' → '%s' on type '%s'", req.OldField, req.NewField, req.TypeName),
	})

	if nameField, ok := typeNode["name_field"].(string); ok && nameField == req.OldField {
		typeNode["name_field"] = req.NewField
		plan.Changes = append(plan.Changes, FieldRenameChange{
			FilePath:    "schema.yaml",
			ChangeType:  "schema_name_field",
			Description: fmt.Sprintf("update name_field: %s → %s", req.OldField, req.NewField),
		})
	}

	plan.TemplateSpec, _ = typeNode["template"].(string)
	schemaOut, err := req.SchemaDoc.Marshal()
	if err != nil {
		return nil, MapSchemaDocError(err, "", codes.ErrSchemaNotFound)
	}
	plan.SchemaYAML = schemaOut
	return plan, nil
}

func suggestRenamedDefaultPath(oldDefaultPath, oldName, newName string) (string, bool) {
	normalized := paths.NormalizeDirRoot(oldDefaultPath)
	if normalized == "" {
		return "", false
	}
	trimmed := strings.TrimSuffix(normalized, "/")
	if trimmed == "" {
		return "", false
	}
	lastSlash := strings.LastIndex(trimmed, "/")
	parent := ""
	base := trimmed
	if lastSlash >= 0 {
		parent = trimmed[:lastSlash]
		base = trimmed[lastSlash+1:]
	}

	newBase := ""
	switch base {
	case oldName:
		newBase = newName
	case oldName + "s":
		newBase = newName + "s"
	default:
		return "", false
	}

	next := newBase
	if parent != "" {
		next = parent + "/" + newBase
	}
	next = paths.NormalizeDirRoot(next)
	if next == normalized {
		return "", false
	}
	return next, true
}

func sortedRenameKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
