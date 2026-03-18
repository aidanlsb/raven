package mcp

import (
	"strings"

	"github.com/aidanlsb/raven/internal/schemapayload"
	"github.com/aidanlsb/raven/internal/schemasvc"
)

func (s *Server) resolveDirectSchemaArgs(args map[string]interface{}) (string, map[string]interface{}, string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return "", nil, errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	return vaultPath, normalizeArgs(args), "", false
}

func schemaSuccess(data map[string]interface{}) (string, bool) {
	return successEnvelope(data, nil), false
}

func schemaSuccessWithWarnings(data map[string]interface{}, warnings []directWarning) (string, bool) {
	return successEnvelope(data, warnings), false
}

func schemaTemplateDefinitionPayload(id, file, description string) map[string]interface{} {
	return map[string]interface{}{
		"id":          id,
		"file":        file,
		"description": description,
	}
}

func schemaTemplateBindingStatePayload(scopeKey, scopeValue string, state *schemasvc.TemplateBindingState) map[string]interface{} {
	return map[string]interface{}{
		scopeKey:           scopeValue,
		"templates":        state.Templates,
		"default_template": state.DefaultTemplate,
	}
}

func schemaTemplateBindingSetPayload(scopeKey, scopeValue, templateID string, result *schemasvc.AddTemplateBindingResult) map[string]interface{} {
	data := map[string]interface{}{
		scopeKey:      scopeValue,
		"template_id": templateID,
	}
	if result.AlreadySet {
		data["already_set"] = true
		data["default_match"] = result.DefaultMatch
	}
	return data
}

func schemaTemplateBindingRemovePayload(scopeKey, scopeValue, templateID string) map[string]interface{} {
	return map[string]interface{}{
		scopeKey:      scopeValue,
		"template_id": templateID,
		"removed":     true,
	}
}

func schemaTemplateDefaultPayload(scopeKey, scopeValue, defaultTemplate string) map[string]interface{} {
	return map[string]interface{}{
		scopeKey:           scopeValue,
		"default_template": defaultTemplate,
	}
}

func schemaTemplateTargetFromArgs(normalized map[string]interface{}, requireTarget bool) (string, string, string, bool, string, bool) {
	typeName := strings.TrimSpace(toString(normalized["type"]))
	coreType := strings.TrimSpace(toString(normalized["core"]))

	switch {
	case typeName != "" && coreType != "":
		return "", "", "", false, errorEnvelope("INVALID_INPUT", "specify exactly one of type or core", "", nil), true
	case typeName != "":
		return "type", "type", typeName, true, "", false
	case coreType != "":
		return "core", "core_type", coreType, true, "", false
	case requireTarget:
		return "", "", "", false, errorEnvelope("MISSING_ARGUMENT", "specify type or core", "", nil), true
	default:
		return "", "", "", false, "", false
	}
}

func (s *Server) callDirectSchemaValidate(args map[string]interface{}) (string, bool) {
	vaultPath, _, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}

	result, err := schemasvc.Validate(vaultPath)
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return schemaSuccess(schemapayload.Validate(result))
}

func (s *Server) callDirectSchemaAddType(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}

	typeName := strings.TrimSpace(toString(normalized["name"]))
	defaultPath := toString(normalized["default-path"])
	nameField := toString(normalized["name-field"])
	description := toString(normalized["description"])

	result, err := schemasvc.AddType(schemasvc.AddTypeRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		DefaultPath: defaultPath,
		NameField:   nameField,
		Description: description,
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return schemaSuccess(schemapayload.AddType(result))
}

