package schemasvc

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
)

type UpdateTypeRequest struct {
	VaultPath   string
	TypeName    string
	DefaultPath string
	NameField   string
	Description string
	AddTrait    string
	RemoveTrait string
}

type UpdateTraitRequest struct {
	VaultPath string
	TraitName string
	TraitType string
	Values    string
	Default   string
}

type UpdateFieldRequest struct {
	VaultPath   string
	TypeName    string
	FieldName   string
	FieldType   string
	Required    string
	Default     string
	Values      string
	Target      string
	Description string
}

type UpdateResult struct {
	Name    string
	Type    string
	Field   string
	Changes []string
}

type RemoveTypeRequest struct {
	VaultPath   string
	TypeName    string
	Force       bool
	Interactive bool
}

type RemoveTraitRequest struct {
	VaultPath   string
	TraitName   string
	Force       bool
	Interactive bool
}

type RemoveFieldRequest struct {
	VaultPath string
	TypeName  string
	FieldName string
}

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type RemoveResult struct {
	Name     string
	Type     string
	Field    string
	Warnings []Warning
}

func UpdateType(req UpdateTypeRequest) (*UpdateResult, error) {
	typeName := strings.TrimSpace(req.TypeName)
	if typeName == "" {
		return nil, newError(ErrorInvalidInput, "type name cannot be empty", "", nil, nil)
	}
	if schema.IsBuiltinType(typeName) {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("'%s' is a built-in type and cannot be modified", typeName),
			"",
			nil,
			nil,
		)
	}

	sch, err := loadSchema(req.VaultPath, "Run 'rvn init' first")
	if err != nil {
		return nil, err
	}

	typeDef, exists := sch.Types[typeName]
	if !exists {
		return nil, newError(
			ErrorTypeNotFound,
			fmt.Sprintf("type '%s' not found", typeName),
			"Use 'rvn schema add type' to create it",
			nil,
			nil,
		)
	}

	schemaDoc, typesNode, err := readSchemaDocWithTypes(req.VaultPath)
	if err != nil {
		return nil, err
	}
	typeNode := ensureMapNode(typesNode, typeName)

	changes := make([]string, 0)

	if strings.TrimSpace(req.DefaultPath) != "" {
		typeNode["default_path"] = req.DefaultPath
		changes = append(changes, fmt.Sprintf("default_path=%s", req.DefaultPath))
	}

	if strings.TrimSpace(req.Description) != "" {
		if isClearSentinel(req.Description) {
			delete(typeNode, "description")
			changes = append(changes, "removed description")
		} else {
			typeNode["description"] = req.Description
			changes = append(changes, fmt.Sprintf("description=%s", req.Description))
		}
	}

	if strings.TrimSpace(req.NameField) != "" {
		if isClearSentinel(req.NameField) {
			delete(typeNode, "name_field")
			changes = append(changes, "removed name_field")
		} else {
			fieldExists := false
			if typeDef != nil && typeDef.Fields != nil {
				if fieldDef, ok := typeDef.Fields[req.NameField]; ok {
					fieldExists = true
					if fieldDef.Type != schema.FieldTypeString {
						return nil, newError(
							ErrorInvalidInput,
							fmt.Sprintf("name_field must reference a string field, '%s' is type '%s'", req.NameField, fieldDef.Type),
							"Choose a string field or create a new one",
							nil,
							nil,
						)
					}
				}
			}

			typeNode["name_field"] = req.NameField

			if !fieldExists {
				fieldsNode := ensureMapNode(typeNode, "fields")
				fieldsNode[req.NameField] = map[string]interface{}{
					"type":     "string",
					"required": true,
				}
				changes = append(changes, fmt.Sprintf("name_field=%s (auto-created as required string)", req.NameField))
			} else {
				changes = append(changes, fmt.Sprintf("name_field=%s", req.NameField))
			}
		}
	}

	if strings.TrimSpace(req.AddTrait) != "" {
		if _, exists := sch.Traits[req.AddTrait]; !exists {
			return nil, newError(
				ErrorTraitNotFound,
				fmt.Sprintf("trait '%s' not found", req.AddTrait),
				"Add it first with 'rvn schema add trait'",
				nil,
				nil,
			)
		}

		currentTraits := interfaceSlice(typeNode["traits"])
		if !containsString(currentTraits, req.AddTrait) {
			currentTraits = append(currentTraits, req.AddTrait)
			typeNode["traits"] = currentTraits
			changes = append(changes, fmt.Sprintf("added trait %s", req.AddTrait))
		}
	}

	if strings.TrimSpace(req.RemoveTrait) != "" {
		currentTraits := interfaceSlice(typeNode["traits"])
		if len(currentTraits) > 0 {
			filtered := make([]interface{}, 0, len(currentTraits))
			for _, traitValue := range currentTraits {
				if toStringSafe(traitValue) == req.RemoveTrait {
					continue
				}
				filtered = append(filtered, traitValue)
			}
			typeNode["traits"] = filtered
			changes = append(changes, fmt.Sprintf("removed trait %s", req.RemoveTrait))
		}
	}

	if len(changes) == 0 {
		return nil, newError(
			ErrorInvalidInput,
			"no changes specified",
			"Use flags like --default-path, --description, --name-field, --add-trait, --remove-trait",
			nil,
			nil,
		)
	}

	if err := writeSchemaDoc(req.VaultPath, schemaDoc); err != nil {
		return nil, err
	}

	return &UpdateResult{
		Name:    typeName,
		Changes: changes,
	}, nil
}

