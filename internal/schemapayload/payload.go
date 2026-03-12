package schemapayload

import "github.com/aidanlsb/raven/internal/schemasvc"

func Validate(result *schemasvc.ValidateResult) map[string]interface{} {
	return map[string]interface{}{
		"valid":  result.Valid,
		"issues": result.Issues,
		"types":  result.Types,
		"traits": result.Traits,
	}
}

func AddType(result *schemasvc.AddTypeResult) map[string]interface{} {
	data := map[string]interface{}{
		"added":        "type",
		"name":         result.Name,
		"default_path": result.DefaultPath,
	}
	if result.Description != "" {
		data["description"] = result.Description
	}
	if result.NameField != "" {
		data["name_field"] = result.NameField
		data["auto_created_field"] = result.AutoCreatedField
	}
	return data
}

func AddTrait(result *schemasvc.AddTraitResult) map[string]interface{} {
	data := map[string]interface{}{
		"added": "trait",
		"name":  result.Name,
		"type":  result.Type,
	}
	if len(result.Values) > 0 {
		data["values"] = result.Values
	}
	return data
}

func AddField(result *schemasvc.AddFieldResult) map[string]interface{} {
	data := map[string]interface{}{
		"added":      "field",
		"type":       result.TypeName,
		"field":      result.FieldName,
		"field_type": result.FieldType,
		"required":   result.Required,
	}
	if result.Description != "" {
		data["description"] = result.Description
	}
	return data
}

func Update(kind, name, typeName, fieldName string, changes []string) map[string]interface{} {
	data := map[string]interface{}{
		"updated": kind,
		"changes": changes,
	}
	switch kind {
	case "type", "trait":
		data["name"] = name
	case "field":
		data["type"] = typeName
		data["field"] = fieldName
	}
	return data
}

func Remove(kind, name, typeName, fieldName string) map[string]interface{} {
	data := map[string]interface{}{
		"removed": kind,
	}
	switch kind {
	case "type", "trait":
		data["name"] = name
	case "field":
		data["type"] = typeName
		data["field"] = fieldName
	}
	return data
}

func RenameField(result *schemasvc.RenameFieldResult) map[string]interface{} {
	if result.Preview {
		return map[string]interface{}{
			"preview":       true,
			"type":          result.TypeName,
			"old_field":     result.OldField,
			"new_field":     result.NewField,
			"total_changes": result.TotalChanges,
			"changes":       result.Changes,
			"hint":          result.Hint,
		}
	}
	return map[string]interface{}{
		"renamed":         true,
		"type":            result.TypeName,
		"old_field":       result.OldField,
		"new_field":       result.NewField,
		"changes_applied": result.ChangesApplied,
		"hint":            result.Hint,
	}
}

func RenameType(result *schemasvc.RenameTypeResult) map[string]interface{} {
	if result.Preview {
		data := map[string]interface{}{
			"preview":       true,
			"old_name":      result.OldName,
			"new_name":      result.NewName,
			"total_changes": result.TotalChanges,
			"changes":       result.Changes,
			"hint":          result.Hint,
		}
		if result.DefaultPathRenameAvailable {
			data["default_path_rename_available"] = true
			data["default_path_old"] = result.DefaultPathOld
			data["default_path_new"] = result.DefaultPathNew
			data["optional_total_changes"] = result.OptionalTotalChanges
			data["optional_changes"] = result.OptionalChanges
			data["files_to_move"] = result.FilesToMove
		}
		return data
	}

	data := map[string]interface{}{
		"renamed":         true,
		"old_name":        result.OldName,
		"new_name":        result.NewName,
		"changes_applied": result.ChangesApplied,
		"hint":            result.Hint,
	}
	if result.DefaultPathRenameAvailable {
		data["default_path_rename_available"] = true
		data["default_path_renamed"] = result.DefaultPathRenamed
		data["default_path_old"] = result.DefaultPathOld
		data["default_path_new"] = result.DefaultPathNew
		data["files_moved"] = result.FilesMoved
		data["reference_files_updated"] = result.ReferenceFilesUpdated
	}
	return data
}

func MapWarnings[T any](warnings []schemasvc.Warning, build func(code, message string) T) []T {
	if len(warnings) == 0 {
		return nil
	}
	out := make([]T, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, build(warning.Code, warning.Message))
	}
	return out
}
