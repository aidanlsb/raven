package schemasvc

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
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
	Code    codes.WarningCode `json:"code"`
	Message string            `json:"message"`
}

type RemoveResult struct {
	Name     string
	Type     string
	Field    string
	Warnings []Warning
}

func UpdateType(rt *vaultruntime.Runtime, req UpdateTypeRequest) (*UpdateResult, error) {
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

	changes := make([]string, 0)
	err := editRuntimeSchema(rt, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		sch := doc.Schema()
		typeDef, exists := sch.Types[typeName]
		if !exists {
			return newError(
				ErrorTypeNotFound,
				fmt.Sprintf("type '%s' not found", typeName),
				"Use 'rvn schema add type' to create it",
				nil,
				nil,
			)
		}

		typesNode, ok := doc.Root()["types"].(map[string]interface{})
		if !ok {
			return newError(ErrorSchemaInvalid, "types section not found", "", nil, nil)
		}
		typeNode := schemadoc.EnsureMap(typesNode, typeName)

		if strings.TrimSpace(req.DefaultPath) != "" {
			defaultPath := normalizeDirRoot(req.DefaultPath)
			typeNode["default_path"] = defaultPath
			changes = append(changes, fmt.Sprintf("default_path=%s", defaultPath))
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
							return newError(
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
					fieldsNode := schemadoc.EnsureMap(typeNode, "fields")
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
				return newError(
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
			return newError(
				ErrorInvalidInput,
				"no changes specified",
				"Use flags like --default-path, --description, --name-field, --add-trait, --remove-trait",
				nil,
				nil,
			)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &UpdateResult{
		Name:    typeName,
		Changes: changes,
	}, nil
}

func UpdateTrait(rt *vaultruntime.Runtime, req UpdateTraitRequest) (*UpdateResult, error) {
	traitName := strings.TrimSpace(req.TraitName)
	if traitName == "" {
		return nil, newError(ErrorInvalidInput, "trait name cannot be empty", "", nil, nil)
	}
	if err := rejectUpdateRemap("trait", traitName, req.TraitType, req.Values); err != nil {
		return nil, err
	}

	changes := make([]string, 0)
	err := editRuntimeSchema(rt, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		traitDef, exists := doc.Schema().Traits[traitName]
		if !exists {
			return newError(
				ErrorTraitNotFound,
				fmt.Sprintf("trait '%s' not found", traitName),
				"Use 'rvn schema add trait' to create it",
				nil,
				nil,
			)
		}

		traitsNode := schemadoc.EnsureMap(doc.Root(), "traits")
		traitNode := schemadoc.EnsureMap(traitsNode, traitName)

		if strings.TrimSpace(req.Default) != "" {
			effectiveType := currentTraitType(traitDef)
			if normalizedDefault, ok := normalizeTraitDefaultValue(effectiveType, req.Default); ok {
				traitNode["default"] = normalizedDefault
				changes = append(changes, fmt.Sprintf("default=%v", normalizedDefault))
			}
		}

		if len(changes) == 0 {
			return newError(
				ErrorInvalidInput,
				"no changes specified",
				"Use --default; use 'rvn schema convert trait' for type or value changes",
				nil,
				nil,
			)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &UpdateResult{
		Name:    traitName,
		Changes: changes,
	}, nil
}

func UpdateField(rt *vaultruntime.Runtime, req UpdateFieldRequest) (*UpdateResult, error) {
	typeName := strings.TrimSpace(req.TypeName)
	fieldName := strings.TrimSpace(req.FieldName)
	if typeName == "" || fieldName == "" {
		return nil, newError(ErrorInvalidInput, "type and field names are required", "", nil, nil)
	}
	if err := rejectUpdateRemap("field", typeName+" "+fieldName, req.FieldType, req.Values); err != nil {
		return nil, err
	}

	changes := make([]string, 0)
	err := editRuntimeSchema(rt, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		sch := doc.Schema()
		typeDef, exists := sch.Types[typeName]
		if !exists {
			return newError(ErrorTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "", nil, nil)
		}
		if schema.IsBuiltinType(typeName) {
			return newError(
				ErrorInvalidInput,
				fmt.Sprintf("cannot modify fields on built-in type '%s'", typeName),
				"Built-in types (page, section, date) have fixed definitions.",
				nil,
				nil,
			)
		}
		if typeDef == nil || typeDef.Fields == nil {
			return newError(
				ErrorFieldNotFound,
				fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName),
				"Use 'rvn schema add field' to create it",
				nil,
				nil,
			)
		}
		currentFieldDef, ok := typeDef.Fields[fieldName]
		if !ok {
			return newError(
				ErrorFieldNotFound,
				fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName),
				"Use 'rvn schema add field' to create it",
				nil,
				nil,
			)
		}

		effectiveFieldType := currentFieldType(currentFieldDef)
		effectiveTarget := ""
		if currentFieldDef != nil {
			effectiveTarget = strings.TrimSpace(currentFieldDef.Target)
		}
		if strings.TrimSpace(req.Target) != "" {
			effectiveTarget = strings.TrimSpace(req.Target)
		}
		effectiveValues := ""
		if currentFieldDef != nil {
			effectiveValues = strings.Join(currentFieldDef.Values, ",")
		}
		validation := ValidateFieldTypeSpec(effectiveFieldType, effectiveTarget, effectiveValues, sch)
		if !validation.Valid {
			details := map[string]interface{}{
				"field_type":  effectiveFieldType,
				"valid_types": validation.ValidTypes,
			}
			if len(validation.Examples) > 0 {
				details["examples"] = validation.Examples
			}
			if validation.TargetHint != "" {
				details["target_hint"] = validation.TargetHint
			}
			return newError(ErrorInvalidInput, validation.Error, validation.Suggestion, details, nil)
		}

		if req.Required == "true" {
			openErr := rt.OpenDB()
			if errors.Is(openErr, index.ErrIndexRebuildRequired) {
				return indexRebuildRequiredError(openErr)
			}
			if openErr == nil {
				db := rt.DB
				objects, objectsErr := objectsByType(db, typeName)
				if objectsErr == nil && len(objects) > 0 {
					missing := make([]string, 0)
					for _, obj := range objects {
						fields := obj.Fields
						if fields == nil {
							fields = map[string]fieldvalue.FieldValue{}
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
						return newError(
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

		typesNode, ok := doc.Root()["types"].(map[string]interface{})
		if !ok {
			return newError(ErrorSchemaInvalid, "types section not found", "", nil, nil)
		}
		typeNode, ok := typesNode[typeName].(map[string]interface{})
		if !ok {
			return newError(
				ErrorSchemaInvalid,
				fmt.Sprintf("type '%s' has invalid schema definition", typeName),
				"",
				nil,
				nil,
			)
		}

		fieldsNode, ok := typeNode["fields"].(map[string]interface{})
		if !ok {
			return newError(
				ErrorSchemaInvalid,
				fmt.Sprintf("type '%s' has invalid fields definition", typeName),
				"",
				nil,
				nil,
			)
		}
		fieldNode := schemadoc.EnsureMap(fieldsNode, fieldName)

		if strings.TrimSpace(req.Required) != "" {
			required := req.Required == "true"
			fieldNode["required"] = required
			changes = append(changes, fmt.Sprintf("required=%v", required))
		}
		if strings.TrimSpace(req.Default) != "" {
			fieldNode["default"] = req.Default
			changes = append(changes, fmt.Sprintf("default=%s", req.Default))
		}
		if strings.TrimSpace(req.Target) != "" {
			fieldNode["target"] = strings.TrimSpace(req.Target)
			changes = append(changes, fmt.Sprintf("target=%s", strings.TrimSpace(req.Target)))
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
			return newError(
				ErrorInvalidInput,
				"no changes specified",
				"Use flags like --required, --default, --target, --description; use 'rvn schema convert field' for type or value changes",
				nil,
				nil,
			)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &UpdateResult{
		Type:    typeName,
		Field:   fieldName,
		Changes: changes,
	}, nil
}

func rejectUpdateRemap(kind, target, requestedType, values string) *Error {
	flags := make([]string, 0, 2)
	if strings.TrimSpace(requestedType) != "" {
		flags = append(flags, "--type")
	}
	if strings.TrimSpace(values) != "" {
		flags = append(flags, "--values")
	}
	if len(flags) == 0 {
		return nil
	}

	return newError(
		ErrorInvalidInput,
		fmt.Sprintf("schema update %s does not support %s", kind, strings.Join(flags, " or ")),
		fmt.Sprintf(
			"Use 'rvn schema convert %s %s [--type <target-type>] --map-json <mapping>' to migrate schema and live values",
			kind,
			target,
		),
		map[string]interface{}{"unsupported_flags": flags},
		nil,
	)
}

func currentTraitType(def *schema.TraitDefinition) string {
	if def == nil {
		return "string"
	}
	if strings.TrimSpace(string(def.Type)) == "" {
		return "boolean"
	}
	return strings.TrimSpace(string(def.Type))
}

func currentFieldType(def *schema.FieldDefinition) string {
	if def == nil {
		return "string"
	}
	return strings.TrimSpace(string(def.Type))
}

func RemoveType(rt *vaultruntime.Runtime, req RemoveTypeRequest) (*RemoveResult, error) {
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

	warnings := make([]Warning, 0)
	err := editRuntimeSchema(rt, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		if _, exists := doc.Schema().Types[typeName]; !exists {
			return newError(ErrorTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "", nil, nil)
		}

		dbErr := rt.OpenDB()
		if errors.Is(dbErr, index.ErrIndexRebuildRequired) {
			return indexRebuildRequiredError(dbErr)
		}
		if dbErr == nil {
			db := rt.DB
			if objects, objectsErr := objectsByType(db, typeName); objectsErr == nil && len(objects) > 0 {
				warnings = append(warnings, Warning{
					Code:    codes.WarnOrphanedFiles,
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
					return newError(
						ErrorConfirmation,
						fmt.Sprintf("%d files of type '%s' will become 'page' type", len(objects), typeName),
						"Use --force to skip confirmation",
						details,
						nil,
					)
				}
			}
		}

		typesNode, ok := doc.Root()["types"].(map[string]interface{})
		if !ok {
			return newError(ErrorSchemaInvalid, "types section not found", "", nil, nil)
		}
		delete(typesNode, typeName)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &RemoveResult{
		Name:     typeName,
		Warnings: warnings,
	}, nil
}

func RemoveTrait(rt *vaultruntime.Runtime, req RemoveTraitRequest) (*RemoveResult, error) {
	traitName := strings.TrimSpace(req.TraitName)
	if traitName == "" {
		return nil, newError(ErrorInvalidInput, "trait name cannot be empty", "", nil, nil)
	}

	warnings := make([]Warning, 0)
	err := editRuntimeSchema(rt, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		if _, exists := doc.Schema().Traits[traitName]; !exists {
			return newError(ErrorTraitNotFound, fmt.Sprintf("trait '%s' not found", traitName), "", nil, nil)
		}

		dbErr := rt.OpenDB()
		if errors.Is(dbErr, index.ErrIndexRebuildRequired) {
			return indexRebuildRequiredError(dbErr)
		}
		if dbErr == nil {
			db := rt.DB
			if instances, instancesErr := traitsByType(db, traitName); instancesErr == nil && len(instances) > 0 {
				warnings = append(warnings, Warning{
					Code:    codes.WarnOrphanedTraits,
					Message: fmt.Sprintf("%d instances of @%s will remain in files (no longer indexed)", len(instances), traitName),
				})
				if req.Interactive && !req.Force {
					return newError(
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

		traitsNode := schemadoc.EnsureMap(doc.Root(), "traits")
		delete(traitsNode, traitName)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &RemoveResult{
		Name:     traitName,
		Warnings: warnings,
	}, nil
}

func RemoveField(rt *vaultruntime.Runtime, req RemoveFieldRequest) (*RemoveResult, error) {
	typeName := strings.TrimSpace(req.TypeName)
	fieldName := strings.TrimSpace(req.FieldName)
	if typeName == "" || fieldName == "" {
		return nil, newError(ErrorInvalidInput, "type and field names are required", "", nil, nil)
	}

	err := editRuntimeSchema(rt, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		typeDef, exists := doc.Schema().Types[typeName]
		if !exists {
			return newError(ErrorTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "", nil, nil)
		}
		if schema.IsBuiltinType(typeName) {
			return newError(
				ErrorInvalidInput,
				fmt.Sprintf("cannot remove fields from built-in type '%s'", typeName),
				"Built-in types (page, section, date) have fixed definitions.",
				nil,
				nil,
			)
		}
		if typeDef == nil || typeDef.Fields == nil {
			return newError(ErrorFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName), "", nil, nil)
		}

		fieldDef, exists := typeDef.Fields[fieldName]
		if !exists {
			return newError(ErrorFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName), "", nil, nil)
		}

		if fieldDef != nil && fieldDef.Required {
			dbErr := rt.OpenDB()
			if errors.Is(dbErr, index.ErrIndexRebuildRequired) {
				return indexRebuildRequiredError(dbErr)
			}
			if dbErr == nil {
				db := rt.DB
				if objects, objectsErr := objectsByType(db, typeName); objectsErr == nil && len(objects) > 0 {
					return newError(
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

		typesNode, ok := doc.Root()["types"].(map[string]interface{})
		if !ok {
			return newError(ErrorSchemaInvalid, "types section not found", "", nil, nil)
		}
		typeNode, ok := typesNode[typeName].(map[string]interface{})
		if !ok {
			return newError(
				ErrorSchemaInvalid,
				fmt.Sprintf("type '%s' has invalid schema definition", typeName),
				"",
				nil,
				nil,
			)
		}
		fieldsNode, ok := typeNode["fields"].(map[string]interface{})
		if !ok {
			return newError(
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
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &RemoveResult{
		Type:  typeName,
		Field: fieldName,
	}, nil
}

func indexRebuildRequiredError(err error) *Error {
	return newError(
		ErrorDatabase,
		"index requires a full reindex",
		"Run 'rvn reindex --full' and retry",
		nil,
		err,
	)
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