func UpdateTrait(req UpdateTraitRequest) (*UpdateResult, error) {
	traitName := strings.TrimSpace(req.TraitName)
	if traitName == "" {
		return nil, newError(ErrorInvalidInput, "trait name cannot be empty", "", nil, nil)
	}

	sch, err := loadSchema(req.VaultPath, "Run 'rvn init' first")
	if err != nil {
		return nil, err
	}
	if _, exists := sch.Traits[traitName]; !exists {
		return nil, newError(
			ErrorTraitNotFound,
			fmt.Sprintf("trait '%s' not found", traitName),
			"Use 'rvn schema add trait' to create it",
			nil,
			nil,
		)
	}

	schemaDoc, err := readSchemaDoc(req.VaultPath)
	if err != nil {
		return nil, err
	}
	traitsNode := ensureMapNode(schemaDoc, "traits")
	traitNode := ensureMapNode(traitsNode, traitName)

	changes := make([]string, 0)
	if strings.TrimSpace(req.TraitType) != "" {
		traitNode["type"] = req.TraitType
		changes = append(changes, fmt.Sprintf("type=%s", req.TraitType))
	}
	if strings.TrimSpace(req.Values) != "" {
		traitNode["values"] = strings.Split(req.Values, ",")
		changes = append(changes, fmt.Sprintf("values=%s", req.Values))
	}
	if strings.TrimSpace(req.Default) != "" {
		traitNode["default"] = req.Default
		changes = append(changes, fmt.Sprintf("default=%s", req.Default))
	}

	if len(changes) == 0 {
		return nil, newError(
			ErrorInvalidInput,
			"no changes specified",
			"Use flags like --type, --values, --default",
			nil,
			nil,
		)
	}

	if err := writeSchemaDoc(req.VaultPath, schemaDoc); err != nil {
		return nil, err
	}

	return &UpdateResult{
		Name:    traitName,
		Changes: changes,
	}, nil
}