func (s *Server) callDirectSchemaAddTrait(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}

	result, err := schemasvc.AddTrait(schemasvc.AddTraitRequest{
		VaultPath: vaultPath,
		TraitName: strings.TrimSpace(toString(normalized["name"])),
		TraitType: toString(normalized["type"]),
		Values:    toString(normalized["values"]),
		Default:   toString(normalized["default"]),
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return schemaSuccess(schemapayload.AddTrait(result))
}

func (s *Server) callDirectSchemaAddField(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}

	result, err := schemasvc.AddField(schemasvc.AddFieldRequest{
		VaultPath:   vaultPath,
		TypeName:    strings.TrimSpace(toString(normalized["type_name"])),
		FieldName:   strings.TrimSpace(toString(normalized["field_name"])),
		FieldType:   toString(normalized["type"]),
		Required:    boolValue(normalized["required"]),
		Default:     toString(normalized["default"]),
		Values:      toString(normalized["values"]),
		Target:      toString(normalized["target"]),
		Description: toString(normalized["description"]),
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return schemaSuccess(schemapayload.AddField(result))
}

func (s *Server) callDirectSchemaUpdateType(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}

	typeName := strings.TrimSpace(toString(normalized["name"]))
	result, err := schemasvc.UpdateType(schemasvc.UpdateTypeRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		DefaultPath: toString(normalized["default-path"]),
		NameField:   toString(normalized["name-field"]),
		Description: toString(normalized["description"]),
		AddTrait:    toString(normalized["add-trait"]),
		RemoveTrait: toString(normalized["remove-trait"]),
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return schemaSuccess(schemapayload.Update("type", typeName, "", "", result.Changes))
}

func (s *Server) callDirectSchemaUpdateTrait(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}

	traitName := strings.TrimSpace(toString(normalized["name"]))
	result, err := schemasvc.UpdateTrait(schemasvc.UpdateTraitRequest{
		VaultPath: vaultPath,
		TraitName: traitName,
		TraitType: toString(normalized["type"]),
		Values:    toString(normalized["values"]),
		Default:   toString(normalized["default"]),
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return schemaSuccess(schemapayload.Update("trait", traitName, "", "", result.Changes))
}

func (s *Server) callDirectSchemaUpdateField(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}

	typeName := strings.TrimSpace(toString(normalized["type_name"]))
	fieldName := strings.TrimSpace(toString(normalized["field_name"]))
	result, err := schemasvc.UpdateField(schemasvc.UpdateFieldRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		FieldName:   fieldName,
		FieldType:   toString(normalized["type"]),
		Required:    toString(normalized["required"]),
		Default:     toString(normalized["default"]),
		Values:      toString(normalized["values"]),
		Target:      toString(normalized["target"]),
		Description: toString(normalized["description"]),
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return schemaSuccess(schemapayload.Update("field", "", typeName, fieldName, result.Changes))
}

func (s *Server) callDirectSchemaRemoveType(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}

	typeName := strings.TrimSpace(toString(normalized["name"]))
	result, err := schemasvc.RemoveType(schemasvc.RemoveTypeRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		Force:       boolValue(normalized["force"]),
		Interactive: false,
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return schemaSuccessWithWarnings(schemapayload.Remove("type", typeName, "", ""), schemaWarningsToDirect(result.Warnings))
}

func (s *Server) callDirectSchemaRemoveTrait(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}

	traitName := strings.TrimSpace(toString(normalized["name"]))
	result, err := schemasvc.RemoveTrait(schemasvc.RemoveTraitRequest{
		VaultPath:   vaultPath,
		TraitName:   traitName,
		Force:       boolValue(normalized["force"]),
		Interactive: false,
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return schemaSuccessWithWarnings(schemapayload.Remove("trait", traitName, "", ""), schemaWarningsToDirect(result.Warnings))
}

func (s *Server) callDirectSchemaRemoveField(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}

	typeName := strings.TrimSpace(toString(normalized["type_name"]))
	fieldName := strings.TrimSpace(toString(normalized["field_name"]))
	_, err := schemasvc.RemoveField(schemasvc.RemoveFieldRequest{
		VaultPath: vaultPath,
		TypeName:  typeName,
		FieldName: fieldName,
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return schemaSuccess(schemapayload.Remove("field", "", typeName, fieldName))
}

func (s *Server) callDirectSchemaRenameField(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}

	result, err := schemasvc.RenameField(schemasvc.RenameFieldRequest{
		VaultPath: vaultPath,
		TypeName:  strings.TrimSpace(toString(normalized["type_name"])),
		OldField:  strings.TrimSpace(toString(normalized["old_field"])),
		NewField:  strings.TrimSpace(toString(normalized["new_field"])),
		Confirm:   boolValue(normalized["confirm"]),
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return schemaSuccess(schemapayload.RenameField(result))
}

func (s *Server) callDirectSchemaRenameType(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}

	result, err := schemasvc.RenameType(schemasvc.RenameTypeRequest{
		VaultPath:         vaultPath,
		OldName:           strings.TrimSpace(toString(normalized["old_name"])),
		NewName:           strings.TrimSpace(toString(normalized["new_name"])),
		Confirm:           boolValue(normalized["confirm"]),
		RenameDefaultPath: boolValue(normalized["rename-default-path"]),
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}

	return schemaSuccess(schemapayload.RenameType(result))
}

func (s *Server) callDirectSchemaTemplateList(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}

	targetKind, scopeKey, scopeValue, hasTarget, targetErr, targetIsErr := schemaTemplateTargetFromArgs(normalized, false)
	if targetIsErr {
		return targetErr, true
	}
	if hasTarget {
		switch targetKind {
		case "type":
			state, err := schemasvc.ListTypeTemplates(vaultPath, scopeValue)
			if err != nil {
				return mapDirectSchemaServiceError(err)
			}
			return schemaSuccess(schemaTemplateBindingStatePayload(scopeKey, scopeValue, state))
		case "core":
			state, err := schemasvc.ListCoreTemplates(vaultPath, scopeValue)
			if err != nil {
				return mapDirectSchemaServiceError(err)
			}
			return schemaSuccess(schemaTemplateBindingStatePayload(scopeKey, scopeValue, state))
		}
	}

	items, err := schemasvc.ListTemplates(vaultPath)
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}
	return schemaSuccess(map[string]interface{}{"templates": items})
}

func (s *Server) callDirectSchemaTemplateGet(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}
	templateID := strings.TrimSpace(toString(normalized["template_id"]))
	item, err := schemasvc.GetTemplate(vaultPath, templateID)
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}
	return schemaSuccess(schemaTemplateDefinitionPayload(item.ID, item.File, item.Description))
}

func (s *Server) callDirectSchemaTemplateSet(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}
	templateID := strings.TrimSpace(toString(normalized["template_id"]))
	description := toString(normalized["description"])
	item, err := schemasvc.SetTemplate(schemasvc.SetTemplateRequest{
		VaultPath:   vaultPath,
		TemplateID:  templateID,
		File:        toString(normalized["file"]),
		Description: description,
	})
	if err != nil {
		return mapDirectSchemaServiceError(err)
	}
	return schemaSuccess(schemaTemplateDefinitionPayload(item.ID, item.File, strings.TrimSpace(description)))
}

func (s *Server) callDirectSchemaTemplateRemove(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}
	templateID := strings.TrimSpace(toString(normalized["template_id"]))
	if err := schemasvc.RemoveTemplate(vaultPath, templateID); err != nil {
		return mapDirectSchemaServiceError(err)
	}
	return schemaSuccess(map[string]interface{}{
		"removed": true,
		"id":      templateID,
	})
}

func (s *Server) callDirectSchemaTemplateBind(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}
	targetKind, scopeKey, scopeValue, _, targetErr, targetIsErr := schemaTemplateTargetFromArgs(normalized, true)
	if targetIsErr {
		return targetErr, true
	}

	templateID := strings.TrimSpace(toString(normalized["template_id"]))
	setDefault := boolValue(normalized["default"])

	switch targetKind {
	case "type":
		result, err := schemasvc.AddTypeTemplate(vaultPath, scopeValue, templateID)
		if err != nil {
			return mapDirectSchemaServiceError(err)
		}
		if setDefault {
			if _, err := schemasvc.SetTypeDefaultTemplate(vaultPath, scopeValue, templateID, false); err != nil {
				return mapDirectSchemaServiceError(err)
			}
		}
		payload := schemaTemplateBindingSetPayload(scopeKey, scopeValue, templateID, result)
		if setDefault {
			payload["default_template"] = templateID
		}
		return schemaSuccess(payload)
	case "core":
		result, err := schemasvc.AddCoreTemplate(vaultPath, scopeValue, templateID)
		if err != nil {
			return mapDirectSchemaServiceError(err)
		}
		if setDefault {
			if _, err := schemasvc.SetCoreDefaultTemplate(vaultPath, scopeValue, templateID, false); err != nil {
				return mapDirectSchemaServiceError(err)
			}
		}
		payload := schemaTemplateBindingSetPayload(scopeKey, scopeValue, templateID, result)
		if setDefault {
			payload["default_template"] = templateID
		}
		return schemaSuccess(payload)
	default:
		return errorEnvelope("INVALID_INPUT", "unknown template target", "", nil), true
	}
}

func (s *Server) callDirectSchemaTemplateUnbind(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}
	targetKind, scopeKey, scopeValue, _, targetErr, targetIsErr := schemaTemplateTargetFromArgs(normalized, true)
	if targetIsErr {
		return targetErr, true
	}

	templateID := strings.TrimSpace(toString(normalized["template_id"]))
	clearDefault := boolValue(normalized["clear-default"])

	switch targetKind {
	case "type":
		if err := schemasvc.RemoveTypeTemplate(vaultPath, scopeValue, templateID, clearDefault); err != nil {
			return mapDirectSchemaServiceError(err)
		}
	case "core":
		if err := schemasvc.RemoveCoreTemplate(vaultPath, scopeValue, templateID, clearDefault); err != nil {
			return mapDirectSchemaServiceError(err)
		}
	default:
		return errorEnvelope("INVALID_INPUT", "unknown template target", "", nil), true
	}

	payload := schemaTemplateBindingRemovePayload(scopeKey, scopeValue, templateID)
	if clearDefault {
		payload["default_cleared"] = true
	}
	return schemaSuccess(payload)
}

func (s *Server) callDirectSchemaTemplateDefault(args map[string]interface{}) (string, bool) {
	vaultPath, normalized, errOut, isErr := s.resolveDirectSchemaArgs(args)
	if isErr {
		return errOut, true
	}
	targetKind, scopeKey, scopeValue, _, targetErr, targetIsErr := schemaTemplateTargetFromArgs(normalized, true)
	if targetIsErr {
		return targetErr, true
	}

	templateID := strings.TrimSpace(toString(normalized["template_id"]))
	clearDefault := boolValue(normalized["clear"])

	switch targetKind {
	case "type":
		newDefault, err := schemasvc.SetTypeDefaultTemplate(vaultPath, scopeValue, templateID, clearDefault)
		if err != nil {
			return mapDirectSchemaServiceError(err)
		}
		return schemaSuccess(schemaTemplateDefaultPayload(scopeKey, scopeValue, newDefault))
	case "core":
		newDefault, err := schemasvc.SetCoreDefaultTemplate(vaultPath, scopeValue, templateID, clearDefault)
		if err != nil {
			return mapDirectSchemaServiceError(err)
		}
		return schemaSuccess(schemaTemplateDefaultPayload(scopeKey, scopeValue, newDefault))
	default:
		return errorEnvelope("INVALID_INPUT", "unknown template target", "", nil), true
	}
}