func UpdateField(req UpdateFieldRequest) (*UpdateResult, error) {
	typeName := strings.TrimSpace(req.TypeName)
	fieldName := strings.TrimSpace(req.FieldName)
	if typeName == "" || fieldName == "" {
		return nil, newError(ErrorInvalidInput, "type and field names are required", "", nil, nil)
	}

	sch, err := loadSchema(req.VaultPath, "Run 'rvn init' first")
	if err != nil {
		return nil, err
	}

	typeDef, exists := sch.Types[typeName]
	if !exists {
		return nil, newError(ErrorTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "", nil, nil)
	}
	if schema.IsBuiltinType(typeName) {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("cannot modify fields on built-in type '%s'", typeName),
			"Built-in types (page, section, date) have fixed definitions.",
			nil,
			nil,
		)
	}
	if typeDef == nil || typeDef.Fields == nil {
		return nil, newError(
			ErrorFieldNotFound,
			fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName),
			"Use 'rvn schema add field' to create it",
			nil,
			nil,
		)
	}
	if _, ok := typeDef.Fields[fieldName]; !ok {
		return nil, newError(
			ErrorFieldNotFound,
			fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName),
			"Use 'rvn schema add field' to create it",
			nil,
			nil,
		)
	}

	if req.Required == "true" {
		db, err := index.Open(req.VaultPath)
		if err == nil {
			defer db.Close()
			objects, err := db.QueryObjects(typeName)
			if err == nil && len(objects) > 0 {
				missing := make([]string, 0)
				for _, obj := range objects {
					fields := obj.Fields
					if fields == nil {
						fields = map[string]interface{}{}
					}
					if _, hasField := fields[fieldName]; !hasField {
						missing = append(missing, obj.ID)
					}
				}
				if len(missing) > 0 {
					details := map[string]interface{}{
						"missing_field":    fieldName,
						"affected_count":   len(missing),
						"affected_objects": missing,
					}
					if len(missing) > 5 {
						details["affected_objects"] = append(missing[:5], "... and more")
					}
					return nil, newError(
						ErrorDataIntegrity,
						fmt.Sprintf("%d objects of type '%s' lack field '%s'", len(missing), typeName, fieldName),
						"Add the field to these files, then retry",
						details,
						nil,
					)
				}
			}
		}
	}

	schemaDoc, typesNode, err := readSchemaDocWithTypes(req.VaultPath)
	if err != nil {
		return nil, err
	}
	typeNode, ok := typesNode[typeName].(map[string]interface{})
	if !ok {
		return nil, newError(
			ErrorSchemaInvalid,
			fmt.Sprintf("type '%s' has invalid schema definition", typeName),
			"",
			nil,
			nil,
		)
	}

	fieldsNode, ok := typeNode["fields"].(map[string]interface{})
	if !ok {
		return nil, newError(
			ErrorSchemaInvalid,
			fmt.Sprintf("type '%s' has invalid fields definition", typeName),
			"",
			nil,
			nil,
		)
	}
	fieldNode := ensureMapNode(fieldsNode, fieldName)

	changes := make([]string, 0)
	if strings.TrimSpace(req.FieldType) != "" {
		fieldNode["type"] = req.FieldType
		changes = append(changes, fmt.Sprintf("type=%s", req.FieldType))
	}
	if strings.TrimSpace(req.Required) != "" {
		required := req.Required == "true"
		fieldNode["required"] = required
		changes = append(changes, fmt.Sprintf("required=%v", required))
	}
	if strings.TrimSpace(req.Default) != "" {
		fieldNode["default"] = req.Default
		changes = append(changes, fmt.Sprintf("default=%s", req.Default))
	}
	if strings.TrimSpace(req.Values) != "" {
		fieldNode["values"] = strings.Split(req.Values, ",")
		changes = append(changes, fmt.Sprintf("values=%s", req.Values))
	}
	if strings.TrimSpace(req.Target) != "" {
		fieldNode["target"] = req.Target
		changes = append(changes, fmt.Sprintf("target=%s", req.Target))
	}
	if strings.TrimSpace(req.Description) != "" {
		if isClearSentinel(req.Description) {
			delete(fieldNode, "description")
			changes = append(changes, "removed description")
		} else {
			fieldNode["description"] = req.Description
			changes = append(changes, fmt.Sprintf("description=%s", req.Description))
		}
	}

	if len(changes) == 0 {
		return nil, newError(
			ErrorInvalidInput,
			"no changes specified",
			"Use flags like --type, --required, --default, --description",
			nil,
			nil,
		)
	}

	if err := writeSchemaDoc(req.VaultPath, schemaDoc); err != nil {
		return nil, err
	}

	return &UpdateResult{
		Type:    typeName,
		Field:   fieldName,
		Changes: changes,
	}, nil
}

func RemoveType(req RemoveTypeRequest) (*RemoveResult, error) {
	typeName := strings.TrimSpace(req.TypeName)
	if typeName == "" {
		return nil, newError(ErrorInvalidInput, "type name cannot be empty", "", nil, nil)
	}
	if schema.IsBuiltinType(typeName) {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("'%s' is a built-in type and cannot be removed", typeName),
			"",
			nil,
			nil,
		)
	}

	sch, err := loadSchema(req.VaultPath, "Run 'rvn init' first")
	if err != nil {
		return nil, err
	}
	if _, exists := sch.Types[typeName]; !exists {
		return nil, newError(ErrorTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "", nil, nil)
	}

	warnings := make([]Warning, 0)
	if db, err := index.Open(req.VaultPath); err == nil {
		defer db.Close()
		if objects, err := db.QueryObjects(typeName); err == nil && len(objects) > 0 {
			warnings = append(warnings, Warning{
				Code:    "ORPHANED_FILES",
				Message: fmt.Sprintf("%d files of type '%s' will become 'page' type", len(objects), typeName),
			})
			if req.Interactive && !req.Force {
				details := map[string]interface{}{
					"type":           typeName,
					"affected_count": len(objects),
				}
				sample := make([]string, 0, 5)
				for i, obj := range objects {
					if i >= 5 {
						break
					}
					sample = append(sample, obj.FilePath)
				}
				if len(sample) > 0 {
					details["affected_files"] = sample
				}
				if len(objects) > len(sample) {
					details["remaining_count"] = len(objects) - len(sample)
				}
				return nil, newError(
					ErrorConfirmation,
					fmt.Sprintf("%d files of type '%s' will become 'page' type", len(objects), typeName),
					"Use --force to skip confirmation",
					details,
					nil,
				)
			}
		}
	}

	schemaDoc, typesNode, err := readSchemaDocWithTypes(req.VaultPath)
	if err != nil {
		return nil, err
	}
	delete(typesNode, typeName)
	if err := writeSchemaDoc(req.VaultPath, schemaDoc); err != nil {
		return nil, err
	}

	return &RemoveResult{
		Name:     typeName,
		Warnings: warnings,
	}, nil
}

func RemoveTrait(req RemoveTraitRequest) (*RemoveResult, error) {
	traitName := strings.TrimSpace(req.TraitName)
	if traitName == "" {
		return nil, newError(ErrorInvalidInput, "trait name cannot be empty", "", nil, nil)
	}

	sch, err := loadSchema(req.VaultPath, "Run 'rvn init' first")
	if err != nil {
		return nil, err
	}
	if _, exists := sch.Traits[traitName]; !exists {
		return nil, newError(ErrorTraitNotFound, fmt.Sprintf("trait '%s' not found", traitName), "", nil, nil)
	}

	warnings := make([]Warning, 0)
	if db, err := index.Open(req.VaultPath); err == nil {
		defer db.Close()
		if instances, err := db.QueryTraits(traitName, nil); err == nil && len(instances) > 0 {
			warnings = append(warnings, Warning{
				Code:    "ORPHANED_TRAITS",
				Message: fmt.Sprintf("%d instances of @%s will remain in files (no longer indexed)", len(instances), traitName),
			})
			if req.Interactive && !req.Force {
				return nil, newError(
					ErrorConfirmation,
					fmt.Sprintf("%d instances of @%s will remain in files (no longer indexed)", len(instances), traitName),
					"Use --force to skip confirmation",
					map[string]interface{}{
						"trait":          traitName,
						"affected_count": len(instances),
					},
					nil,
				)
			}
		}
	}

	schemaDoc, err := readSchemaDoc(req.VaultPath)
	if err != nil {
		return nil, err
	}
	traitsNode := ensureMapNode(schemaDoc, "traits")
	delete(traitsNode, traitName)

	if err := writeSchemaDoc(req.VaultPath, schemaDoc); err != nil {
		return nil, err
	}

	return &RemoveResult{
		Name:     traitName,
		Warnings: warnings,
	}, nil
}

func RemoveField(req RemoveFieldRequest) (*RemoveResult, error) {
	typeName := strings.TrimSpace(req.TypeName)
	fieldName := strings.TrimSpace(req.FieldName)
	if typeName == "" || fieldName == "" {
		return nil, newError(ErrorInvalidInput, "type and field names are required", "", nil, nil)
	}

	sch, err := loadSchema(req.VaultPath, "Run 'rvn init' first")
	if err != nil {
		return nil, err
	}

	typeDef, exists := sch.Types[typeName]
	if !exists {
		return nil, newError(ErrorTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "", nil, nil)
	}
	if schema.IsBuiltinType(typeName) {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("cannot remove fields from built-in type '%s'", typeName),
			"Built-in types (page, section, date) have fixed definitions.",
			nil,
			nil,
		)
	}
	if typeDef == nil || typeDef.Fields == nil {
		return nil, newError(ErrorFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName), "", nil, nil)
	}

	fieldDef, exists := typeDef.Fields[fieldName]
	if !exists {
		return nil, newError(ErrorFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName), "", nil, nil)
	}

	if fieldDef != nil && fieldDef.Required {
		if db, err := index.Open(req.VaultPath); err == nil {
			defer db.Close()
			if objects, err := db.QueryObjects(typeName); err == nil && len(objects) > 0 {
				return nil, newError(
					ErrorDataIntegrity,
					fmt.Sprintf("cannot remove required field '%s': %d objects have this field", fieldName, len(objects)),
					"First make the field optional with 'rvn schema update field', then remove it",
					map[string]interface{}{
						"field":          fieldName,
						"type":           typeName,
						"affected_count": len(objects),
					},
					nil,
				)
			}
		}
	}

	schemaDoc, typesNode, err := readSchemaDocWithTypes(req.VaultPath)
	if err != nil {
		return nil, err
	}
	typeNode, ok := typesNode[typeName].(map[string]interface{})
	if !ok {
		return nil, newError(
			ErrorSchemaInvalid,
			fmt.Sprintf("type '%s' has invalid schema definition", typeName),
			"",
			nil,
			nil,
		)
	}
	fieldsNode, ok := typeNode["fields"].(map[string]interface{})
	if !ok {
		return nil, newError(
			ErrorSchemaInvalid,
			fmt.Sprintf("type '%s' has invalid fields definition", typeName),
			"",
			nil,
			nil,
		)
	}

	delete(fieldsNode, fieldName)
	if len(fieldsNode) == 0 {
		delete(typeNode, "fields")
	}

	if err := writeSchemaDoc(req.VaultPath, schemaDoc); err != nil {
		return nil, err
	}

	return &RemoveResult{
		Type:  typeName,
		Field: fieldName,
	}, nil
}

func isClearSentinel(value string) bool {
	switch strings.TrimSpace(value) {
	case "-", "none", "\"\"":
		return true
	default:
		return false
	}
}

func interfaceSlice(raw interface{}) []interface{} {
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	return items
}

func containsString(values []interface{}, target string) bool {
	for _, value := range values {
		if toStringSafe(value) == target {
			return true
		}
	}
	return false
}

func toStringSafe(value interface{}) string {
	if value == nil {
		return ""
	}
	if stringValue, ok := value.(string); ok {
		return stringValue
	}
	return fmt.Sprintf("%v", value)
}
